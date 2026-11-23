//go:build linux

package api

import (
	"os"
	"path/filepath"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

// testStore opens a test store root with its images in a sibling directory
// (root + "-images"), the layout of the original's filesystem_store_datadir.
func testStore(root, scope string) (*store.Filesystem, error) {
	objects := filepath.Clean(root) + "-images"
	if err := os.MkdirAll(objects, 0o755); err != nil {
		return nil, err
	}
	return store.NewSharedFilesystem(root, scope, objects)
}
