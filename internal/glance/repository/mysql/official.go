package mysql

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/google/uuid"
)

// Official catalog dialect. With UseOfficialCatalog the repository reads and
// writes the original Glance tables (images, image_properties, image_tags,
// image_locations in the official database) instead of Glance Go's own copy,
// so both implementations serve one catalog and one filesystem store.
// Differences handled here, as the original's SQLAlchemy models define them:
//   - children carry id/created_at/deleted columns; reads skip deleted rows;
//   - an active image has an image_locations row pointing at its file;
//   - deletion soft-deletes children and locations, as image_destroy does.
// Transfer bookkeeping (image_data_operations) stays in the paired shared
// database; images the original uploaded get a committed operation adopted
// from their verified metadata and single local location.

// SupportedOfficialRevisions are the original's alembic heads this dialect
// was checked against; any other revision fails closed at startup.
var SupportedOfficialRevisions = []string{"2024_1_contract01"}

var officialTables = regexp.MustCompile(`\b(FROM|INTO|UPDATE|JOIN)\s+(images|image_properties|image_tags|image_locations)\b`)

// officialNamespace derives the attempt ID of an adopted original upload.
var officialNamespace = uuid.MustParse("6f1d1c64-6c2b-5b8e-9b0a-2f6f4d6e9a10")

type officialCatalog struct {
	schema   string
	storeDir string
	cache    sync.Map // original SQL -> rewritten SQL
}

// UseOfficialCatalog switches the repository to the original's tables in
// schema (normally "glance") with files under storeDir (its
// filesystem_store_datadir). Call before PrepareStatements and before serving.
func (r *Repository) UseOfficialCatalog(schema, storeDir string) error {
	if !OfficialCatalogName(schema) || !filepath.IsAbs(storeDir) || filepath.Clean(storeDir) != storeDir || r.stmts != nil {
		return errors.New("invalid official catalog configuration")
	}
	r.official = &officialCatalog{schema: schema, storeDir: storeDir}
	return nil
}

// VerifyOfficialCatalog checks the original's schema revision.
func (r *Repository) VerifyOfficialCatalog(ctx context.Context) error {
	if r.official == nil {
		return nil
	}
	var revision string
	if err := r.db.QueryRowContext(ctx, "SELECT version_num FROM `"+r.official.schema+"`.alembic_version").Scan(&revision); err != nil {
		return errors.New("cannot read the official catalog revision")
	}
	for _, known := range SupportedOfficialRevisions {
		if revision == known {
			return nil
		}
	}
	return errors.New("unsupported official catalog revision " + revision + "; manual review required")
}

// sql qualifies catalog tables with the official schema and adds the
// soft-delete filter to child reads. Own mode returns q unchanged.
func (r *Repository) sql(q string) string {
	o := r.official
	if o == nil {
		return q
	}
	if v, ok := o.cache.Load(q); ok {
		return v.(string)
	}
	out := officialTables.ReplaceAllString(q, "$1 `"+o.schema+"`.$2")
	if strings.HasPrefix(q, "SELECT") {
		for _, table := range []string{"image_properties", "image_tags", "image_locations"} {
			out = strings.ReplaceAll(out, "`"+o.schema+"`."+table+" WHERE ", "`"+o.schema+"`."+table+" WHERE deleted=0 AND ")
		}
		out = strings.ReplaceAll(out, "image_properties p WHERE ", "image_properties p WHERE p.deleted=0 AND ")
		out = strings.ReplaceAll(out, "image_tags t WHERE ", "image_tags t WHERE t.deleted=0 AND ")
	}
	if len(q) < 4096 { // dynamic list queries vary; keep the cache bounded to static SQL
		o.cache.Store(q, out)
	}
	return out
}

func (r *Repository) exec(ctx context.Context, tx *sql.Tx, q string, args ...any) (sql.Result, error) {
	return tx.ExecContext(ctx, r.sql(q), args...)
}

func (r *Repository) ack(ctx context.Context, tx *sql.Tx, q string, args ...any) error {
	return acknowledged(ctx, tx, r.sql(q), args...)
}

func (r *Repository) location(id string) string { return "file://" + r.official.storeDir + "/" + id }

// OfficialAttempt is the attempt ID adopted for an image the original uploaded.
func OfficialAttempt(id string) string { return uuid.NewSHA1(officialNamespace, []byte(id)).String() }

// adoptable builds the committed operation of an active original upload
// with no bookkeeping row: verified sha512 metadata and exactly one active
// location, the local file named by its ID. Anything else is not adopted.
func (r *Repository) adoptable(ctx context.Context, tx *sql.Tx, s model.DataState) (*model.DataOperation, error) {
	if r.official == nil || s.Deleted || s.Status != "active" || s.Size == nil || s.Checksum == nil ||
		s.HashAlgorithm == nil || *s.HashAlgorithm != "sha512" || s.HashValue == nil {
		return nil, nil
	}
	rows, err := r.query(ctx, tx, "SELECT value FROM image_locations WHERE image_id=? AND status='active'", s.ImageID)
	if err != nil {
		return nil, err
	}
	var values []string
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			_ = rows.Close()
			return nil, err
		}
		values = append(values, v)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if len(values) != 1 || values[0] != r.location(s.ImageID) {
		return nil, nil
	}
	o := model.DataOperation{ImageID: s.ImageID, AttemptID: OfficialAttempt(s.ImageID), Phase: model.DataCommitted,
		ExpectedSize: s.Size, Size: s.Size, VirtualSize: s.VirtualSize, Checksum: s.Checksum, SHA512: s.HashValue,
		CreatedAt: s.UpdatedAt, UpdatedAt: s.UpdatedAt}
	if o.Validate() != nil {
		return nil, nil
	}
	return &o, nil
}

// materialize records an adopted operation so later bookkeeping updates
// (deletion cleanup) act on a real row, inside the caller's locking tx.
func (r *Repository) materialize(ctx context.Context, tx *sql.Tx, o *model.DataOperation) error {
	return r.ack(ctx, tx, "INSERT INTO image_data_operations (image_id, attempt_id, phase, expected_size, size, virtual_size, checksum, os_hash_value, created_at, updated_at) VALUES (?, ?, 'committed', ?, ?, ?, ?, ?, ?, ?)",
		o.ImageID, o.AttemptID, o.ExpectedSize, o.Size, o.VirtualSize, o.Checksum, o.SHA512, o.CreatedAt, o.UpdatedAt)
}

// destroyChildren soft-deletes what the original's image_destroy removes
// with the image: locations, properties and tags.
func (r *Repository) destroyChildren(ctx context.Context, tx *sql.Tx, id string, at any) error {
	if r.official == nil {
		return nil
	}
	for _, q := range []string{
		"UPDATE image_locations SET deleted=1, deleted_at=?, updated_at=?, status='deleted' WHERE image_id=? AND deleted=0",
		"UPDATE image_properties SET deleted=1, deleted_at=?, updated_at=? WHERE image_id=? AND deleted=0",
		"UPDATE image_tags SET deleted=1, deleted_at=?, updated_at=? WHERE image_id=? AND deleted=0",
	} {
		if _, err := r.exec(ctx, tx, q, at, at, id); err != nil {
			return err
		}
	}
	return nil
}
