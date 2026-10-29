package model

import "regexp"

var customName = regexp.MustCompile(`^CUSTOM_[A-Z0-9_]+$`)

func ValidateCustomName(name string) error {
	if len(name) > 255 || !customName.MatchString(name) {
		return Fail(400, "Invalid custom catalogue name.")
	}
	return nil
}

type CatalogEntry struct {
	Name     string
	Standard bool
}
type TraitFilter struct {
	Prefix     *string
	Names      []string
	Associated *bool
}
type TraitSet struct {
	Generation int64    `json:"resource_provider_generation"`
	Traits     []string `json:"traits"`
}

type AggregateSet struct {
	Generation int64    `json:"resource_provider_generation"`
	Aggregates []string `json:"aggregates"`
}
