package model

import "encoding/json"

type CandidateProvider struct {
	Provider    Provider
	Inventories map[string]Inventory
	Usage       map[string]int64
	Traits      map[string]bool
	Aggregates  map[string]bool
}
type CandidateState struct {
	Providers []CandidateProvider
	Classes   map[string]bool
	Traits    map[string]bool
}
type RequestGroup struct {
	Suffix              string
	Resources           map[string]int64
	Required, Forbidden []string
	MemberOf            [][]string
	ForbiddenAggregates []string
	InTree              string
}
type CandidateQuery struct {
	Groups                      []RequestGroup
	RootRequired, RootForbidden []string
	SameSubtrees                [][]string
	GroupPolicy                 string
	Limit                       int
}
type CandidateResources struct {
	Resources map[string]int64 `json:"resources"`
}
type AllocationCandidate struct {
	Allocations map[string]CandidateResources `json:"allocations"`
	Mappings    map[string][]string           `json:"mappings,omitempty"`
}
type SummaryResource struct {
	Capacity json.Number `json:"capacity"`
	Used     int64       `json:"used"`
}
type ProviderSummary struct {
	Resources map[string]SummaryResource `json:"resources"`
	Traits    []string                   `json:"traits"`
}
type CandidateResult struct {
	Requests  []AllocationCandidate
	Summaries map[string]ProviderSummary
}
