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
