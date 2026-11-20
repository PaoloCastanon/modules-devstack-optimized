package config

import (
	"errors"
	"path/filepath"
	"regexp"
)

var officialCatalogName = regexp.MustCompile(`^(glance|glance_official_test_[a-z0-9_]+)$`)

// loadCatalog reads GLANCE_GO_CATALOG: "own" (default, Glance Go's own
// database and store) or "official" (the original's catalog and store
// directory, shared with it; only one of them serves /image at a time).
func loadCatalog(c *Config, env func(string) string) error {
	switch env("GLANCE_GO_CATALOG") {
	case "", "own":
		if env("GLANCE_GO_OFFICIAL_DATABASE") != "" || env("GLANCE_GO_OFFICIAL_STORE_DIR") != "" {
			return errors.New("GLANCE_GO_OFFICIAL_* settings require GLANCE_GO_CATALOG=official")
		}
		return nil
	case "official":
	default:
		return errors.New("GLANCE_GO_CATALOG must be own or official")
	}
	c.OfficialCatalog = "glance"
	if v := env("GLANCE_GO_OFFICIAL_DATABASE"); v != "" {
		c.OfficialCatalog = v
	}
	if !officialCatalogName.MatchString(c.OfficialCatalog) {
		return errors.New("GLANCE_GO_OFFICIAL_DATABASE must name the original Glance database")
	}
	dir := env("GLANCE_GO_OFFICIAL_STORE_DIR")
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || dir == "/" {
		return errors.New("GLANCE_GO_OFFICIAL_STORE_DIR must be the original's absolute filesystem_store_datadir")
	}
	if c.Binary.Backend != "" && c.Binary.Backend != "filesystem" {
		return errors.New("the official catalog is supported only with the filesystem store")
	}
	c.Binary.OfficialStoreDir = dir
	return nil
}
