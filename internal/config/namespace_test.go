package config

import (
	"strings"
	"testing"
	"time"
)

func TestPlacementIgnoresGlanceBinaryConfiguration(t *testing.T) {
	for _, mode := range []string{"serve", "migrate"} {
		t.Run(mode, func(t *testing.T) {
			dsn := "placement_go_app:fixture@tcp(127.0.0.1:1)/placement_go"
			env := map[string]string{
				"PLACEMENT_GO_DATABASE_DSN_FILE":        secretFile(t, dsn),
				"PLACEMENT_GO_KEYSTONE_AUTH_URL":        "http://127.0.0.1:5000/v3",
				"PLACEMENT_GO_KEYSTONE_CREDENTIAL_FILE": secretFile(t, `{"application_credential_id":"placement-id","application_credential_secret":"placement-fixture"}`),
				"GLANCE_GO_DATABASE_DSN_FILE":           "/not-used",
				"GLANCE_GO_BINARY_ENABLED":              "true",
				"GLANCE_GO_STORE_ROOT":                  "invalid-relative-root",
				"GLANCE_GO_STORE_SCOPE":                 "invalid-scope",
				"GLANCE_GO_UPLOAD_TIMEOUT":              "0s",
				"GLANCE_GO_MAX_CONCURRENT_TRANSFERS":    "0",
			}
			c, err := LoadPlacement(func(key string) string {
				if !strings.HasPrefix(key, "PLACEMENT_GO_") {
					t.Errorf("Placement read another namespace: %s", key)
				}
				return env[key]
			}, mode)
			if err != nil {
				t.Fatal(err)
			}
			if c.DatabaseDSN != dsn || c.ListenAddress != "127.0.0.1:8779" || c.PublicURL != "http://127.0.0.1:8779" || c.DependencyTimeout != 5*time.Second {
				t.Fatal("Placement configuration changed")
			}
			if c.Binary != (Binary{}) {
				t.Fatal("Placement acquired Glance binary policy")
			}
			if mode == "serve" && c.Credential.ID != "placement-id" {
				t.Fatal("Placement did not load its own credential")
			}
		})
	}
}

func TestGlanceModesIgnorePlacementConfiguration(t *testing.T) {
	for _, mode := range []string{"serve", "migrate", "store-scope", "store-init"} {
		t.Run(mode, func(t *testing.T) {
			env := environment(t)
			env["GLANCE_GO_BINARY_ENABLED"] = "true"
			env["GLANCE_GO_STORE_ROOT"] = "/private/glance"
			env["GLANCE_GO_STORE_SCOPE"] = "00000000-0000-5000-8000-000000000001"
			env["PLACEMENT_GO_DATABASE_DSN_FILE"] = "/not-used"
			env["PLACEMENT_GO_LISTEN_ADDR"] = ":invalid"
			env["PLACEMENT_GO_KEYSTONE_AUTH_URL"] = "invalid"
			if mode != "serve" {
				delete(env, "GLANCE_GO_KEYSTONE_AUTH_URL")
				delete(env, "GLANCE_GO_KEYSTONE_CREDENTIAL_FILE")
			}
			c, err := Load(func(key string) string {
				if !strings.HasPrefix(key, "GLANCE_GO_") {
					t.Errorf("Glance read another namespace: %s", key)
				}
				return env[key]
			}, mode)
			if err != nil {
				t.Fatal(err)
			}
			if c.ListenAddress != "127.0.0.1:9293" || c.PublicURL != "http://127.0.0.1:9293" || !strings.Contains(c.DatabaseDSN, "/glance_go") {
				t.Fatal("Glance configuration changed")
			}
			if !c.Binary.Enabled || c.Binary.Root != env["GLANCE_GO_STORE_ROOT"] || c.Binary.Scope != env["GLANCE_GO_STORE_SCOPE"] || c.Binary.UploadTimeout != 10*time.Minute {
				t.Fatal("Glance lost its binary policy")
			}
		})
	}
}

func TestPlacementRejectsGlanceStoreModes(t *testing.T) {
	for _, mode := range []string{"store-scope", "store-init", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			_, err := LoadPlacement(func(key string) string {
				t.Errorf("unsupported Placement mode read configuration: %s", key)
				return ""
			}, mode)
			if err == nil || err.Error() != "command must be serve or migrate" {
				t.Fatalf("unsupported Placement mode accepted: %v", err)
			}
		})
	}
}
