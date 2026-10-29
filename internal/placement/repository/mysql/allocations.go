package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"math"
	"sort"
	"strings"
)

var _ repository.Allocations = (*Repository)(nil)

func (r *Repository) ConsumerAllocations(ctx context.Context, id string) (model.ConsumerAllocations, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out := model.ConsumerAllocations{Allocations: map[string]model.ResourceAllocation{}}
	rows, err := r.db.QueryContext(ctx, "SELECT a.provider_uuid,p.generation,a.resource_class,a.used FROM allocations a JOIN resource_providers p ON p.uuid=a.provider_uuid WHERE a.consumer_uuid=? ORDER BY a.provider_uuid,a.resource_class", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var provider, class string
		var generation, used int64
		if err := rows.Scan(&provider, &generation, &class, &used); err != nil {
			return out, err
		}
		item, ok := out.Allocations[provider]
		if !ok {
			item = model.ResourceAllocation{Generation: generation, Resources: map[string]int64{}}
		}
		item.Resources[class] = used
		out.Allocations[provider] = item
	}
	return out, rows.Err()
}
func (r *Repository) ProviderAllocations(ctx context.Context, id string) (model.ProviderAllocations, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out := model.ProviderAllocations{Allocations: map[string]model.ConsumerResources{}}
	rows, err := r.db.QueryContext(ctx, "SELECT p.generation,a.consumer_uuid,a.resource_class,a.used FROM resource_providers p LEFT JOIN allocations a ON a.provider_uuid=p.uuid WHERE p.uuid=? ORDER BY a.consumer_uuid,a.resource_class", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var consumer, class sql.NullString
		var used sql.NullInt64
		if err := rows.Scan(&out.Generation, &consumer, &class, &used); err != nil {
			return out, err
		}
		found = true
		if !consumer.Valid {
			continue
		}
		item, ok := out.Allocations[consumer.String]
		if !ok {
			item = model.ConsumerResources{Resources: map[string]int64{}}
		}
		item.Resources[class.String] = used.Int64
		out.Allocations[consumer.String] = item
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if !found {
		return out, repository.ErrNotFound
	}
	return out, nil
}
func (r *Repository) Usages(ctx context.Context, id string) (model.Usages, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	out := model.Usages{Usages: map[string]int64{}}
	rows, err := r.db.QueryContext(ctx, "SELECT p.generation,i.resource_class,COALESCE(SUM(a.used),0) FROM resource_providers p LEFT JOIN inventories i ON i.provider_uuid=p.uuid LEFT JOIN allocations a ON a.provider_uuid=i.provider_uuid AND a.resource_class=i.resource_class WHERE p.uuid=? GROUP BY p.generation,i.resource_class", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var class sql.NullString
		var used int64
		if err := rows.Scan(&out.Generation, &class, &used); err != nil {
			return out, err
		}
		found = true
		if class.Valid {
			out.Usages[class.String] = used
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if !found {
		return out, repository.ErrNotFound
	}
	return out, nil
}

// Consumer first, then the sorted union of old/new providers. Inventory and
// trait writers lock only their provider, so no path takes consumer locks second.
// READ COMMITTED makes usage reads fresh after a blocked provider lock is won.
func (r *Repository) allocationTransaction(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return translate(err)
	}
	defer tx.Rollback()
	if err = f(tx); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}
func allocationProviders(ctx context.Context, tx *sql.Tx, consumer string, wanted map[string]map[string]int64) ([]string, error) {
	all := map[string]bool{}
	for id := range wanted {
		all[id] = true
	}
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT provider_uuid FROM allocations WHERE consumer_uuid=?", consumer)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		all[id] = true
	}
	rowErr, closeErr := rows.Err(), rows.Close()
	if err != nil {
		return nil, err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	ids := make([]string, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
func allocationFailure(class, provider, reason string) error {
	return model.Fail(409, fmt.Sprintf("Unable to allocate inventory: Unable to create allocation for '%s' on resource provider '%s'. The requested amount would %s.", class, provider, reason))
}
func (r *Repository) ReplaceAllocations(ctx context.Context, consumer string, update model.AllocationUpdate) error {
	wanted := update.Allocations
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if len(wanted) == 0 {
		return model.Fail(400, "Empty allocations require DELETE at this microversion.")
	}
	return r.allocationTransaction(ctx, func(tx *sql.Tx) error {
		// An insert collision also obtains the same exclusive consumer row lock.
		if _, err := tx.ExecContext(ctx, "INSERT INTO consumers(uuid,project_id,user_id) VALUES (?,?,?) ON DUPLICATE KEY UPDATE uuid=uuid", consumer, update.ProjectID, update.UserID); err != nil {
			return err
		}
		ids, err := allocationProviders(ctx, tx, consumer, wanted)
		if err != nil {
			return err
		}
		generations := map[string]int64{}
		for _, id := range ids {
			p, err := lockProvider(ctx, tx, id)
			if errors.Is(err, repository.ErrNotFound) {
				return model.Fail(400, "Allocation for resource provider '"+id+"' that does not exist.")
			}
			if err != nil {
				return err
			}
			generations[id] = p.Generation
		}
		// Snapshot usage before removing this consumer, including any existing overage.
		previous := map[string]map[string]int64{}
		for _, id := range ids {
			if _, ok := wanted[id]; !ok {
				continue
			}
			previous[id] = map[string]int64{}
			classes := sortedResourceNames(wanted[id])
			if len(classes) == 0 {
				return model.Fail(400, "Allocation resources must be non-empty.")
			}
			for _, class := range classes {
				var known bool
				if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM resource_classes WHERE BINARY name=BINARY ?)", class).Scan(&known); err != nil {
					return err
				}
				if !known {
					return model.Fail(400, "Unable to allocate inventory for consumer "+consumer+": No such resource class "+class+".")
				}
				var used int64
				if err := tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(used),0) FROM allocations WHERE provider_uuid=? AND resource_class=?", id, class).Scan(&used); err != nil {
					return err
				}
				previous[id][class] = used
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM allocations WHERE consumer_uuid=?", consumer); err != nil {
			return err
		}
		for _, id := range ids {
			resources, ok := wanted[id]
			if !ok {
				continue
			}
			for _, class := range sortedResourceNames(resources) {
				amount := resources[class]
				var inv model.Inventory
				err := tx.QueryRowContext(ctx, "SELECT total,reserved,min_unit,max_unit,step_size,CAST(allocation_ratio AS CHAR) FROM inventories WHERE provider_uuid=? AND resource_class=? FOR SHARE", id, class).Scan(&inv.Total, &inv.Reserved, &inv.MinUnit, &inv.MaxUnit, &inv.StepSize, &inv.AllocationRatio)
				if errors.Is(err, sql.ErrNoRows) {
					return model.Fail(409, fmt.Sprintf("Unable to allocate inventory: Inventory for '%s' on resource provider '%s' invalid.", class, id))
				}
				if err != nil {
					return err
				}
				if amount < inv.MinUnit || amount > inv.MaxUnit || amount%inv.StepSize != 0 {
					return allocationFailure(class, id, "violate inventory constraints")
				}
				var other int64
				if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(used),0) FROM allocations WHERE provider_uuid=? AND resource_class=?", id, class).Scan(&other); err != nil {
					return err
				}
				if amount > math.MaxInt64-other {
					return allocationFailure(class, id, "exceed the capacity")
				}
				combined := other + amount
				// Match upstream: existing overcapacity may stay equal or improve.
				if allocationExceedsCapacity(combined, inv) && combined > previous[id][class] {
					return allocationFailure(class, id, "exceed the capacity")
				}
				if _, err = tx.ExecContext(ctx, "INSERT INTO allocations(consumer_uuid,provider_uuid,resource_class,used) VALUES (?,?,?,?)", consumer, id, class, amount); err != nil {
					return err
				}
			}
		}
		for _, id := range ids {
			if _, ok := wanted[id]; !ok {
				continue
			}
			result, err := tx.ExecContext(ctx, "UPDATE resource_providers SET generation=generation+1 WHERE uuid=? AND generation=?", id, generations[id])
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
		}
		_, err = tx.ExecContext(ctx, "UPDATE consumers SET generation=generation+1,project_id=?,user_id=? WHERE uuid=?", update.ProjectID, update.UserID, consumer)
		return err
	})
}
func sortedResourceNames(resources map[string]int64) []string {
	out := make([]string, 0, len(resources))
	for name := range resources {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func (r *Repository) DeleteAllocations(ctx context.Context, consumer string) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.allocationTransaction(ctx, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM consumers WHERE uuid=? FOR UPDATE", consumer).Scan(&id); err != nil {
			return err
		}
		ids, err := allocationProviders(ctx, tx, id, nil)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return repository.ErrNotFound
		}
		for _, provider := range ids {
			if _, err := lockProvider(ctx, tx, provider); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM consumers WHERE uuid=?", id)
		return err // FK removes allocations, without provider generation increments.
	})
}

// Inventory deletion is refused for used classes; updating a retained class is
// allowed even when capacity shrinks below usage, matching the original warning.
func inventoryRemovalCheck(ctx context.Context, tx *sql.Tx, id string, wanted map[string]model.Inventory) error {
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT resource_class FROM allocations WHERE provider_uuid=? ORDER BY resource_class", id)
	if err != nil {
		return err
	}
	classes := []string{}
	for rows.Next() {
		var class string
		if err = rows.Scan(&class); err != nil {
			break
		}
		if _, exists := wanted[class]; !exists {
			classes = append(classes, class)
		}
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
	if len(classes) > 0 {
		return &repository.InventoryInUse{Provider: id, Classes: classes}
	}
	return nil
}
func replaceInventoryRows(ctx context.Context, tx *sql.Tx, id string, invs map[string]model.Inventory) error {
	classes := make([]string, 0, len(invs))
	for class := range invs {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	query := "DELETE FROM inventories WHERE provider_uuid=?"
	args := []any{id}
	if len(classes) > 0 {
		query += " AND resource_class NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", len(classes)), ",") + ")"
		for _, class := range classes {
			args = append(args, class)
		}
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	for _, class := range classes {
		inv := invs[class]
		if _, err := tx.ExecContext(ctx, "INSERT INTO inventories(provider_uuid,resource_class,total,reserved,min_unit,max_unit,step_size,allocation_ratio) VALUES (?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE total=VALUES(total),reserved=VALUES(reserved),min_unit=VALUES(min_unit),max_unit=VALUES(max_unit),step_size=VALUES(step_size),allocation_ratio=VALUES(allocation_ratio)", id, class, inv.Total, inv.Reserved, inv.MinUnit, inv.MaxUnit, inv.StepSize, inv.AllocationRatio); err != nil {
			return err
		}
	}
	return nil
}

// Compare an integer against the floor of the binary64 capacity. Converting used
// to float would round 2^53+1 down and could admit one extra unit. Python's
// int/float comparison preserves that distinction.
func allocationExceedsCapacity(used int64, inv model.Inventory) bool {
	capacity := float64(inv.Total-inv.Reserved) * inv.AllocationRatio
	return capacity < math.Exp2(63) && (capacity < 0 || used > int64(capacity))
}
