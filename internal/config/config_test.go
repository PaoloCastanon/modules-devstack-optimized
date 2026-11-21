package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func secretFile(t *testing.T, value string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(p, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// glanceConf is a DevStack-like glance-api.conf; extra lines go in [DEFAULT].
func glanceConf(extra string) string {
	return "[DEFAULT]\ntransport_url = rabbit://stackrabbit:s3cret@127.0.0.1:5672/\n" + extra +
		"[database]\nconnection = mysql+pymysql://root:example@127.0.0.1/glance?charset=utf8\n" +
		"[keystone_authtoken]\nauth_url = http://127.0.0.1:5000\nauth_type = password\nusername = glance\npassword = fixture\n" +
		"project_name = service\nuser_domain_name = Default\nproject_domain_name = Default\n" +
		"[oslo_messaging_notifications]\ndriver = noop\n[glance_store]\nfilesystem_store_datadir = /opt/stack/data/glance/images/\n"
}

func environment(t *testing.T) map[string]string {
	return map[string]string{"GLANCE_GO_CONFIG_FILE": secretFile(t, glanceConf(""))}
}

func TestConfigDefaultsAndModes(t *testing.T) {
	env := environment(t)
	c, err := Load(func(k string) string { return env[k] }, "serve")
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddress != "127.0.0.1:9293" || c.PublicURL != "http://127.0.0.1:9293" || c.MaxBodyBytes != 1048576 || c.DependencyTimeout != 5*time.Second ||
		c.Credential.Username != "glance" || c.KeystoneURL != "http://127.0.0.1:5000/v3" || c.OfficialCatalog != "glance" {
		t.Fatal("unexpected defaults", c.Credential.Username, c.KeystoneURL)
	}
	if _, err = Load(func(k string) string { return env[k] }, "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(func(string) string { return "" }, "serve"); err == nil {
		t.Fatal("Glance Go started without the original's glance-api.conf")
	}
}

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"GLANCE_GO_LISTEN_ADDR", ":9293"}, {"GLANCE_GO_LISTEN_ADDR", "127.0.0.1:0"},
		{"GLANCE_GO_PUBLIC_BASE_URL", "http://user:password@localhost"}, {"GLANCE_GO_PUBLIC_BASE_URL", "http://localhost/?x=1"},
		{"GLANCE_GO_PUBLIC_BASE_URL", "/relative"}, {"GLANCE_GO_PUBLIC_BASE_URL", "http://localhost/a/../b"},
		{"GLANCE_GO_KEYSTONE_AUTH_URL", "http://localhost/identity"}, {"GLANCE_GO_MAX_BODY_BYTES", "0"},
		{"GLANCE_GO_REQUEST_TIMEOUT", "-1s"}, {"GLANCE_GO_DEPENDENCY_TIMEOUT", "20s"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			env := environment(t)
			env[tc.key] = tc.value
			_, err := Load(func(k string) string { return env[k] }, "serve")
			if err == nil {
				t.Fatal("accepted invalid configuration")
			}
			if strings.Contains(err.Error(), "password@") {
				t.Fatal("credential leaked")
			}
		})
	}
	if _, err := Load(func(string) string { return "" }, "unknown"); err == nil {
		t.Fatal("unknown mode")
	}
}

func TestPrivateFiles(t *testing.T) {
	p := secretFile(t, "a-private-value")
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(p); err == nil {
		t.Fatal("accepted public secret")
	}
	if _, err := ReadSecret(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	env := environment(t)
	env["GLANCE_GO_KEYSTONE_CREDENTIAL_FILE"] = secretFile(t, `{"application_credential_secret":"NEVER-LOG-THIS"`)
	_, err := Load(func(k string) string { return env[k] }, "serve")
	if err == nil || strings.Contains(err.Error(), "NEVER-LOG-THIS") {
		t.Fatal("unsafe credential error")
	}
}

func TestURLSeparatorsAndBasePaths(t *testing.T) {
	for _, value := range []string{
		"http://example.invalid?", "http://example.invalid/#", "http://example.invalid?x=1", "http://example.invalid/#frag",
		"http://example.invalid/?/", "http://example.invalid/#/", "http://example.invalid/a//b", "http://example.invalid/a/./b",
		"http://example.invalid/%2e%2e/b", "http://example.invalid/%3f", "http://example.invalid:0", "http://example.invalid:65536",
		"http://example.invalid:", "ftp://example.invalid", "https:///image", "http://bad!host", "http://[:::]", "http://user:secret@example.invalid",
	} {
		t.Run(value, func(t *testing.T) {
			env := environment(t)
			env["GLANCE_GO_PUBLIC_BASE_URL"] = value
			if _, err := Load(func(k string) string { return env[k] }, "serve"); err == nil {
				t.Fatal("ambiguous public URL accepted")
			}
		})
	}
	for _, value := range []string{"http://127.0.0.1:9293", "https://images.example.invalid/image/", "https://images.example.invalid/image///", "http://[::1]:9293/"} {
		t.Run(value, func(t *testing.T) {
			env := environment(t)
			env["GLANCE_GO_PUBLIC_BASE_URL"] = value
			c, err := Load(func(k string) string { return env[k] }, "serve")
			if err != nil || c.PublicURL != strings.TrimRight(value, "/") {
				t.Fatalf("base URL: %v", err)
			}
		})
	}
}

func TestPlacementNamespaceIsIndependent(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "placement.dsn")
	if err := os.WriteFile(dsn, []byte("placement_go_app:fixture@tcp(127.0.0.1:1)/placement_go"), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PLACEMENT_GO_DATABASE_DSN_FILE": dsn, "GLANCE_GO_LISTEN_ADDR": "0.0.0.0:9292", "GLANCE_GO_DATABASE_DSN_FILE": "/not-used"}
	c, err := LoadPlacement(func(k string) string { return env[k] }, "migrate")
	if err != nil || c.ListenAddress != "127.0.0.1:8779" || c.PublicURL != "http://127.0.0.1:8779" {
		t.Fatal(c.ListenAddress, err)
	}
	env["PLACEMENT_GO_LISTEN_ADDR"] = "127.0.0.1:18779"
	env["PLACEMENT_GO_PUBLIC_BASE_URL"] = "http://localhost/placement"
	c, err = LoadPlacement(func(k string) string { return env[k] }, "migrate")
	if err != nil || c.ListenAddress != "127.0.0.1:18779" || c.PublicURL != "http://localhost/placement" {
		t.Fatal(err)
	}
	delete(env, "PLACEMENT_GO_DATABASE_DSN_FILE")
	if _, err = LoadPlacement(func(k string) string { return env[k] }, "migrate"); err == nil {
		t.Fatal("Glance secret used for Placement")
	}
}

// keystonemiddleware's token_cache_time: 300 s by default, -1 disables.
func TestGlanceTokenCacheTime(t *testing.T) {
	e := environment(t)
	c, err := Load(func(k string) string { return e[k] }, "serve")
	if err != nil || c.TokenCacheTime != 300*time.Second {
		t.Fatal("default token cache time", c.TokenCacheTime, err)
	}
	conf := func(value string) map[string]string {
		text := strings.Replace(glanceConf(""), "auth_type = password\n", "auth_type = password\ntoken_cache_time = "+value+"\n", 1)
		return map[string]string{"GLANCE_GO_CONFIG_FILE": secretFile(t, text)}
	}
	for value, want := range map[string]time.Duration{"-1": 0, "1": time.Second, "600": 10 * time.Minute} {
		e = conf(value)
		if c, err = Load(func(k string) string { return e[k] }, "serve"); err != nil || c.TokenCacheTime != want {
			t.Fatal(value, c.TokenCacheTime, err)
		}
	}
	for _, bad := range []string{"-2", "86401", "5m", "x"} {
		e = conf(bad)
		if _, err = Load(func(k string) string { return e[k] }, "serve"); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	e = environment(t)
	e["GLANCE_GO_TOKEN_CACHE_TIME"] = "10"
	if _, err = Load(func(k string) string { return e[k] }, "serve"); err == nil {
		t.Fatal("an environment override competed with glance-api.conf")
	}
}

func placementEnvironment(t *testing.T) map[string]string {
	return map[string]string{"PLACEMENT_GO_DATABASE_DSN_FILE": secretFile(t, "placement_go_app:example@tcp(127.0.0.1:3306)/placement_go"), "PLACEMENT_GO_KEYSTONE_AUTH_URL": "http://127.0.0.1:5000/v3", "PLACEMENT_GO_KEYSTONE_CREDENTIAL_FILE": secretFile(t, `{"application_credential_id":"fixture-id","application_credential_secret":"fixture-secret"}`)}
}

// Official Placement runs keystonemiddleware with token_cache_time 300 s.
func TestPlacementTokenCacheTime(t *testing.T) {
	e := placementEnvironment(t)
	c, err := LoadPlacement(func(k string) string { return e[k] }, "serve")
	if err != nil || c.TokenCacheTime != 300*time.Second {
		t.Fatal("default token cache time", c.TokenCacheTime, err)
	}
	for value, want := range map[string]time.Duration{"-1": 0, "1": time.Second, "600": 10 * time.Minute} {
		e["PLACEMENT_GO_TOKEN_CACHE_TIME"] = value
		if c, err = LoadPlacement(func(k string) string { return e[k] }, "serve"); err != nil || c.TokenCacheTime != want {
			t.Fatal(value, c.TokenCacheTime, err)
		}
	}
	for _, bad := range []string{"0", "-2", "86401", "5m", "x"} {
		e["PLACEMENT_GO_TOKEN_CACHE_TIME"] = bad
		if _, err = LoadPlacement(func(k string) string { return e[k] }, "serve"); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	// The Glance variable must not configure Placement.
	delete(e, "PLACEMENT_GO_TOKEN_CACHE_TIME")
	e["GLANCE_GO_TOKEN_CACHE_TIME"] = "-1"
	if c, err = LoadPlacement(func(k string) string { return e[k] }, "serve"); err != nil || c.TokenCacheTime != 300*time.Second {
		t.Fatal("Glance namespace leaked into Placement", c.TokenCacheTime, err)
	}
}

func TestPlacementDatabasePoolSize(t *testing.T) {
	e := placementEnvironment(t)
	c, err := LoadPlacement(func(k string) string { return e[k] }, "serve")
	if err != nil || c.DBMaxConnections != 16 {
		t.Fatal("default pool size", c.DBMaxConnections, err)
	}
	for value, want := range map[string]int{"1": 1, "32": 32, "256": 256} {
		e["PLACEMENT_GO_DB_MAX_CONNECTIONS"] = value
		if c, err = LoadPlacement(func(k string) string { return e[k] }, "serve"); err != nil || c.DBMaxConnections != want {
			t.Fatal(value, c.DBMaxConnections, err)
		}
	}
	for _, bad := range []string{"0", "-1", "257", "x", "1.5"} {
		e["PLACEMENT_GO_DB_MAX_CONNECTIONS"] = bad
		if _, err = LoadPlacement(func(k string) string { return e[k] }, "serve"); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	// Glance keeps its own fixed pool; the setting is Placement only.
	g := environment(t)
	g["GLANCE_GO_DB_MAX_CONNECTIONS"] = "8"
	if c, err = Load(func(k string) string { return g[k] }, "serve"); err != nil || c.DBMaxConnections != 0 {
		t.Fatal("pool setting leaked into Glance", c.DBMaxConnections, err)
	}
}
