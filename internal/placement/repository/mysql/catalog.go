package mysql

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	driver "github.com/go-sql-driver/mysql"
)

var _ repository.Catalog = (*Repository)(nil)

func (r *Repository) ResourceClassExists(ctx context.Context, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var found bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM resource_classes WHERE BINARY name=BINARY ?)", name).Scan(&found)
	return found, err
}
func (r *Repository) Classes(ctx context.Context) ([]model.CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, "SELECT name,standard FROM resource_classes ORDER BY COALESCE(ordinal,2147483647),name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CatalogEntry{}
	for rows.Next() {
		var item model.CatalogEntry
		if err := rows.Scan(&item.Name, &item.Standard); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func catalogRow(row interface{ Scan(...any) error }) (model.CatalogEntry, error) {
	var item model.CatalogEntry
	err := row.Scan(&item.Name, &item.Standard)
	return item, translate(err)
}
func (r *Repository) Class(ctx context.Context, name string) (model.CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return catalogRow(r.db.QueryRowContext(ctx, "SELECT name,standard FROM resource_classes WHERE BINARY name=BINARY ?", name))
}
func (r *Repository) CreateClass(ctx context.Context, name string, idempotent bool) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "INSERT INTO resource_classes(name,standard) VALUES (?,FALSE)", name)
	err = translate(err)
	if idempotent && errors.Is(err, repository.ErrDuplicate) {
		return false, nil
	}
	return err == nil, err
}
func (r *Repository) RenameClass(ctx context.Context, name, to string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.transaction(ctx, func(tx *sql.Tx) error {
		item, err := catalogRow(tx.QueryRowContext(ctx, "SELECT name,standard FROM resource_classes WHERE BINARY name=BINARY ? FOR UPDATE", name))
		if err != nil {
			return err
		}
		if item.Standard {
			return repository.ErrStandard
		}
		_, err = tx.ExecContext(ctx, "UPDATE resource_classes SET name=? WHERE BINARY name=BINARY ?", to, name)
		return err // FK ON UPDATE CASCADE preserves inventory and provider generation.
	})
}
func catalogDeleteError(err error) error {
	var mysql *driver.MySQLError
	if errors.As(err, &mysql) && (mysql.Number == 1217 || mysql.Number == 1451) {
		return repository.ErrInUse
	}
	return translate(err)
}
func (r *Repository) DeleteClass(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		item, err := catalogRow(tx.QueryRowContext(ctx, "SELECT name,standard FROM resource_classes WHERE BINARY name=BINARY ? FOR UPDATE", name))
		if err != nil {
			return err
		}
		if item.Standard {
			return repository.ErrStandard
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM resource_classes WHERE BINARY name=BINARY ?", name)
		return err
	})
	return catalogDeleteError(err)
}
func (r *Repository) Traits(ctx context.Context, f model.TraitFilter) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	q := "SELECT t.name FROM traits t"
	args := []any{}
	where := []string{}
	if f.Prefix != nil {
		where = append(where, "t.name LIKE ?")
		args = append(args, *f.Prefix+"%")
	}
	if f.Names != nil {
		if len(f.Names) == 0 {
			return []string{}, nil
		}
		where = append(where, "t.name IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.Names)), ",")+")")
		for _, name := range f.Names {
			args = append(args, name)
		}
	}
	if f.Associated != nil {
		condition := "EXISTS(SELECT 1 FROM resource_provider_traits a WHERE a.trait_name=t.name)"
		if !*f.Associated {
			condition = "NOT " + condition
		}
		where = append(where, condition)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY t.name"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
func (r *Repository) Trait(ctx context.Context, name string) (model.CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return catalogRow(r.db.QueryRowContext(ctx, "SELECT name,standard FROM traits WHERE BINARY name=BINARY ?", name))
}
func (r *Repository) EnsureTrait(ctx context.Context, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "INSERT INTO traits(name,standard) VALUES (?,FALSE)", name)
	err = translate(err)
	if errors.Is(err, repository.ErrDuplicate) {
		return false, nil
	}
	return err == nil, err
}
func (r *Repository) DeleteTrait(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		item, err := catalogRow(tx.QueryRowContext(ctx, "SELECT name,standard FROM traits WHERE BINARY name=BINARY ? FOR UPDATE", name))
		if err != nil {
			return err
		}
		if item.Standard {
			return repository.ErrStandard
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM traits WHERE BINARY name=BINARY ?", name)
		return err
	})
	return catalogDeleteError(err)
}

// query is an internal constant, never an identifier or query supplied by HTTP.
func (r *Repository) associationSnapshot(ctx context.Context, id, query string) (model.TraitSet, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out := model.TraitSet{Traits: []string{}}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT generation FROM resource_providers WHERE uuid=?", id).Scan(&out.Generation); err != nil {
		return out, translate(err)
	}
	rows, err := tx.QueryContext(ctx, query, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			break
		}
		out.Traits = append(out.Traits, name)
	}
	rowErr, closeErr := rows.Err(), rows.Close()
	if err != nil {
		return out, err
	}
	if rowErr != nil {
		return out, rowErr
	}
	if closeErr != nil {
		return out, closeErr
	}
	return out, tx.Commit()
}
func (r *Repository) ProviderTraits(ctx context.Context, id string) (model.TraitSet, error) {
	return r.associationSnapshot(ctx, id, "SELECT trait_name FROM resource_provider_traits WHERE provider_uuid=? ORDER BY trait_name")
}
func (r *Repository) ReplaceTraits(ctx context.Context, id string, generation int64, names []string) (model.TraitSet, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	wanted := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		if !seen[name] {
			wanted = append(wanted, name)
			seen[name] = true
		}
	}
	sort.Strings(wanted)
	out := model.TraitSet{Traits: wanted}
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		p, err := lockProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		out.Generation = p.Generation
		if p.Generation != generation {
			return repository.ErrConcurrent
		}
		missing := []string{}
		// Sorted catalogue row locks keep writers/deletes from racing the FK.
		for _, name := range wanted {
			var actual string
			err := tx.QueryRowContext(ctx, "SELECT name FROM traits WHERE BINARY name=BINARY ? FOR SHARE", name).Scan(&actual)
			if errors.Is(err, sql.ErrNoRows) {
				missing = append(missing, name)
			} else if err != nil {
				return err
			}
		}
		if len(missing) > 0 {
			return &repository.MissingTraits{Names: missing}
		}
		rows, err := tx.QueryContext(ctx, "SELECT trait_name FROM resource_provider_traits WHERE provider_uuid=? ORDER BY trait_name", id)
		if err != nil {
			return err
		}
		existing := []string{}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				break
			}
			existing = append(existing, name)
		}
		rowErr, closeErr := rows.Err(), rows.Close()
		if err != nil {
			return err
		}
		if rowErr != nil {
			return rowErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(existing) == len(wanted) {
			same := true
			for i := range existing {
				same = same && existing[i] == wanted[i]
			}
			if same {
				return nil
			}
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM resource_provider_traits WHERE provider_uuid=?", id); err != nil {
			return err
		}
		for _, name := range wanted {
			if _, err = tx.ExecContext(ctx, "INSERT INTO resource_provider_traits(provider_uuid,trait_name) VALUES (?,?)", id, name); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, "UPDATE resource_providers SET generation=generation+1 WHERE uuid=? AND generation=?", id, generation)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return repository.ErrConcurrent
		}
		out.Generation++
		return nil
	})
	return out, err
}
func (r *Repository) Aggregates(ctx context.Context, id string) ([]string, error) {
	out, err := r.associationSnapshot(ctx, id, "SELECT aggregate_uuid FROM resource_provider_aggregates WHERE provider_uuid=? ORDER BY aggregate_uuid")
	return out.Traits, err
}
func (r *Repository) ReplaceAggregates(ctx context.Context, id string, names []string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := lockProvider(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM resource_provider_aggregates WHERE provider_uuid=?", id); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, name := range names {
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, err := tx.ExecContext(ctx, "INSERT INTO resource_provider_aggregates(provider_uuid,aggregate_uuid) VALUES (?,?)", id, name); err != nil {
				return err
			}
		}
		return nil // 1.1–1.7 aggregates do not change generation.
	})
	return append([]string{}, names...), err
}
func (r *Repository) DeleteInventories(ctx context.Context, id string) error {
	p, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.ReplaceInventories(ctx, id, p.Generation, map[string]model.Inventory{})
	return err
}
