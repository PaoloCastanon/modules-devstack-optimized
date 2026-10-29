package model

import "time"

// AllocationRequest covers the 1.0–1.8 array format. Ownership becomes
// required at 1.8; provider-indexed writes and consumer generations come later.
type AllocationRequest struct {
	ProjectID   *string `json:"project_id,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
	Allocations []struct {
		Provider struct {
			UUID string `json:"uuid"`
		} `json:"resource_provider"`
		Resources map[string]int64 `json:"resources"`
	} `json:"allocations"`
}
type ResourceAllocation struct {
	Generation int64            `json:"generation"`
	Resources  map[string]int64 `json:"resources"`
}
type ConsumerAllocations struct {
	LastModified time.Time                     `json:"-"`
	ProjectID    *string                       `json:"project_id,omitempty"`
	UserID       *string                       `json:"user_id,omitempty"`
	Generation   *int64                        `json:"consumer_generation,omitempty"`
	Allocations  map[string]ResourceAllocation `json:"allocations"`
}
type ConsumerResources struct {
	Generation *int64           `json:"consumer_generation,omitempty"`
	Resources  map[string]int64 `json:"resources"`
}
type ProviderAllocations struct {
	LastModified time.Time                    `json:"-"`
	Generation   int64                        `json:"resource_provider_generation"`
	Allocations  map[string]ConsumerResources `json:"allocations"`
}
type Usages struct {
	Generation int64            `json:"resource_provider_generation"`
	Usages     map[string]int64 `json:"usages"`
}

// AllocationUpdate is the service-to-repository request, separate from JSON.
type AllocationUpdate struct {
	CheckGeneration bool
	Generation      *int64
	Allocations     map[string]map[string]int64
	ProjectID       string
	UserID          string
}

// ModernAllocationRequest is the provider-indexed wire format. Schema validation
// distinguishes absent generation from explicit null before this is decoded.
type ModernAllocationRequest struct {
	Allocations        map[string]ResourceAllocation `json:"allocations"`
	ProjectID          string                        `json:"project_id"`
	UserID             string                        `json:"user_id"`
	ConsumerGeneration *int64                        `json:"consumer_generation"`
	Mappings           map[string][]string           `json:"mappings,omitempty"`
}
