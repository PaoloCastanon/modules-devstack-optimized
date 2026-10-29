package model

import (
	"math"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestProviderContract(t *testing.T) {
	p, err := NewProvider("compute", "4446c55fee694f16b6c0b562d5f200ab")
	if err != nil || p.UUID != "4446c55f-ee69-4f16-b6c0-b562d5f200ab" || p.Generation != 0 {
		t.Fatal(p, err)
	}
	for _, input := range []struct{ name, id string }{{strings.Repeat("界", 201), ""}, {"x", "bad"}} {
		if _, err := NewProvider(input.name, input.id); err == nil {
			t.Fatal("invalid provider accepted")
		}
	}
	if _, err := NewProvider("", ""); err != nil {
		t.Fatal("schema permits empty name", err)
	}
}

func TestInventoryDefaultsAndCapacityV10(t *testing.T) {
	inv, err := (InventoryInput{Total: ptr(int64(8))}).Resolve()
	if err != nil || inv.Total != 8 || inv.Reserved != 0 || inv.MinUnit != 1 || inv.MaxUnit != math.MaxInt32 || inv.StepSize != 1 || inv.AllocationRatio != 1 {
		t.Fatal(inv, err)
	}
	for _, input := range []InventoryInput{
		{}, {Total: ptr(int64(0))}, {Total: ptr(int64(-1))}, {Total: ptr(int64(math.MaxInt32 + 1))},
		{Total: ptr(int64(1)), Reserved: ptr(int64(1))},
		{Total: ptr(int64(2)), MinUnit: ptr(int64(0))},
		{Total: ptr(int64(2)), AllocationRatio: ptr(math.NaN())},
		{Total: ptr(int64(2)), AllocationRatio: ptr(math.Inf(1))},
		{Total: ptr(int64(2)), AllocationRatio: ptr(0.0)},
	} {
		if _, err := input.Resolve(); err == nil {
			t.Fatalf("invalid inventory accepted: %+v", input)
		}
	}
	// Upstream checks capacity, not min_unit <= max_unit, at this boundary.
	if _, err := (InventoryInput{Total: ptr(int64(2)), MinUnit: ptr(int64(4)), MaxUnit: ptr(int64(1))}).Resolve(); err != nil {
		t.Fatal(err)
	}
}
