//go:build linux

package service

import (
	"sync"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

var uploadFilesystems sync.Map

// NewUploadCoordinator takes exclusive ownership of an already opened
// NewFilesystem instance (which holds the kernel root flock). A second coordinator
// on that instance is refused. The caller must not use/close fs while owned.
// Root initialization, scope/database binding and SQL configuration are explicit
// operator/startup responsibilities, never request input. No services start here.
func NewUploadCoordinator(images ImageReader, t repository.Transfers, fs *store.Filesystem, limits UploadLimits) (*UploadCoordinator, error) {
	if fs == nil {
		return nil, store.ErrUnsafe
	}
	u, err := newUploadCoordinator(images, t, fs, limits)
	if err != nil {
		return nil, err
	}
	if _, loaded := uploadFilesystems.LoadOrStore(fs, struct{}{}); loaded {
		return nil, store.ErrBusy
	}
	u.release = func() { uploadFilesystems.Delete(fs) }
	return u, nil
}
