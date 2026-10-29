package service

import "context"

func (u *UploadCoordinator) notifyLocked() {
	close(u.changed)
	u.changed = make(chan struct{})
}

// StopAdmission is irreversible for this owner. Existing operations keep their
// contexts and bounded detached compensation. The HTTP owner first drains them,
// then cancels/closes network I/O if its grace expires. Never release the flock
// while a source, fsync, cleanup or returned descriptor is still alive.
func (u *UploadCoordinator) StopAdmission() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopping = true
	u.notifyLocked()
}

// WaitIdle does not close dependencies or abandon workers on timeout. The caller
// must keep the store owned if this returns an error. Close still checks activity.
func (u *UploadCoordinator) WaitIdle(ctx context.Context) error {
	for {
		u.mu.Lock()
		idle := !u.recovering && len(u.active) == 0 && len(u.deleting) == 0 && u.readers == 0
		changed := u.changed
		u.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
