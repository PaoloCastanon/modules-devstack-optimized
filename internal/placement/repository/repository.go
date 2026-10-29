package repository

import (
	"context"
	"errors"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

var (
	ErrNotFound      = errors.New("provider not found")
	ErrDuplicate     = errors.New("provider name or UUID already exists")
	ErrDuplicateUUID = errors.Join(ErrDuplicate, errors.New("duplicate provider UUID"))
	ErrDuplicateName = errors.Join(ErrDuplicate, errors.New("duplicate provider name"))
	ErrConcurrent    = errors.New("resource provider generation conflict")
	ErrUnknownClass  = errors.New("unknown resource class")
)

type Repository interface {
	Create(context.Context, model.Provider) error
	Get(context.Context, string) (model.Provider, error)
	List(context.Context, model.Filter) ([]model.Provider, error)
	Rename(context.Context, string, string) (model.Provider, error)
	Delete(context.Context, string) error
	Inventories(context.Context, string) (model.Inventories, error)
	ReplaceInventories(context.Context, string, int64, map[string]model.Inventory) (model.Inventories, error)
}
