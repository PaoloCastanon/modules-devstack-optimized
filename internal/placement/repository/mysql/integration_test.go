//go:build integration

package mysql_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/api"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository/mysql"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
)

func secret(t *testing.T, key string) string {
	t.Helper()
	b, err := config.ReadSecret(os.Getenv(key))
	if err != nil {
		t.Fatal(err)
	}
	dsn := strings.TrimSpace(string(b))
	if err = mysql.ValidateTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	return dsn
}
func TestB07MySQLHTTP(t *testing.T) {
	if os.Getenv("PLACEMENT_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable MySQL opt-in required")
	}
	migrator, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var count int
	if err = migrator.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&count); err != nil || count != 0 {
		t.Fatal("test requires a NEW EMPTY schema", err)
	}
	if err = mysql.Migrate(ctx, migrator); err != nil {
		t.Fatal(err)
	}
	if err = mysql.Migrate(ctx, migrator); err != nil {
		t.Fatal("migration not idempotent", err)
	}
	if err = mysql.CheckAccount(ctx, migrator, "serve"); err == nil {
		t.Fatal("DDL account accepted for HTTP")
	}
	appDSN := secret(t, "PLACEMENT_GO_TEST_APP_DSN_FILE")
	db, err := mysql.Open(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err = mysql.CheckAccount(ctx, db, "serve"); err != nil {
		t.Fatal(err)
	}
	if err = mysql.VerifySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	r := mysql.New(db, 5*time.Second)
	p, _ := model.NewProvider("b07-integration", "")
	if err = r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = r.Create(ctx, p); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("duplicate", err)
	}
	inv, _ := (model.InventoryInput{Total: func() *int64 { v := int64(8); return &v }()}).Resolve()
	initial, err := r.ReplaceInventories(ctx, p.UUID, 0, map[string]model.Inventory{"VCPU": inv})
	if err != nil || initial.Generation != 1 {
		t.Fatal(initial, err)
	}
	if _, err = r.ReplaceInventories(ctx, p.UUID, 0, map[string]model.Inventory{}); !errors.Is(err, repository.ErrConcurrent) {
		t.Fatal("stale generation", err)
	}
	// The first insertion succeeds; the second fails its FK. Parent generation,
	// deletion of old inventory and all child insertions must roll back together.
	if _, err = r.ReplaceInventories(ctx, p.UUID, 1, map[string]model.Inventory{"VCPU": inv, "ZZ_INVALID": inv}); !errors.Is(err, repository.ErrUnknownClass) {
		t.Fatal("child SQL failure", err)
	}
	after, err := r.Inventories(ctx, p.UUID)
	if err != nil || !reflect.DeepEqual(initial, after) {
		t.Fatal("partial SQL mutation", after, err)
	}
	ch := make(chan error, 2)
	for n := 0; n < 2; n++ {
		go func() {
			_, err := r.ReplaceInventories(ctx, p.UUID, 1, map[string]model.Inventory{"VCPU": inv})
			ch <- err
		}()
	}
	one, two := <-ch, <-ch
	if !((one == nil && errors.Is(two, repository.ErrConcurrent)) || (two == nil && errors.Is(one, repository.ErrConcurrent))) {
		t.Fatal("concurrent writers", one, two)
	}
	renamed, err := r.Rename(ctx, p.UUID, "renamed")
	if err != nil || renamed.Generation != 2 {
		t.Fatal("rename generation", renamed, err)
	}
	// Match the deployed utf8mb3_general_ci equality, including PAD SPACE.
	for _, name := range []string{"RENAMED", "renamed "} {
		values, err := r.List(ctx, model.Filter{Name: &name})
		if err != nil || len(values) != 1 || values[0].UUID != p.UUID {
			t.Fatal("name collation", name, err)
		}
	}
	duplicate, _ := model.NewProvider("RENAMED", "")
	if err = r.Create(ctx, duplicate); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("case-insensitive uniqueness", err)
	}
	oldID := p.UUID
	p, _ = model.NewProvider("other", "")
	if err = r.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Rename(ctx, p.UUID, "renamed"); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatal("duplicate rename", err)
	}
	name := "renamed"
	listed, err := r.List(ctx, model.Filter{Name: &name})
	if err != nil || len(listed) != 1 || listed[0].UUID != oldID {
		t.Fatal("list", err)
	}
	// The Python driver reads MySQL FLOAT's text representation. Binary FLOAT
	// retrieval differs; the adapter must preserve the observed API value exactly.
	precise := inv
	precise.AllocationRatio = 1.23456789
	if _, err = r.ReplaceInventories(ctx, p.UUID, 0, map[string]model.Inventory{"VCPU": precise}); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := r.Inventories(ctx, p.UUID)
	if err != nil || roundTrip.Inventories["VCPU"].AllocationRatio != 1.23457 {
		t.Fatal("FLOAT text precision", roundTrip, err)
	}
	// New pool, same persistent database. This is not a MySQL server restart.
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = mysql.Open(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	r = mysql.New(db, 5*time.Second)
	if got, err := r.Get(ctx, oldID); err != nil || got.Generation != 2 {
		t.Fatal("pool reopen persistence", got, err)
	}
	if err = r.Delete(ctx, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Inventories(ctx, oldID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("delete", err)
	}
	if err = r.Delete(ctx, oldID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("repeat delete", err)
	}
	var orphans int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventories WHERE provider_uuid=?", oldID).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatal("orphan inventories", err)
	}

	// Real HTTP + production Keystone adapter + real MySQL. Keystone below is a
	// local protocol fixture; this does NOT validate a deployed identity service.
	keystone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		role := "service"
		if q.Method == "POST" {
			w.Header().Set("X-Subject-Token", "validator-service-token")
			w.WriteHeader(201)
		} else {
			if q.Header.Get("X-Auth-Token") != "validator-service-token" {
				w.WriteHeader(401)
				return
			}
			switch q.Header.Get("X-Subject-Token") {
			case "nova-token":
			case "reader-token":
				role = "reader"
			default:
				w.WriteHeader(404)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{"user": map[string]string{"id": "nova"}, "project": map[string]string{"id": "service"}, "roles": []map[string]string{{"name": role}}, "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}})
	}))
	defer keystone.Close()
	logs := &bytes.Buffer{}
	handler, err := api.New(service.New(r), auth.NewKeystone(keystone.URL+"/v3", keystone.Client(), config.ServiceCredential{ID: "fixture-id", Secret: "fixture-secret"}), "http://localhost/placement", keystone.URL+"/v3", 1048576, slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	call := func(method, path, token, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Auth-Token", token)
		req.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		b, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, b
	}
	if status, _ := call("GET", "/resource_providers", "invalid-token", ""); status != 401 {
		t.Fatal(status)
	}
	if status, _ := call("GET", "/resource_providers", "reader-token", ""); status != 403 {
		t.Fatal(status)
	}
	id := "4446c55f-ee69-4f16-b6c0-b562d5f200ab"
	path := "/resource_providers/" + id
	if status, b := call("POST", "/resource_providers", "nova-token", `{"uuid":"`+id+`","name":"http-b07"}`); status != 201 || len(b) != 0 {
		t.Fatal(status, string(b))
	}
	if status, b := call("PUT", path+"/inventories", "nova-token", `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":2}}}`); status != 200 || !bytes.Contains(b, []byte(`"resource_provider_generation":1`)) {
		t.Fatal(status, string(b))
	}
	if status, _ := call("GET", path+"/inventories/VCPU", "nova-token", ""); status != 200 {
		t.Fatal(status)
	}
	beforeRejected, err := r.Inventories(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"VCPU", "MEMORY_MB"} {
		payload := fmt.Sprintf(`{"resource_provider_generation":1,"inventories":{"%s":{"total":4,"reserved":4}}}`, class)
		status, body := call("PUT", path+"/inventories", "nova-token", payload)
		var failure struct {
			Errors []struct {
				Status        int
				Title, Detail string
			}
		}
		want := fmt.Sprintf("The server could not comply with the request since it is either malformed or otherwise incorrect.\n\n Unable to update inventory for resource provider %s: Invalid inventory for '%s' on resource provider '%s'. The reserved value is greater than or equal to total.  ", id, class, id)
		if status != 400 || json.Unmarshal(body, &failure) != nil || len(failure.Errors) != 1 || failure.Errors[0].Status != 400 || failure.Errors[0].Title != "Bad Request" || failure.Errors[0].Detail != want {
			t.Fatal("capacity error contract", status, string(body))
		}
		afterRejected, err := r.Inventories(ctx, id)
		if err != nil || !reflect.DeepEqual(beforeRejected, afterRejected) {
			t.Fatal("capacity rejection changed persisted state", afterRejected, err)
		}
	}
	if status, _ := call("DELETE", path, "nova-token", ""); status != 204 {
		t.Fatal(status)
	}
	if strings.Contains(logs.String(), "nova-token") || strings.Contains(logs.String(), "fixture-secret") {
		t.Fatal("secret leaked")
	}
	// Unknown/dirty migration state must stop serving; no schema repair here.
	if _, err = migrator.ExecContext(ctx, "UPDATE schema_migrations SET dirty=TRUE WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	if err = mysql.VerifySchema(ctx, db); err == nil {
		t.Fatal("dirty schema verified")
	}
	if err = mysql.Migrate(ctx, migrator); err == nil {
		t.Fatal("dirty schema automatically cleared")
	}
	t.Log("PASS real MySQL 8.0.46; Keystone is an HTTP fixture; dirty disposable schema retained")
}

// A separate NEW EMPTY schema is required. v1 can contain names that collide
// under the measured collation; upgrading must stop dirty without deleting data.
func TestV1CollisionStopsUpgrade(t *testing.T) {
	if os.Getenv("PLACEMENT_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable MySQL opt-in required")
	}
	db, err := mysql.Open(secret(t, "PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = mysql.CheckAccount(ctx, db, "migrate"); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("NEW EMPTY schema required", err)
	}
	history, err := os.ReadFile("migrations/000_history.sql")
	if err != nil {
		t.Fatal(err)
	}
	v1, err := os.ReadFile("migrations/001_providers.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{string(history), string(v1)} {
		for _, statement := range strings.Split(source, ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err = db.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(v1))
	if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations (version,checksum,dirty) VALUES (1,?,FALSE)", hash); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"UpgradeName", "upgradename"} {
		p, _ := model.NewProvider(name, "")
		if err = mysql.New(db, time.Second).Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err = mysql.VerifySchema(ctx, db); err == nil {
		t.Fatal("serve accepted schema v1 without required v2")
	}
	if err = mysql.Migrate(ctx, db); err == nil {
		t.Fatal("conflicting names silently migrated")
	}
	var count int
	var dirty bool
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_providers").Scan(&count); err != nil || count != 2 {
		t.Fatal("existing data changed", count, err)
	}
	if err = db.QueryRowContext(ctx, "SELECT dirty FROM schema_migrations WHERE version=2").Scan(&dirty); err != nil || !dirty {
		t.Fatal("failed v2 did not remain dirty", err)
	}
	if err = mysql.Migrate(ctx, db); err == nil {
		t.Fatal("dirty migration was automatically repaired")
	}
	t.Log("PASS v1 preserved; colliding v2 upgrade remains dirty without deleting either provider")
}
