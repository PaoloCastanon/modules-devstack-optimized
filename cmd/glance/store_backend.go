package main

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

// openStore opens Glance Go's private root (lock, scope marker) with the
// images in the original's filesystem_store_datadir, as glance_store writes
// them. initStore prepares that private root once.
func openStore(b config.Binary, scope string) (store.Backend, error) {
	return store.NewSharedFilesystem(b.Root, scope, b.OfficialStoreDir)
}

func initStore(b config.Binary, scope string) error { return store.Initialize(b.Root, scope) }
