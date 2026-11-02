package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"strings"
)

type Allocations interface {
	ConsumerAllocations(context.Context, string) (model.ConsumerAllocations, error)
	ProviderAllocations(context.Context, string) (model.ProviderAllocations, error)
	Usages(context.Context, string) (model.Usages, error)
	ReplaceAllocations(context.Context, string, model.AllocationUpdate) error
	DeleteAllocations(context.Context, string) error
}

var ErrProviderInUse = errors.New("provider has allocations")

type InventoryInUse struct {
	Provider string
	Classes  []string
}

func (e *InventoryInUse) Error() string {
	return fmt.Sprintf("Inventory for '%s' on resource provider '%s' in use.", strings.Join(e.Classes, ", "), e.Provider)
}
