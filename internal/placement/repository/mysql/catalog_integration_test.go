//go:build integration

package mysql_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository/mysql"
	"github.com/google/uuid"
)

// NEW EMPTY disposable schema, separate accounts, same explicit opt-in as B07.
// Deliberate generation overflow is confined to this schema; never the lab DB.
func TestB08MySQLCatalogs(t *testing.T) {
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
	var count int
	if err = admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count); err != nil || count != 0 {
		t.Fatal("NEW EMPTY schema required", err)
	}
	// Build the immutable B07 v2 schema, then upgrade a real canary to v3.
	for version, file := range []string{"000_history.sql", "001_providers.sql", "002_contract.sql"} {
		data, err := os.ReadFile("migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(data), ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err = admin.ExecContext(ctx, statement); err != nil {
					t.Fatal("v2 setup", err)
				}
			}
		}
		if version > 0 {
			if _, err = admin.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum,dirty) VALUES (?,?,FALSE)", version, fmt.Sprintf("%x", sha256.Sum256(data))); err != nil {
				t.Fatal(err)
			}
		}
	}
	r0 := mysql.New(admin, 5*time.Second)
	p, _ := model.NewProvider("b08-upgrade", "")
	if err = r0.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	inv := model.Inventory{Total: 8, MinUnit: 1, MaxUnit: 8, StepSize: 1, AllocationRatio: 1}
	before, err := r0.ReplaceInventories(ctx, p.UUID, 0, map[string]model.Inventory{"VCPU": inv})
	if err != nil {
		t.Fatal(err)
	}
	if mysql.VerifySchema(ctx, admin) == nil {
		t.Fatal("runtime accepted v2 after B08")
	}
	if err = mysql.Migrate(ctx, admin); err != nil {
		t.Fatal("v2 to v3", err)
	}
	if err = mysql.Migrate(ctx, admin); err != nil {
		t.Fatal("idempotent migration", err)
	}
	app, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_APP_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if err = mysql.CheckAccount(ctx, app, "serve"); err != nil {
		t.Fatal(err)
	}
	if err = mysql.VerifySchema(ctx, app); err != nil {
		t.Fatal(err)
	}
	r := mysql.New(app, 5*time.Second)
	after, err := r.Inventories(ctx, p.UUID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("upgrade lost inventory or generation", after, err)
	}
	var source struct {
		Classes []string `json:"resource_classes"`
		Traits  []string `json:"traits"`
	}
	data, err := os.ReadFile("../../../../tests/compatibility/fixtures/placement-standard-catalogs.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &source) != nil {
		t.Fatal("invalid standard snapshot")
	}
	classes, err := r.Classes(ctx)
	if err != nil || len(classes) != len(source.Classes) {
		t.Fatal("standard classes", len(classes), err)
	}
	traits, err := r.Traits(ctx, model.TraitFilter{})
	if err != nil || !reflect.DeepEqual(traits, source.Traits) {
		t.Fatal("standard traits differ", err)
	}
	for i, item := range classes {
		if item.Name != source.Classes[i] || !item.Standard {
			t.Fatal("class order/standard flag", item)
		}
	}
	if known, err := r.ResourceClassExists(ctx, "VCPU "); err != nil || known {
		t.Fatal("class comparison adopted padding", err)
	}
	created, err := r.CreateClass(ctx, "CUSTOM_B08_OLD", false)
	if err != nil || !created {
		t.Fatal(err)
	}
	if _, err = r.CreateClass(ctx, "CUSTOM_B08_OLD", false); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("duplicate class", err)
	}
	if created, err = r.CreateClass(ctx, "CUSTOM_B08_OLD", true); err != nil || created {
		t.Fatal("idempotent class", err)
	}
	state, err := r.ReplaceInventories(ctx, p.UUID, 1, map[string]model.Inventory{"CUSTOM_B08_OLD": inv})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.RenameClass(ctx, "CUSTOM_B08_OLD", "CUSTOM_B08_NEW"); err != nil {
		t.Fatal("class rename", err)
	}
	state2, err := r.Inventories(ctx, p.UUID)
	if err != nil || state2.Generation != state.Generation || state2.Inventories["CUSTOM_B08_NEW"] != inv {
		t.Fatal("rename lost inventory or changed generation", state2, err)
	}
	if err = r.DeleteClass(ctx, "CUSTOM_B08_NEW"); !errors.Is(err, repository.ErrInUse) {
		t.Fatal("deleted class in use", err)
	}
	if err = r.DeleteClass(ctx, "VCPU"); !errors.Is(err, repository.ErrStandard) {
		t.Fatal("deleted standard class", err)
	}
	if err = r.RenameClass(ctx, "VCPU", "CUSTOM_BAD"); !errors.Is(err, repository.ErrStandard) {
		t.Fatal("renamed standard", err)
	}
	if created, err = r.EnsureTrait(ctx, "CUSTOM_B08_TRAIT"); err != nil || !created {
		t.Fatal(err)
	}
	if created, err = r.EnsureTrait(ctx, "CUSTOM_B08_TRAIT"); err != nil || created {
		t.Fatal("idempotent trait", err)
	}
	set, err := r.ReplaceTraits(ctx, p.UUID, state2.Generation, []string{"OWNER_NOVA", "CUSTOM_B08_TRAIT"})
	if err != nil || set.Generation != state2.Generation+1 {
		t.Fatal(set, err)
	}
	same, err := r.ReplaceTraits(ctx, p.UUID, set.Generation, []string{"CUSTOM_B08_TRAIT", "OWNER_NOVA", "CUSTOM_B08_TRAIT"})
	if err != nil || !reflect.DeepEqual(same, set) {
		t.Fatal("unchanged trait set incremented", same, err)
	}
	if _, err = r.ReplaceTraits(ctx, p.UUID, set.Generation-1, set.Traits); !errors.Is(err, repository.ErrConcurrent) {
		t.Fatal("stale generation accepted for no-op", err)
	}
	if err = r.DeleteTrait(ctx, "CUSTOM_B08_TRAIT"); !errors.Is(err, repository.ErrInUse) {
		t.Fatal("deleted assigned trait", err)
	}
	if err = r.DeleteTrait(ctx, "OWNER_NOVA"); !errors.Is(err, repository.ErrStandard) {
		t.Fatal("deleted standard trait", err)
	}
	if _, err = r.ReplaceTraits(ctx, p.UUID, set.Generation, []string{"CUSTOM_NOT_THERE"}); err == nil {
		t.Fatal("unknown trait accepted")
	}
	current, err := r.ProviderTraits(ctx, p.UUID)
	if err != nil || !reflect.DeepEqual(current, set) {
		t.Fatal("unknown trait mutated association", current, err)
	}
	// Fail after child replacement, at the final generation increment.
	if _, err = admin.ExecContext(ctx, "UPDATE resource_providers SET generation=? WHERE uuid=?", int64(math.MaxInt64), p.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ReplaceTraits(ctx, p.UUID, math.MaxInt64, []string{"COMPUTE_STATUS_DISABLED"}); err == nil {
		t.Fatal("SQL overflow did not fail")
	}
	rolled, err := r.ProviderTraits(ctx, p.UUID)
	if err != nil || rolled.Generation != math.MaxInt64 || !reflect.DeepEqual(rolled.Traits, set.Traits) {
		t.Fatal("partial trait replacement survived SQL failure", rolled, err)
	}
	if _, err = admin.ExecContext(ctx, "UPDATE resource_providers SET generation=? WHERE uuid=?", set.Generation, p.UUID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := r.ReplaceTraits(ctx, p.UUID, set.Generation, []string{"COMPUTE_STATUS_DISABLED"})
		results <- err
	}()
	go func() {
		<-start
		_, err := r.ReplaceInventories(ctx, p.UUID, set.Generation, map[string]model.Inventory{"CUSTOM_B08_NEW": inv})
		results <- err
	}()
	close(start)
	one, two := <-results, <-results
	if !((one == nil && errors.Is(two, repository.ErrConcurrent)) || (two == nil && errors.Is(one, repository.ErrConcurrent))) {
		t.Fatal("mixed mutation CAS", one, two)
	}
	provider, err := r.Get(ctx, p.UUID)
	if err != nil || provider.Generation != set.Generation+1 {
		t.Fatal(provider, err)
	}
	agg := uuid.NewString()
	if _, err = r.ReplaceAggregates(ctx, p.UUID, []string{agg}); err != nil {
		t.Fatal(err)
	}
	provider2, err := r.Get(ctx, p.UUID)
	if err != nil || provider2.Generation != provider.Generation {
		t.Fatal("aggregate changed generation", provider2, err)
	}
	filters := model.Filter{Name: &p.Name, Aggregates: []string{agg}, Resources: map[string]int64{"CUSTOM_B08_NEW": 4}}
	values, err := r.List(ctx, filters)
	if err != nil || len(values) != 1 {
		t.Fatal("member_of/resources", values, err)
	}
	filters.Resources["CUSTOM_B08_NEW"] = 9
	values, err = r.List(ctx, filters)
	if err != nil || len(values) != 0 {
		t.Fatal("exhausted filter", values, err)
	}
	if err = r.DeleteInventories(ctx, p.UUID); err != nil {
		t.Fatal(err)
	}
	empty, err := r.Inventories(ctx, p.UUID)
	if err != nil || len(empty.Inventories) != 0 || empty.Generation != provider.Generation+1 {
		t.Fatal("delete inventories", empty, err)
	}
	if err = r.DeleteClass(ctx, "CUSTOM_B08_NEW"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.ReplaceTraits(ctx, p.UUID, empty.Generation, []string{}); err != nil {
		t.Fatal(err)
	}
	if err = r.DeleteTrait(ctx, "CUSTOM_B08_TRAIT"); err != nil {
		t.Fatal(err)
	}
	if err = r.Delete(ctx, p.UUID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"resource_provider_traits", "resource_provider_aggregates"} {
		if err = app.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE provider_uuid=?", p.UUID).Scan(&count); err != nil || count != 0 {
			t.Fatal("orphan association", table, count, err)
		}
	}
	t.Log("PASS v2→v3 preservation, standard/custom catalogues, rename cascade, protected deletes, no-op generation, rollback, mixed CAS and filters; isolated MySQL")
}
