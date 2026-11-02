package mysql

import (
	"context"
	"database/sql"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
)

var _ repository.Candidates = (*Repository)(nil)

// Every relation is read in one repeatable-read snapshot. An allocation writer
// cannot make a summary mix new usages with an older inventory or trait set.
func (r *Repository) CandidateState(ctx context.Context) (model.CandidateState, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out := model.CandidateState{Providers: []model.CandidateProvider{}, Classes: map[string]bool{}, Traits: map[string]bool{}}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	read := func(query string, scan func(*sql.Rows) error) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		for rows.Next() {
			if err = scan(rows); err != nil {
				break
			}
		}
		rowErr, closeErr := rows.Err(), rows.Close()
		if err != nil {
			return err
		}
		if rowErr != nil {
			return rowErr
		}
		return closeErr
	}
	if err = read("SELECT name FROM resource_classes", func(rows *sql.Rows) error {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		out.Classes[name] = true
		return nil
	}); err != nil {
		return out, err
	}
	if err = read("SELECT name FROM traits", func(rows *sql.Rows) error {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		out.Traits[name] = true
		return nil
	}); err != nil {
		return out, err
	}
	indexes := map[string]int{}
	if err = read("SELECT uuid,name,generation FROM resource_providers ORDER BY uuid", func(rows *sql.Rows) error {
		item := model.CandidateProvider{Inventories: map[string]model.Inventory{}, Usage: map[string]int64{}, Traits: map[string]bool{}, Aggregates: map[string]bool{}}
		if err := rows.Scan(&item.Provider.UUID, &item.Provider.Name, &item.Provider.Generation); err != nil {
			return err
		}
		indexes[item.Provider.UUID] = len(out.Providers)
		out.Providers = append(out.Providers, item)
		return nil
	}); err != nil {
		return out, err
	}
	if err = read("SELECT provider_uuid,resource_class,total,reserved,min_unit,max_unit,step_size,CAST(allocation_ratio AS CHAR) FROM inventories", func(rows *sql.Rows) error {
		var id, class string
		var inv model.Inventory
		if err := rows.Scan(&id, &class, &inv.Total, &inv.Reserved, &inv.MinUnit, &inv.MaxUnit, &inv.StepSize, &inv.AllocationRatio); err != nil {
			return err
		}
		out.Providers[indexes[id]].Inventories[class] = inv
		return nil
	}); err != nil {
		return out, err
	}
	if err = read("SELECT provider_uuid,trait_name FROM resource_provider_traits", func(rows *sql.Rows) error {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		out.Providers[indexes[id]].Traits[name] = true
		return nil
	}); err != nil {
		return out, err
	}
	if err = read("SELECT provider_uuid,aggregate_uuid FROM resource_provider_aggregates", func(rows *sql.Rows) error {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		out.Providers[indexes[id]].Aggregates[name] = true
		return nil
	}); err != nil {
		return out, err
	}
	if err = read("SELECT provider_uuid,resource_class,SUM(used) FROM allocations GROUP BY provider_uuid,resource_class", func(rows *sql.Rows) error {
		var id, class string
		var used int64
		if err := rows.Scan(&id, &class, &used); err != nil {
			return err
		}
		out.Providers[indexes[id]].Usage[class] = used
		return nil
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
