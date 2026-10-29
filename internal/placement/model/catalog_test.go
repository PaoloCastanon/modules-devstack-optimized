package model

import "testing"

func TestCustomCatalogNames(t *testing.T) {
	for _, name := range []string{"CUSTOM_X", "CUSTOM_B08_123"} {
		if err := ValidateCustomName(name); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"CUSTOM_", "VCPU", "custom_x", "CUSTOM_BAD-NAME", "CUSTOM_界"} {
		if err := ValidateCustomName(name); err == nil {
			t.Fatal("invalid custom name accepted", name)
		}
	}
}
