package main

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository/mysql"
)

// effectiveSettings lists the non-secret limits this process actually uses,
// so benchmark evidence records them from the running binary, not by hand.
func effectiveSettings(c config.Config) []any {
	b := c.Binary
	return []any{
		"binary_enabled", b.Enabled, "store_backend", b.Backend,
		"max_concurrent_transfers", b.MaxConcurrent, "max_download_streams", b.MaxDownloadStreams,
		"max_upload_bytes", b.MaxUploadBytes,
		"upload_timeout_s", b.UploadTimeout.Seconds(), "download_timeout_s", b.DownloadTimeout.Seconds(),
		"request_timeout_s", c.RequestTimeout.Seconds(), "dependency_timeout_s", c.DependencyTimeout.Seconds(),
		"sql_pool_open", mysql.PoolMaxOpen, "sql_pool_idle", mysql.PoolMaxIdle,
		"token_cache_s", c.TokenCacheTime.Seconds(), "notification_driver", c.Notifications.Driver,
		"catalog", catalogName(c), "original_account", c.OriginalAccount, "store_layout", storeLayout(c),
	}
}

func catalogName(c config.Config) string {
	if c.OfficialCatalog != "" {
		return "official"
	}
	return "own"
}

func storeLayout(c config.Config) string {
	if c.Binary.OfficialStoreDir != "" {
		return "official-direct"
	}
	return "own-staged"
}
