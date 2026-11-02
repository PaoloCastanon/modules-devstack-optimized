package service

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"testing"
)

func TestFlatCandidatesUseCapacityTraitsAndAggregates(t *testing.T) {
	provider := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	state := model.CandidateState{Classes: map[string]bool{"VCPU": true, "MEMORY_MB": true}, Traits: map[string]bool{"OWNER_NOVA": true, "COMPUTE_STATUS_DISABLED": true}, Providers: []model.CandidateProvider{{Provider: model.Provider{UUID: provider}, Inventories: map[string]model.Inventory{"VCPU": {Total: 8, MinUnit: 1, MaxUnit: 8, StepSize: 1, AllocationRatio: 1}, "MEMORY_MB": {Total: 16, MinUnit: 1, MaxUnit: 16, StepSize: 1, AllocationRatio: 1}}, Usage: map[string]int64{"VCPU": 5}, Traits: map[string]bool{"OWNER_NOVA": true}, Aggregates: map[string]bool{"aggregate": true}}}}
	q := model.CandidateQuery{Groups: []model.RequestGroup{{Resources: map[string]int64{"VCPU": 2}}}, RootRequired: []string{"OWNER_NOVA"}, RootForbidden: []string{"COMPUTE_STATUS_DISABLED"}}
	result, err := candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 1 || result.Requests[0].Allocations[provider].Resources["VCPU"] != 2 || result.Summaries[provider].Resources["VCPU"].Used != 5 || result.Summaries[provider].Resources["MEMORY_MB"].Capacity != "16" {
		t.Fatal(result, err)
	}
	q.Groups[0].Resources["VCPU"] = 4
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 0 {
		t.Fatal("capacity ignored", result, err)
	}
	q.Groups[0].Resources["VCPU"] = 2
	q.Groups[0].MemberOf = [][]string{{"aggregate"}, {"other", "aggregate"}}
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 1 {
		t.Fatal("AND/OR aggregate filters", result, err)
	}
	q.Groups[0].ForbiddenAggregates = []string{"aggregate"}
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 0 {
		t.Fatal("forbidden aggregate ignored", result, err)
	}
	q.Groups[0].ForbiddenAggregates = nil
	state.Providers[0].Traits["COMPUTE_STATUS_DISABLED"] = true
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 0 {
		t.Fatal("disabled root accepted", result, err)
	}
	delete(state.Providers[0].Traits, "COMPUTE_STATUS_DISABLED")
	q.Groups = append(q.Groups, model.RequestGroup{Suffix: "2", Resources: map[string]int64{"VCPU": 2}})
	q.Groups[0].Suffix = "1"
	q.GroupPolicy = "none"
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 0 {
		t.Fatal("merged resource capacity ignored", result, err)
	}
	state.Providers[0].Usage["VCPU"] = 0
	q.GroupPolicy = "isolate"
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 0 {
		t.Fatal("isolated groups shared flat root", result, err)
	}
	q.GroupPolicy = "none"
	result, err = candidatesFromState(state, q)
	if err != nil || len(result.Requests) != 1 || result.Requests[0].Allocations[provider].Resources["VCPU"] != 4 {
		t.Fatal("group merge", result, err)
	}
	q.Groups[0].Resources = map[string]int64{"MISSING": 1}
	if _, err = candidatesFromState(state, q); model.Status(err) != 400 {
		t.Fatal("unknown resource ignored", err)
	}
}
