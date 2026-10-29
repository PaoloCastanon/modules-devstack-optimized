package mysql

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	driver "github.com/go-sql-driver/mysql"
)

type Repository struct {
	db      *sql.DB
	timeout time.Duration
}

var _ repository.Repository = (*Repository)(nil)

func New(db *sql.DB, timeout time.Duration) *Repository { return &Repository{db, timeout} }
func translate(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	var my *driver.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1062:
			_, key, ok := strings.Cut(my.Message, "for key ")
			if ok {
				key = strings.Trim(key, "'` ")
				if key == "PRIMARY" || strings.HasSuffix(key, ".PRIMARY") {
					return repository.ErrDuplicateUUID
				}
				if key == "name" || strings.HasSuffix(key, ".name") {
					return repository.ErrDuplicateName
				}
			}
			return repository.ErrDuplicate
		case 1213, 1205:
			return repository.ErrConcurrent
		case 1216, 1452:
			return repository.ErrUnknownClass
		}
	}
	return err
}
func (r *Repository) Create(ctx context.Context, p model.Provider) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "INSERT INTO resource_providers (uuid,name,generation) VALUES (?,?,0)", p.UUID, p.Name)
	return translate(err)
}
func scanProvider(row interface{ Scan(...any) error }) (model.Provider, error) {
	var p model.Provider
	err := row.Scan(&p.UUID, &p.Name, &p.Generation)
	return p, translate(err)
}
func (r *Repository) Get(ctx context.Context, id string) (model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return scanProvider(r.db.QueryRowContext(ctx, "SELECT uuid,name,generation FROM resource_providers WHERE uuid=?", id))
}
func (r *Repository) List(ctx context.Context, f model.Filter) ([]model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query := "SELECT uuid,name,generation FROM resource_providers"
	predicates := []string{}
	args := []any{}
	if f.Name != nil {
		predicates = append(predicates, "name=?")
		args = append(args, *f.Name)
	}
	if f.UUID != nil {
		predicates = append(predicates, "uuid=?")
		args = append(args, *f.UUID)
	}
	if len(f.Aggregates) > 0 {
		predicates = append(predicates, "EXISTS(SELECT 1 FROM resource_provider_aggregates a WHERE a.provider_uuid=resource_providers.uuid AND a.aggregate_uuid IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.Aggregates)), ",")+"))")
		for _, id := range f.Aggregates {
			args = append(args, id)
		}
	}
	classes := make([]string, 0, len(f.Resources))
	for name := range f.Resources {
		classes = append(classes, name)
	}
	sort.Strings(classes)
	for _, name := range classes {
		amount := f.Resources[name]
		// B08 has no allocations. B09 must subtract usage before enabling claims.
		predicates = append(predicates, "EXISTS(SELECT 1 FROM inventories i WHERE i.provider_uuid=resource_providers.uuid AND i.resource_class=? AND (i.total-i.reserved)*i.allocation_ratio>=? AND i.min_unit<=? AND i.max_unit>=? AND MOD(?,i.step_size)=0)")
		args = append(args, name, amount, amount, amount, amount)
	}
	if len(predicates) > 0 {
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	query += " ORDER BY uuid"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Provider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// All mutations lock the parent row, serializing them with inventory generation changes.
func (r *Repository) transaction(ctx context.Context, f func(*sql.Tx) error) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return translate(err)
	}
	defer tx.Rollback()
	if err = f(tx); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}
func lockProvider(ctx context.Context, tx *sql.Tx, id string) (model.Provider, error) {
	return scanProvider(tx.QueryRowContext(ctx, "SELECT uuid,name,generation FROM resource_providers WHERE uuid=? FOR UPDATE", id))
}
func (r *Repository) Rename(ctx context.Context, id, name string) (model.Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var p model.Provider
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = lockProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE resource_providers SET name=? WHERE uuid=?", name, id)
		if err == nil {
			p.Name = name
		}
		return err
	})
	return p, err
}
func (r *Repository) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := lockProvider(ctx, tx, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM resource_providers WHERE uuid=?", id)
		return err
	})
}
func (r *Repository) Inventories(ctx context.Context, id string) (model.Inventories, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	result := model.Inventories{Inventories: map[string]model.Inventory{}}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT generation FROM resource_providers WHERE uuid=?", id).Scan(&result.Generation); err != nil {
		return result, translate(err)
	}
	// Match MySQL's text FLOAT representation used by the original Python driver.
	// Parameter values remain bound; CAST does not interpolate SQL.
	rows, err := tx.QueryContext(ctx, "SELECT resource_class,total,reserved,min_unit,max_unit,step_size,CAST(allocation_ratio AS CHAR) FROM inventories WHERE provider_uuid=? ORDER BY resource_class", id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var class string
		var inv model.Inventory
		if err = rows.Scan(&class, &inv.Total, &inv.Reserved, &inv.MinUnit, &inv.MaxUnit, &inv.StepSize, &inv.AllocationRatio); err != nil {
			break
		}
		result.Inventories[class] = inv
	}
	rowErr := rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return result, err
	}
	if rowErr != nil {
		return result, rowErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	return result, tx.Commit()
}
func (r *Repository) ReplaceInventories(ctx context.Context, id string, generation int64, invs map[string]model.Inventory) (model.Inventories, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	result := model.Inventories{Generation: generation + 1, Inventories: invs}
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		p, err := lockProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Generation != generation {
			return repository.ErrConcurrent
		}
		updated, err := tx.ExecContext(ctx, "UPDATE resource_providers SET generation=generation+1 WHERE uuid=? AND generation=?", id, generation)
		if err != nil {
			return err
		}
		n, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return repository.ErrConcurrent
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM inventories WHERE provider_uuid=?", id); err != nil {
			return err
		}
		classes := make([]string, 0, len(invs))
		for class := range invs {
			classes = append(classes, class)
		}
		sort.Strings(classes)
		for _, class := range classes {
			inv := invs[class]
			if _, err = tx.ExecContext(ctx, "INSERT INTO inventories (provider_uuid,resource_class,total,reserved,min_unit,max_unit,step_size,allocation_ratio) VALUES (?,?,?,?,?,?,?,?)", id, class, inv.Total, inv.Reserved, inv.MinUnit, inv.MaxUnit, inv.StepSize, inv.AllocationRatio); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Inventories{}, err
	}
	return result, nil
}
