package service

import (
	"context"
	"encoding/json"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/repository"
	"math"
	"sort"
	"strconv"
)

func (s *Service) Candidates(ctx context.Context, who auth.Identity, query model.CandidateQuery) (model.CandidateResult, error) {
	if err := authorize(who); err != nil {
		return model.CandidateResult{}, err
	}
	r, ok := s.repo.(repository.Candidates)
	if !ok {
		return model.CandidateResult{}, model.Fail(503, "Candidate repository unavailable.")
	}
	state, err := r.CandidateState(ctx)
	if err != nil {
		return model.CandidateResult{}, err
	}
	return candidatesFromState(state, query)
}
func candidatesFromState(state model.CandidateState, q model.CandidateQuery) (model.CandidateResult, error) {
	out := model.CandidateResult{Requests: []model.AllocationCandidate{}, Summaries: map[string]model.ProviderSummary{}}
	traits := append(append([]string{}, q.RootRequired...), q.RootForbidden...)
	numbered := 0
	for _, group := range q.Groups {
		if group.Suffix != "" {
			numbered++
		}
		traits = append(traits, group.Required...)
		traits = append(traits, group.Forbidden...)
		for class := range group.Resources {
			if !state.Classes[class] {
				return out, model.Fail(400, "Invalid resource class in resources parameter: No such resource class "+class+".")
			}
		}
	}
	sort.Strings(traits)
	for _, trait := range traits {
		if !state.Traits[trait] {
			return out, model.Fail(400, "No such trait(s): "+trait+".")
		}
	}
	for _, p := range state.Providers {
		if p.Traits["MISC_SHARES_VIA_AGGREGATE"] {
			return out, model.Fail(400, "Shared providers are outside the flat-provider profile.")
		}
		if !hasTraits(p.Traits, q.RootRequired, q.RootForbidden) {
			continue
		}
		// In a flat tree all numbered groups use the same root. Isolation of two
		// numbered groups is impossible without child providers, so no candidate.
		if q.GroupPolicy == "isolate" && numbered > 1 {
			continue
		}
		combined := map[string]int64{}
		mapping := map[string][]string{}
		matches := true
		for _, g := range q.Groups {
			if g.InTree != "" && g.InTree != p.Provider.UUID {
				matches = false
				break
			}
			if !hasTraits(p.Traits, g.Required, g.Forbidden) || !hasAggregates(p.Aggregates, g.MemberOf, g.ForbiddenAggregates) {
				matches = false
				break
			}
			mapping[g.Suffix] = []string{p.Provider.UUID}
			for class, amount := range g.Resources {
				inv, ok := p.Inventories[class]
				used := p.Usage[class]
				if !ok || amount < inv.MinUnit || amount > inv.MaxUnit || amount%inv.StepSize != 0 || amount > math.MaxInt64-used || model.ExceedsCapacity(used+amount, inv) {
					matches = false
					break
				}
				if amount > math.MaxInt64-combined[class] {
					matches = false
					break
				}
				combined[class] += amount
			}
			if !matches {
				break
			}
		}
		if !matches {
			continue
		}
		for class, amount := range combined {
			inv := p.Inventories[class]
			used := p.Usage[class]
			if amount > inv.MaxUnit || amount > math.MaxInt64-used || model.ExceedsCapacity(used+amount, inv) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		out.Requests = append(out.Requests, model.AllocationCandidate{Allocations: map[string]model.CandidateResources{p.Provider.UUID: {Resources: combined}}, Mappings: mapping})
		summary := model.ProviderSummary{Resources: map[string]model.SummaryResource{}, Traits: []string{}}
		for class, inv := range p.Inventories {
			capacity := math.Trunc(float64(inv.Total-inv.Reserved) * inv.AllocationRatio)
			summary.Resources[class] = model.SummaryResource{Capacity: json.Number(strconv.FormatFloat(capacity, 'f', 0, 64)), Used: p.Usage[class]}
		}
		for trait := range p.Traits {
			summary.Traits = append(summary.Traits, trait)
		}
		sort.Strings(summary.Traits)
		out.Summaries[p.Provider.UUID] = summary
		if q.Limit > 0 && len(out.Requests) >= q.Limit {
			break
		}
	}
	return out, nil
}
func hasTraits(actual map[string]bool, required, forbidden []string) bool {
	for _, trait := range required {
		if !actual[trait] {
			return false
		}
	}
	for _, trait := range forbidden {
		if actual[trait] {
			return false
		}
	}
	return true
}
func hasAggregates(actual map[string]bool, required [][]string, forbidden []string) bool {
	for _, group := range required {
		found := false
		for _, id := range group {
			found = found || actual[id]
		}
		if !found {
			return false
		}
	}
	for _, id := range forbidden {
		if actual[id] {
			return false
		}
	}
	return true
}
