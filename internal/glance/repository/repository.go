package repository

import (
	"context"
	"errors"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

var (
	ErrNotFound  = errors.New("image not found")
	ErrDuplicate = errors.New("image UUID already exists")
)

type Repository interface {
	Create(context.Context, model.Image) error
	Get(context.Context, string) (model.Image, error)
}
