// Package model defines the selected Placement 1.0 provider/inventory contract.
package model

import (
	"errors"
	"math"
	"net/http"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Error struct {
	HTTPStatus int
	Detail     string
}

func (e *Error) Error() string             { return e.Detail }
func Fail(status int, detail string) error { return &Error{status, detail} }
func Status(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.HTTPStatus
	}
	return http.StatusServiceUnavailable
}

type Provider struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	Generation int64  `json:"generation"`
}
type Filter struct {
	Name       *string
	UUID       *string
	Aggregates []string
	Resources  map[string]int64
}

func NormalizeUUID(value string) (string, error) {
	// The pinned upstream accepts compact UUIDs as well as hyphenated UUIDs.
	if len(value) != 32 && len(value) != 36 {
		return "", Fail(400, "Invalid UUID.")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return "", Fail(400, "Invalid UUID.")
	}
	return id.String(), nil
}
func ValidateName(name string) error {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 {
		return Fail(400, "Provider name must contain at most 200 characters.")
	}
	return nil
}
func NewProvider(name, id string) (Provider, error) {
	if err := ValidateName(name); err != nil {
		return Provider{}, err
	}
	var err error
	if id == "" {
		var generated uuid.UUID
		generated, err = uuid.NewRandom()
		id = generated.String()
	} else {
		id, err = NormalizeUUID(id)
	}
	return Provider{UUID: id, Name: name}, err
}

type Inventory struct {
	Total           int64   `json:"total"`
	Reserved        int64   `json:"reserved"`
	MinUnit         int64   `json:"min_unit"`
	MaxUnit         int64   `json:"max_unit"`
	StepSize        int64   `json:"step_size"`
	AllocationRatio float64 `json:"allocation_ratio"`
}
type InventoryInput struct {
	Total           *int64   `json:"total"`
	Reserved        *int64   `json:"reserved"`
	MinUnit         *int64   `json:"min_unit"`
	MaxUnit         *int64   `json:"max_unit"`
	StepSize        *int64   `json:"step_size"`
	AllocationRatio *float64 `json:"allocation_ratio"`
}

// ErrInventoryCapacity identifies the domain rule separately from malformed
// fields, so the service can include the actual provider and resource class.
var ErrInventoryCapacity = Fail(400, "Inventory capacity must be positive at microversion 1.0.")

func (in InventoryInput) Resolve() (Inventory, error) {
	inv := Inventory{MinUnit: 1, MaxUnit: math.MaxInt32, StepSize: 1, AllocationRatio: 1}
	if in.Total == nil {
		return inv, Fail(400, "Inventory total is required.")
	}
	inv.Total = *in.Total
	for _, v := range []struct {
		src *int64
		dst *int64
	}{{in.Reserved, &inv.Reserved}, {in.MinUnit, &inv.MinUnit}, {in.MaxUnit, &inv.MaxUnit}, {in.StepSize, &inv.StepSize}} {
		if v.src != nil {
			*v.dst = *v.src
		}
	}
	if in.AllocationRatio != nil {
		inv.AllocationRatio = *in.AllocationRatio
	}
	for _, v := range []struct{ value, min int64 }{{inv.Total, 1}, {inv.Reserved, 0}, {inv.MinUnit, 1}, {inv.MaxUnit, 1}, {inv.StepSize, 1}} {
		if v.value < v.min || v.value > math.MaxInt32 {
			return inv, Fail(400, "Inventory integer outside supported range.")
		}
	}
	ratio := inv.AllocationRatio
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio > math.MaxFloat32 || ratio < 0 {
		return inv, Fail(400, "Inventory capacity must be positive at microversion 1.0.")
	}
	if math.Trunc(float64(inv.Total-inv.Reserved)*ratio) <= 0 {
		return inv, ErrInventoryCapacity
	}
	return inv, nil
}

type Inventories struct {
	Generation  int64                `json:"resource_provider_generation"`
	Inventories map[string]Inventory `json:"inventories"`
}

// B08 will extend the catalogue. These classes are observed in the S2 inventory.
func KnownClass(name string) bool { return name == "VCPU" || name == "MEMORY_MB" || name == "DISK_GB" }
