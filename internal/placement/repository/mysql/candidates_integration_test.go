//go:build integration

package mysql_test

import (
	"context"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository/mysql"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
	"github.com/google/uuid"
	"os"
	"testing"
	"time"
)

func TestB10MySQLCandidates(t *testing.T) {
	if os.Getenv("PLACEMENT_GO_MYSQL_TEST") != "1" {
		t.Fatal("disposable MySQL opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	must(err)
	defer admin.Close()
	var count int
	must(admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count))
	if count != 0 {
		t.Fatal("NEW EMPTY schema required")
	}
	must(mysql.Migrate(ctx, admin))
	db, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_APP_DSN_FILE"))
	must(err)
	defer db.Close()
	must(mysql.CheckAccount(ctx, db, "serve"))
	must(mysql.VerifySchema(ctx, db))
	r := mysql.New(db, 5*time.Second)
	p, _ := model.NewProvider("b10-candidate", "")
	must(r.Create(ctx, p))
	inv := model.Inventory{Total: 8, MinUnit: 1, MaxUnit: 8, StepSize: 1, AllocationRatio: 1}
	_, err = r.ReplaceInventories(ctx, p.UUID, 0, map[string]model.Inventory{"VCPU": inv, "MEMORY_MB": inv})
	must(err)
	_, err = r.ReplaceTraits(ctx, p.UUID, 1, []string{"OWNER_NOVA"})
	must(err)
	identity := auth.Identity{UserID: "fixture", ProjectID: "fixture", Roles: []string{"service"}, ExpiresAt: time.Now().Add(time.Hour)}
	q := model.CandidateQuery{Groups: []model.RequestGroup{{Resources: map[string]int64{"VCPU": 2}}}, RootRequired: []string{"OWNER_NOVA"}}
	svc := service.New(r)
	out, err := svc.Candidates(ctx, identity, q)
	must(err)
	if len(out.Requests) != 1 {
		t.Fatal(out)
	}
	agg := uuid.NewString()
	set, err := r.AggregateState(ctx, p.UUID)
	must(err)
	_, err = r.ReplaceAggregatesGeneration(ctx, p.UUID, set.Generation, []string{agg})
	must(err)
	if _, err = r.ReplaceAggregatesGeneration(ctx, p.UUID, set.Generation, []string{}); err == nil {
		t.Fatal("stale aggregate generation accepted")
	}
	q.Groups[0].MemberOf = [][]string{{agg}}
	out, err = svc.Candidates(ctx, identity, q)
	must(err)
	if len(out.Requests) != 1 {
		t.Fatal("persisted aggregate filter", out)
	}
	q.Groups[0].MemberOf = [][]string{{uuid.NewString()}}
	out, err = svc.Candidates(ctx, identity, q)
	must(err)
	if len(out.Requests) != 0 {
		t.Fatal("unknown aggregate filter ignored", out)
	}
	q.Groups[0].MemberOf = nil

	c := uuid.NewString()
	update := model.AllocationUpdate{Allocations: map[string]map[string]int64{p.UUID: {"VCPU": 6}}, ProjectID: "project", UserID: "user"}
	must(r.ReplaceAllocations(ctx, c, update))
	q.Groups[0].Resources["VCPU"] = 3
	out, err = svc.Candidates(ctx, identity, q)
	must(err)
	if len(out.Requests) != 0 {
		t.Fatal("candidates ignored usage", out)
	}
	q.Groups[0].Resources["VCPU"] = 2
	out, err = svc.Candidates(ctx, identity, q)
	must(err)
	if len(out.Requests) != 1 || out.Summaries[p.UUID].Resources["VCPU"].Used != 6 {
		t.Fatal(out)
	}
	must(r.DeleteAllocations(ctx, c))
	// Seed a parity relation: even provider generation => usage 2, odd => 4.
	provider, err := r.Get(ctx, p.UUID)
	must(err)
	first := int64(2)
	if (provider.Generation+1)%2 != 0 {
		first = 4
	}
	update.Allocations[p.UUID]["VCPU"] = first
	must(r.ReplaceAllocations(ctx, c, update))
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			provider, err := r.Get(ctx, p.UUID)
			if err != nil {
				done <- err
				return
			}
			amount := int64(2)
			if (provider.Generation+1)%2 != 0 {
				amount = 4
			}
			update.Allocations[p.UUID]["VCPU"] = amount
			if err = r.ReplaceAllocations(ctx, c, update); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 40; i++ {
		state, err := r.CandidateState(ctx)
		must(err)
		if len(state.Providers) != 1 {
			t.Fatal(state)
		}
		item := state.Providers[0]
		expected := int64(2)
		if item.Provider.Generation%2 != 0 {
			expected = 4
		}
		if item.Usage["VCPU"] != expected {
			t.Fatal("candidate snapshot mixed generations and usage", item)
		}
	}
	must(<-done)
	must(r.DeleteAllocations(ctx, c))
	// Distinct persistent times ensure an individual class cannot inherit the
	// collection maximum (or interpret a class name as SQL).
	older := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	newer := older.Add(time.Hour)
	_, err = admin.ExecContext(ctx, "UPDATE inventories SET updated_at=? WHERE provider_uuid=? AND resource_class='VCPU'", older, p.UUID)
	must(err)
	_, err = admin.ExecContext(ctx, "UPDATE inventories SET updated_at=? WHERE provider_uuid=? AND resource_class='MEMORY_MB'", newer, p.UUID)
	must(err)
	one, err := r.InventorySnapshot(ctx, p.UUID, "VCPU")
	must(err)
	all, err := r.Inventories(ctx, p.UUID)
	must(err)
	if len(one.Inventories) != 1 || !one.LastModified.Equal(older) || !all.LastModified.Equal(newer) || one.Generation != all.Generation {
		t.Fatal("individual timestamp is not class-specific", one, all)
	}
	empty, err := r.InventorySnapshot(ctx, p.UUID, "VCPU' OR 1=1 --")
	must(err)
	if len(empty.Inventories) != 0 {
		t.Fatal("class filter not bound", empty)
	}
	must(r.Delete(ctx, p.UUID))
	t.Log("PASS live MySQL candidates reflect usage/traits, and repeatable-read snapshots preserve generation/usage under concurrent writers")
}
