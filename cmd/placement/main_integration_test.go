//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository/mysql"
)

// Uses another NEW EMPTY disposable schema; do not run on the repository suite's schema.
func TestBinaryMySQLLifecycle(t *testing.T) {
	if os.Getenv("PLACEMENT_GO_MYSQL_TEST") != "1" {
		t.Fatal("explicit disposable MySQL opt-in required")
	}
	for _, key := range []string{"PLACEMENT_GO_TEST_MIGRATE_DSN_FILE", "PLACEMENT_GO_TEST_APP_DSN_FILE"} {
		b, err := config.ReadSecret(os.Getenv(key))
		if err != nil {
			t.Fatal(err)
		}
		if err = mysql.ValidateTestDSN(strings.TrimSpace(string(b))); err != nil {
			t.Fatal(err)
		}
	}
	migrateSecret, err := config.ReadSecret(os.Getenv("PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := mysql.Open(strings.TrimSpace(string(migrateSecret)))
	if err != nil {
		t.Fatal(err)
	}
	inspectionCtx, inspectionCancel := context.WithTimeout(context.Background(), 5*time.Second)
	var tables int
	err = inspection.QueryRowContext(inspectionCtx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables)
	inspectionCancel()
	_ = inspection.Close()
	if err != nil || tables != 0 {
		t.Fatal("binary suite requires a NEW EMPTY disposable schema", err)
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "placement")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatal("binary build", err, string(output))
	}
	fakeIdentity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/auth/tokens" {
			w.WriteHeader(404)
			return
		}
		if r.Method == "POST" {
			w.Header().Set("X-Subject-Token", "validator-token")
			w.WriteHeader(201)
		} else if r.Header.Get("X-Subject-Token") != "fixture-nova" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{"user": map[string]string{"id": "nova"}, "project": map[string]string{"id": "service"}, "roles": []map[string]string{{"name": "service"}}, "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}})
	}))
	defer fakeIdentity.Close()
	credential := filepath.Join(dir, "credential.json")
	if err := os.WriteFile(credential, []byte(`{"application_credential_id":"fixture-id","application_credential_secret":"fixture-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	baseEnv := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PLACEMENT_GO_") {
			baseEnv = append(baseEnv, value)
		}
	}
	baseEnv = append(baseEnv, "PLACEMENT_GO_LISTEN_ADDR="+address, "PLACEMENT_GO_PUBLIC_BASE_URL=http://"+address, "PLACEMENT_GO_KEYSTONE_AUTH_URL="+fakeIdentity.URL+"/v3", "PLACEMENT_GO_KEYSTONE_CREDENTIAL_FILE="+credential)
	migrate := exec.Command(binary, "migrate")
	migrate.Env = append(baseEnv, "PLACEMENT_GO_DATABASE_DSN_FILE="+os.Getenv("PLACEMENT_GO_TEST_MIGRATE_DSN_FILE"))
	if _, err = migrate.CombinedOutput(); err != nil {
		t.Fatal("explicit binary migration failed", err)
	}
	logs, err := os.Create(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	server := exec.Command(binary, "serve")
	server.Env = append(baseEnv, "PLACEMENT_GO_DATABASE_DSN_FILE="+os.Getenv("PLACEMENT_GO_TEST_APP_DSN_FILE"))
	server.Stdout = logs
	server.Stderr = logs
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	defer func() { _ = server.Process.Kill() }()
	client := &http.Client{Timeout: time.Second}
	base := "http://" + address
	deadline := time.Now().Add(8 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("binary listener did not become ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "POST", base+"/resource_providers", strings.NewReader(`{"name":"binary-lifecycle"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Auth-Token", "fixture-nova")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal("binary authenticated create", response.StatusCode)
	}
	if err = server.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal("graceful shutdown failed", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("shutdown did not complete")
	}
	if _, err = client.Get(base + "/"); err == nil {
		t.Fatal("listener survived shutdown")
	}
	// Restart the process against the same DB; it must preserve the created provider.
	server = exec.Command(binary, "serve")
	server.Env = append(baseEnv, "PLACEMENT_GO_DATABASE_DSN_FILE="+os.Getenv("PLACEMENT_GO_TEST_APP_DSN_FILE"))
	server.Stdout = logs
	server.Stderr = logs
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	restart := server
	defer func() { _ = restart.Process.Kill() }()
	done = make(chan error, 1)
	go func() { done <- restart.Wait() }()
	ready = false
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("restart did not become ready")
	}
	restartCtx, restartCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer restartCancel()
	request, err = http.NewRequestWithContext(restartCtx, "GET", base+"/resource_providers?name=binary-lifecycle", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Auth-Token", "fixture-nova")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(data), `"name":"binary-lifecycle"`) {
		t.Fatal("process restart persistence", err)
	}
	_ = server.Process.Signal(syscall.SIGTERM)
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("restart shutdown timed out")
	}
	data, err = os.ReadFile(logs.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-nova", "fixture-secret", "application_credential_secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("secret leaked to executable logs")
		}
	}
	t.Log("PASS executable migrate, authenticated create, SIGTERM, restart and persistence; MySQL real, Keystone fixture")
}
