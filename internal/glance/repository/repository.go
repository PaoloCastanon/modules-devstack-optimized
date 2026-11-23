package repository

import (
	"context"
	"errors"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

var (
	ErrNotFound  = errors.New("image not found")
	ErrDuplicate = errors.New("image UUID already exists")
	ErrConflict  = errors.New("concurrent metadata transaction conflict")
)

type Repository interface {
	Create(context.Context, model.Image) error
	Get(context.Context, string) (model.Image, error)
	List(context.Context, model.ListOptions) ([]model.Image, error)
	// Update and Delete invoke the business callback while holding a row lock.
	// A callback error must cause rollback without any persisted changes.
	Update(context.Context, string, func(model.Image) (model.Image, error)) (model.Image, error)
	Delete(context.Context, string, time.Time, func(model.Image) error) error
}

var ErrMemberNotFound = errors.New("image member not found")
var ErrMemberDuplicate = errors.New("image member already exists")

// Members is the original's image_members table. Writes run under the image
// row lock (as catalog PATCH/DELETE) and call check with the current image
// before changing anything; a check error rolls back.
type Members interface {
	ListMembers(ctx context.Context, imageID string) ([]model.Member, error)
	// GetMember returns ErrMemberNotFound when the project is not a member.
	GetMember(ctx context.Context, imageID, memberID string) (model.Member, error)
	CreateMember(ctx context.Context, imageID, memberID string, now time.Time, check func(image model.Image, members int) error) (model.Member, error)
	UpdateMember(ctx context.Context, imageID, memberID, status string, now time.Time, check func(model.Image, model.Member) error) (model.Member, error)
	DeleteMember(ctx context.Context, imageID, memberID string, now time.Time, check func(model.Image, model.Member) error) (model.Member, error)
}
