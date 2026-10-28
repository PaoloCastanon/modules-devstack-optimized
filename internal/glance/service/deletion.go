package service

import (
	"context"
	"errors"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

// claimDeletion excludes the ENTIRE lifetime of a writer, including compensation
// and uncertain commits. Do not cancel/abandon a blocked Reader or wait holding
// a SQL lock. A live writer yields controlled busy; the client may retry later.
func (u *UploadCoordinator) claimDeletion(id string) (func(), error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed || !u.ready || u.recovering {
		return nil, ErrUploadNotReady
	}
	if u.active[id] || u.deleting[id] || len(u.active)+len(u.deleting)+u.readers >= u.limits.MaxConcurrent {
		return nil, ErrUploadBusy
	}
	u.deleting[id] = true
	return func() { u.mu.Lock(); delete(u.deleting, id); u.mu.Unlock() }, nil
}

func deletionPolicy(who auth.Identity, i model.Image) error {
	if err := writePolicy(who, i); err != nil {
		return err
	}
	if i.Protected {
		return model.Fail(model.Forbidden, "Protected images cannot be deleted.")
	}
	return nil
}

func sameOptionalText(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func sameDataReceipt(a, b model.DataState) bool {
	if !sameNumber(a.Size, b.Size) || !sameNumber(a.VirtualSize, b.VirtualSize) || !sameOptionalText(a.Checksum, b.Checksum) || !sameOptionalText(a.HashAlgorithm, b.HashAlgorithm) || !sameOptionalText(a.HashValue, b.HashValue) {
		return false
	}
	if a.Operation == nil || b.Operation == nil {
		return a.Operation == nil && b.Operation == nil
	}
	x, y := a.Operation, b.Operation
	return x.ImageID == y.ImageID && x.AttemptID == y.AttemptID && sameNumber(x.ExpectedSize, y.ExpectedSize) && sameNumber(x.Size, y.Size) && sameNumber(x.VirtualSize, y.VirtualSize) && sameOptionalText(x.Checksum, y.Checksum) && sameOptionalText(x.SHA512, y.SHA512)
}

func cleanupFence(s model.DataState, key store.Key, target string, phase model.DataPhase) bool {
	o := s.Operation
	if s.ImageID != key.ImageID || o == nil || o.ImageID != key.ImageID || o.AttemptID != key.AttemptID || o.Phase != phase || !sameText(o.CleanupTarget, target) {
		return false
	}
	if target == "deleted" {
		if !s.Deleted || s.Status != "deleted" {
			return false
		}
		if noImageData(s) {
			return true
		}
		if o.Size == nil || o.Checksum == nil || o.SHA512 == nil {
			return false
		}
		c := model.DataContent{Size: *o.Size, VirtualSize: o.VirtualSize, Checksum: *o.Checksum, SHA512: *o.SHA512}
		return c.Validate(o.ExpectedSize) == nil && imageReceiptMatches(s, c)
	}
	return target == "killed" && !s.Deleted && noImageData(s) && (phase == model.DataCleanup && s.Status == "saving" || phase == model.DataCleaned && s.Status == "killed")
}

// finishCleanup is used by the sole deletion owner or exclusive startup only.
// Its caller must prove no writer for this key can still execute Stage/Publish.
// Retrying Delete also synchronizes directories when names are already absent.
func (u *UploadCoordinator) finishCleanup(ctx context.Context, before model.DataState, target string) error {
	if before.Operation == nil {
		return ErrUploadReconciliation
	}
	key := store.Key{ImageID: before.ImageID, AttemptID: before.Operation.AttemptID}
	if !cleanupFence(before, key, target, model.DataCleanup) {
		return ErrUploadReconciliation
	}
	s, err := u.readState(ctx, key.ImageID)
	if err != nil || !cleanupFence(s, key, target, model.DataCleanup) || !sameDataReceipt(before, s) {
		return ErrUploadReconciliation
	}
	if u.bytes.Delete(ctx, key) != nil {
		return ErrUploadReconciliation
	}
	s, err = u.readState(ctx, key.ImageID)
	if err != nil || !cleanupFence(s, key, target, model.DataCleanup) || !sameDataReceipt(before, s) {
		return ErrUploadReconciliation
	}
	err = u.transfers.CompleteDataCleanup(ctx, key.ImageID, key.AttemptID, u.now())
	if err != nil && !errors.Is(err, repository.ErrCommitUncertain) {
		return ErrUploadReconciliation
	}
	s, err = u.readState(ctx, key.ImageID)
	if err != nil || !cleanupFence(s, key, target, model.DataCleaned) || !sameDataReceipt(before, s) {
		return ErrUploadReconciliation
	}
	return nil
}

// DeleteImage extends B20 through the same exclusive owner as PUT/GET. No client
// controls attempt/root/target. Authorization is repeated under the SQL row lock.
func (u *UploadCoordinator) DeleteImage(ctx context.Context, who auth.Identity, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := who.Check(u.now()); err != nil {
		return err
	}
	if !who.HasRole("member") && !who.HasRole("admin") {
		return model.Fail(model.Forbidden, "Deleting images requires member or admin role.")
	}
	id, err := model.CanonicalID(value)
	if err != nil {
		return err
	}
	i, err := u.images.Get(ctx, id)
	if err != nil {
		return uploadDBError(err, id)
	}
	if i.ID != id {
		return ErrUploadReconciliation
	}
	if i.Status == "deleted" {
		return notFound(id)
	}
	if err = deletionPolicy(who, i); err != nil {
		return err
	}
	leave, err := u.claimDeletion(id)
	if err != nil {
		return err
	}
	defer leave()
	d, ok := u.transfers.(repository.ImageDeletion)
	if !ok {
		return ErrUploadUnavailable
	}
	before, err := u.readState(ctx, id)
	if err != nil {
		return err
	}
	if before.Deleted {
		return notFound(id)
	}
	var attempt *string
	if before.Operation != nil {
		v := before.Operation.AttemptID
		attempt = &v
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	_, err = d.TombstoneData(ctx, id, attempt, u.now(), func(current model.Image) error {
		if current.ID != id {
			return ErrUploadReconciliation
		}
		if err := who.Check(u.now()); err != nil {
			return err
		}
		return deletionPolicy(who, current)
	})
	if err != nil && !errors.Is(err, repository.ErrCommitUncertain) {
		var domain *model.Error
		if errors.As(err, &domain) || errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrForbidden) || errors.Is(err, auth.ErrUnavailable) {
			return err
		}
		return uploadDBError(err, id)
	}
	// Detached, bounded resolution after SQL may have committed even if the
	// request was cancelled. A proposed SQL return value is never confirmation.
	resolved, cancel := u.resolution(ctx)
	defer cancel()
	s, readErr := u.readState(resolved, id)
	if readErr != nil || !s.Deleted || s.Status != "deleted" || !sameDataReceipt(before, s) {
		return ErrUploadReconciliation
	}
	if before.Operation == nil {
		if s.Operation != nil {
			return ErrUploadReconciliation
		}
		return nil // queued/killed without any operation: no fake key or FS action
	}
	key := store.Key{ImageID: id, AttemptID: *attempt}
	if !cleanupFence(s, key, "deleted", model.DataCleanup) {
		return ErrUploadReconciliation
	}
	return u.finishCleanup(resolved, s, "deleted")
}
