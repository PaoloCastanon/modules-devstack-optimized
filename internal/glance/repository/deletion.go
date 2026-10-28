package repository

import (
	"context"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

// ImageDeletion atomically tombstones an image and fences its CURRENT data
// operation for cleanup/deleted, preserving metadata, receipts and UUIDs.
// expectedAttempt=nil means no operation may exist; never invent an attempt.
// check runs against the locked image and must only check policy, not perform I/O.
// On ErrCommitUncertain the proposed state is not evidence: reread DataState.
type ImageDeletion interface {
	TombstoneData(context.Context, string, *string, time.Time, func(model.Image) error) (model.DataState, error)
}
