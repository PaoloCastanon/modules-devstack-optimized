package repository

import (
	"context"
	"errors"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

var ErrStandard = errors.New("standard catalogue entry cannot be changed")
var ErrInUse = errors.New("catalogue entry in use")

type MissingTraits struct{ Names []string }

func (e *MissingTraits) Error() string { return "unknown traits" }

type Catalog interface {
	Classes(context.Context) ([]model.CatalogEntry, error)
	Class(context.Context, string) (model.CatalogEntry, error)
	CreateClass(context.Context, string, bool) (bool, error)
	RenameClass(context.Context, string, string) error
	DeleteClass(context.Context, string) error
	Traits(context.Context, model.TraitFilter) ([]string, error)
	Trait(context.Context, string) (model.CatalogEntry, error)
	EnsureTrait(context.Context, string) (bool, error)
	DeleteTrait(context.Context, string) error
	ProviderTraits(context.Context, string) (model.TraitSet, error)
	ReplaceTraits(context.Context, string, int64, []string) (model.TraitSet, error)
	Aggregates(context.Context, string) ([]string, error)
	ReplaceAggregates(context.Context, string, []string) ([]string, error)
	AggregateState(context.Context, string) (model.AggregateSet, error)
	ReplaceAggregatesGeneration(context.Context, string, int64, []string) (model.AggregateSet, error)
	DeleteInventories(context.Context, string) error
}
