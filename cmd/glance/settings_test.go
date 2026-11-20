package main

import (
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
)

func TestEffectiveSettingsReportsRealLimits(t *testing.T) {
	c := config.Config{TokenCacheTime: 300 * time.Second, RequestTimeout: 10 * time.Second}
	c.Binary.MaxConcurrent, c.Binary.MaxDownloadStreams = 16, 2
	got := map[string]any{}
	kv := effectiveSettings(c)
	if len(kv)%2 != 0 {
		t.Fatal("odd key/value list")
	}
	for i := 0; i < len(kv); i += 2 {
		got[kv[i].(string)] = kv[i+1]
	}
	if got["sql_pool_open"] != 55 || got["sql_pool_idle"] != 16 || got["token_cache_s"] != 300.0 ||
		got["max_concurrent_transfers"] != 16 || got["max_download_streams"] != 2 {
		t.Fatalf("unexpected settings: %v", got)
	}
	for k := range got {
		if k == "database_dsn" || k == "credential" || k == "transport_url" {
			t.Fatalf("secret-bearing key %s exposed", k)
		}
	}
}
