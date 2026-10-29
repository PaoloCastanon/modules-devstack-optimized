//go:build integration

package mysql_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository/mysql"
	"github.com/google/uuid"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestB09MySQLAllocations(t *testing.T) {
	if os.Getenv("PLACEMENT_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable MySQL opt-in required")
	}
	admin, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	must(admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count))
	if count != 0 {
		t.Fatal("NEW EMPTY schema required")
	}
	for version, file := range []string{"000_history.sql", "001_providers.sql", "002_contract.sql", "003_catalogs.sql"} {
		data, err := os.ReadFile("migrations/" + file)
		must(err)
		for _, statement := range strings.Split(string(data), ";") {
			if strings.TrimSpace(statement) != "" {
				_, err = admin.ExecContext(ctx, statement)
				must(err)
			}
		}
		if version > 0 {
			_, err = admin.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum,dirty) VALUES (?,?,FALSE)", version, fmt.Sprintf("%x", sha256.Sum256(data)))
			must(err)
		}
	}
	old := mysql.New(admin, 5*time.Second)
	p, _ := model.NewProvider("b09-canary", "")
	must(old.Create(ctx, p))
	_, err = admin.ExecContext(ctx, "INSERT INTO inventories(provider_uuid,resource_class,total,reserved,min_unit,max_unit,step_size,allocation_ratio) VALUES (?,'VCPU',8,0,1,8,1,1)", p.UUID)
	must(err)
	_, err = admin.ExecContext(ctx, "UPDATE resource_providers SET generation=1 WHERE uuid=?", p.UUID)
	must(err)
	_, err = old.EnsureTrait(ctx, "CUSTOM_B09_UPGRADE")
	must(err)
	traits, err := old.ReplaceTraits(ctx, p.UUID, 1, []string{"CUSTOM_B09_UPGRADE", "OWNER_NOVA"})
	must(err)
	before, err := old.Inventories(ctx, p.UUID)
	must(err)
	if mysql.VerifySchema(ctx, admin) == nil {
		t.Fatal("new runtime accepted v3")
	}
	must(mysql.Migrate(ctx, admin))
	must(mysql.Migrate(ctx, admin))
	db, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_APP_DSN_FILE"))
	must(err)
	defer db.Close()
	must(mysql.CheckAccount(ctx, db, "serve"))
	must(mysql.VerifySchema(ctx, db))
	r := mysql.New(db, 5*time.Second)
	after, err := r.Inventories(ctx, p.UUID)
	must(err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("v3 upgrade lost inventory", before, after)
	}
	gotTraits, err := r.ProviderTraits(ctx, p.UUID)
	must(err)
	if !reflect.DeepEqual(traits, gotTraits) {
		t.Fatal("v3 upgrade lost traits")
	}
	empty, err := r.ConsumerAllocations(ctx, uuid.NewString())
	must(err)
	if len(empty.Allocations) != 0 {
		t.Fatal(empty)
	}
	c1, c2 := uuid.NewString(), uuid.NewString()
	alloc := func(id string, n int64) model.AllocationUpdate {
		return model.AllocationUpdate{Allocations: map[string]map[string]int64{id: {"VCPU": n}}, ProjectID: "project", UserID: "user"}
	}
	must(r.ReplaceAllocations(ctx, c1, alloc(p.UUID, 6)))
	first, err := r.ConsumerAllocations(ctx, c1)
	must(err)
	if first.Allocations[p.UUID].Generation != before.Generation+1 || first.Allocations[p.UUID].Resources["VCPU"] != 6 {
		t.Fatal(first)
	}
	must(r.ReplaceAllocations(ctx, c1, alloc(p.UUID, 8))) // exclude this consumer's old 6
	usage, err := r.Usages(ctx, p.UUID)
	must(err)
	if usage.Usages["VCPU"] != 8 {
		t.Fatal(usage)
	}
	if err = r.ReplaceAllocations(ctx, c2, alloc(p.UUID, 1)); model.Status(err) != 409 {
		t.Fatal("oversold", err)
	}
	must(admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM consumers WHERE uuid=?", c2).Scan(&count))
	if count != 0 {
		t.Fatal("failed claim left consumer")
	}
	f := model.Filter{UUID: &p.UUID, Resources: map[string]int64{"VCPU": 1}}
	list, err := r.List(ctx, f)
	must(err)
	if len(list) != 0 {
		t.Fatal("resources ignored usage")
	}
	if err = r.Delete(ctx, p.UUID); !errors.Is(err, repository.ErrProviderInUse) {
		t.Fatal("provider removed while allocated", err)
	}
	if err = r.DeleteInventories(ctx, p.UUID); err == nil {
		t.Fatal("inventory removed while allocated")
	}
	provider, err := r.Get(ctx, p.UUID)
	must(err)
	smaller := before.Inventories["VCPU"]
	smaller.Total = 4
	_, err = r.ReplaceInventories(ctx, p.UUID, provider.Generation, map[string]model.Inventory{"VCPU": smaller})
	must(err)
	must(r.ReplaceAllocations(ctx, c1, alloc(p.UUID, 6))) // reduce existing overage
	if err = r.ReplaceAllocations(ctx, c1, alloc(p.UUID, 7)); model.Status(err) != 409 {
		t.Fatal("worsened overage", err)
	}
	provider, err = r.Get(ctx, p.UUID)
	must(err)
	beforeDelete := provider.Generation
	must(r.DeleteAllocations(ctx, c1))
	provider, err = r.Get(ctx, p.UUID)
	must(err)
	if provider.Generation != beforeDelete {
		t.Fatal("DELETE incremented provider generation")
	}
	if err = r.DeleteAllocations(ctx, c1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("repeated DELETE", err)
	}
	// Two consumers compete for the same last units; exactly one may win.
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, consumer := range []string{c1, c2} {
		go func(c string) { <-start; results <- r.ReplaceAllocations(ctx, c, alloc(p.UUID, 4)) }(consumer)
	}
	close(start)
	one, two := <-results, <-results
	if !((one == nil && model.Status(two) == 409) || (two == nil && model.Status(one) == 409)) {
		t.Fatal("capacity race", one, two)
	}
	usage, err = r.Usages(ctx, p.UUID)
	must(err)
	if usage.Usages["VCPU"] != 4 {
		t.Fatal("capacity race usage", usage)
	}
	for _, c := range []string{c1, c2} {
		err = r.DeleteAllocations(ctx, c)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			t.Fatal(err)
		}
	}
	// Multi-provider replacement is all-or-nothing, including final SQL failure.
	p2, _ := model.NewProvider("b09-second", "")
	must(r.Create(ctx, p2))
	_, err = r.ReplaceInventories(ctx, p2.UUID, 0, map[string]model.Inventory{"VCPU": before.Inventories["VCPU"]})
	must(err)
	both := model.AllocationUpdate{Allocations: map[string]map[string]int64{p.UUID: {"VCPU": 2}, p2.UUID: {"VCPU": 2}}, ProjectID: "project", UserID: "user"}
	must(r.ReplaceAllocations(ctx, c1, both))
	snapshot, err := r.ConsumerAllocations(ctx, c1)
	must(err)
	both.Allocations[p2.UUID]["VCPU"] = 9
	if err = r.ReplaceAllocations(ctx, c1, both); model.Status(err) != 409 {
		t.Fatal("multi-provider invalid claim", err)
	}
	same, err := r.ConsumerAllocations(ctx, c1)
	must(err)
	if !reflect.DeepEqual(snapshot, same) {
		t.Fatal("partial multi-provider allocation", same)
	}
	both.Allocations[p2.UUID]["VCPU"] = 3
	_, err = admin.ExecContext(ctx, "UPDATE resource_providers SET generation=? WHERE uuid=?", int64(math.MaxInt64), p2.UUID)
	must(err)
	snapshot, err = r.ConsumerAllocations(ctx, c1)
	must(err)
	if err = r.ReplaceAllocations(ctx, c1, both); err == nil {
		t.Fatal("expected SQL generation overflow")
	}
	same, err = r.ConsumerAllocations(ctx, c1)
	must(err)
	if !reflect.DeepEqual(snapshot, same) {
		t.Fatal("SQL failure did not roll back", same)
	}
	_, err = admin.ExecContext(ctx, "UPDATE resource_providers SET generation=2 WHERE uuid=?", p2.UUID)
	must(err)
	// An error at the final consumer update rolls back providers, children and owner.
	_, err = admin.ExecContext(ctx, "UPDATE consumers SET generation=? WHERE uuid=?", int64(math.MaxInt64), c1)
	must(err)
	snapshot, err = r.ConsumerAllocations(ctx, c1)
	must(err)
	changed := both
	changed.ProjectID = "changed-project"
	changed.UserID = "changed-user"
	if err = r.ReplaceAllocations(ctx, c1, changed); err == nil {
		t.Fatal("consumer overflow did not fail")
	}
	same, err = r.ConsumerAllocations(ctx, c1)
	must(err)
	if !reflect.DeepEqual(snapshot, same) {
		t.Fatal("consumer update failed to roll back providers", same)
	}
	var project, user string
	var generation int64
	must(admin.QueryRowContext(ctx, "SELECT project_id,user_id,generation FROM consumers WHERE uuid=?", c1).Scan(&project, &user, &generation))
	if project != "project" || user != "user" || generation != math.MaxInt64 {
		t.Fatal("partial consumer update", project, user, generation)
	}
	_, err = admin.ExecContext(ctx, "UPDATE consumers SET generation=1 WHERE uuid=?", c1)
	must(err)

	// Concurrent replacements of one consumer must not leave the union of both.
	start = make(chan struct{})
	results = make(chan error, 2)
	go func() { <-start; results <- r.ReplaceAllocations(ctx, c1, alloc(p.UUID, 2)) }()
	go func() { <-start; results <- r.ReplaceAllocations(ctx, c1, alloc(p2.UUID, 3)) }()
	close(start)
	must(<-results)
	must(<-results)
	final, err := r.ConsumerAllocations(ctx, c1)
	must(err)
	if len(final.Allocations) != 1 {
		t.Fatal("same consumer mixed states", final)
	}
	// A fresh pool reads exactly the committed allocation state.
	pool, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_APP_DSN_FILE"))
	must(err)
	persisted, err := mysql.New(pool, 5*time.Second).ConsumerAllocations(ctx, c1)
	must(err)
	must(pool.Close())
	if !reflect.DeepEqual(final, persisted) {
		t.Fatal("new pool lost allocations")
	}
	must(r.DeleteAllocations(ctx, c1))
	// Two different consumers claim both providers; the loser must leave neither.
	start = make(chan struct{})
	results = make(chan error, 2)
	pair := model.AllocationUpdate{Allocations: map[string]map[string]int64{p.UUID: {"VCPU": 4}, p2.UUID: {"VCPU": 4}}, ProjectID: "project", UserID: "user"}
	for _, c := range []string{c1, c2} {
		go func(id string) { <-start; results <- r.ReplaceAllocations(ctx, id, pair) }(c)
	}
	close(start)
	one, two = <-results, <-results
	if !((one == nil && model.Status(two) == 409) || (two == nil && model.Status(one) == 409)) {
		t.Fatal("multi-provider race", one, two)
	}
	for _, id := range []string{p.UUID, p2.UUID} {
		use, err := r.Usages(ctx, id)
		must(err)
		if use.Usages["VCPU"] != 4 {
			t.Fatal("partial multi-provider race", use)
		}
	}
	for _, c := range []string{c1, c2} {
		err = r.DeleteAllocations(ctx, c)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			t.Fatal(err)
		}
	}

	// Renaming an allocated CUSTOM class cascades without removing the claim.
	_, err = r.CreateClass(ctx, "CUSTOM_B09_OLD", false)
	must(err)
	provider, err = r.Get(ctx, p.UUID)
	must(err)
	_, err = r.ReplaceInventories(ctx, p.UUID, provider.Generation, map[string]model.Inventory{"CUSTOM_B09_OLD": before.Inventories["VCPU"]})
	must(err)
	must(r.ReplaceAllocations(ctx, c1, model.AllocationUpdate{Allocations: map[string]map[string]int64{p.UUID: {"CUSTOM_B09_OLD": 2}}, ProjectID: "project", UserID: "user"}))
	provider, err = r.Get(ctx, p.UUID)
	must(err)
	must(r.RenameClass(ctx, "CUSTOM_B09_OLD", "CUSTOM_B09_NEW"))
	renamed, err := r.ConsumerAllocations(ctx, c1)
	must(err)
	if renamed.Allocations[p.UUID].Resources["CUSTOM_B09_NEW"] != 2 || renamed.Allocations[p.UUID].Generation != provider.Generation {
		t.Fatal("class rename lost claim", renamed)
	}
	// Modern consumer generation: null means absent, zero is a real expectation.
	c3 := uuid.NewString()
	modern := model.AllocationUpdate{Allocations: map[string]map[string]int64{p.UUID: {"CUSTOM_B09_NEW": 2}}, ProjectID: "project", UserID: "user", CheckGeneration: true}
	must(r.ReplaceAllocations(ctx, c3, modern))
	current, err := r.ConsumerAllocations(ctx, c3)
	must(err)
	if current.Generation == nil || *current.Generation != 1 || current.ProjectID == nil || *current.ProjectID != "project" {
		t.Fatal("modern metadata", current)
	}
	stale := modern
	stale.ProjectID = "changed-owner"
	if err = r.ReplaceAllocations(ctx, c3, stale); model.Status(err) != 409 {
		t.Fatal("null accepted for existing consumer", err)
	}
	zero := int64(0)
	stale.Generation = &zero
	if err = r.ReplaceAllocations(ctx, c3, stale); model.Status(err) != 409 {
		t.Fatal("stale zero accepted", err)
	}
	unchanged, err := r.ConsumerAllocations(ctx, c3)
	must(err)
	if !reflect.DeepEqual(current, unchanged) {
		t.Fatal("generation conflict changed owner or state", unchanged)
	}
	gen := int64(1)
	modern.Generation = &gen
	must(r.ReplaceAllocations(ctx, c3, modern))
	gen = 2
	clear := modern
	clear.Allocations = map[string]map[string]int64{}
	providerBefore, err := r.Get(ctx, p.UUID)
	must(err)
	must(r.ReplaceAllocations(ctx, c3, clear))
	providerAfter, err := r.Get(ctx, p.UUID)
	must(err)
	if providerAfter.Generation != providerBefore.Generation+1 {
		t.Fatal("empty PUT did not increment old provider")
	}
	must(admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM consumers WHERE uuid=?", c3).Scan(&count))
	if count != 0 {
		t.Fatal("empty PUT left consumer")
	}
	modern.Generation = &zero
	if err = r.ReplaceAllocations(ctx, c3, modern); model.Status(err) != 409 {
		t.Fatal("zero accepted for absent consumer", err)
	}
	modern.Generation = nil
	start = make(chan struct{})
	results = make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- r.ReplaceAllocations(ctx, c3, modern) }()
	}
	close(start)
	one, two = <-results, <-results
	if !((one == nil && model.Status(two) == 409) || (two == nil && model.Status(one) == 409)) {
		t.Fatal("new consumer generation race", one, two)
	}
	must(r.DeleteAllocations(ctx, c3))

	must(r.DeleteAllocations(ctx, c1))
	must(r.Delete(ctx, p.UUID))
	must(r.Delete(ctx, p2.UUID))
	must(r.DeleteClass(ctx, "CUSTOM_B09_NEW"))
	must(r.DeleteTrait(ctx, "CUSTOM_B09_UPGRADE"))
	must(admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM consumers").Scan(&count))
	if count != 0 {
		t.Fatal("consumer leak", count)
	}
	t.Log("PASS v3→v4 preservation, allocation replacement/rollback, usage filters, protection, capacity race, same-consumer race, pool persistence and allocated-class cascade")
}
