package api

import (
	"fmt"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) candidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	values, object, err := queryObject(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	schema := "candidates_1_10"
	for _, v := range []int{16, 17, 21, 25, 31, 33, 35, 36} {
		if microversion(r) >= v {
			schema = "candidates_1_" + strconv.Itoa(v)
		}
	}
	if err = validateRequest(object, schema, "Invalid query string parameters: "); err != nil {
		h.failure(w, r, err)
		return
	}
	q, err := candidateQuery(r, values, object)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	out, err := h.service.Candidates(r.Context(), identity(r), q)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	requests := []any{}
	summaries := map[string]any{}
	requested := map[string]bool{}
	for _, g := range q.Groups {
		for class := range g.Resources {
			requested[class] = true
		}
	}
	for _, candidate := range out.Requests {
		var allocations any = candidate.Allocations
		if microversion(r) < 12 {
			items := []any{}
			ids := []string{}
			for id := range candidate.Allocations {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				item := candidate.Allocations[id]
				items = append(items, map[string]any{"resource_provider": map[string]string{"uuid": id}, "resources": item.Resources})
			}
			allocations = items
		}
		item := map[string]any{"allocations": allocations}
		if microversion(r) >= 34 {
			item["mappings"] = candidate.Mappings
		}
		requests = append(requests, item)
	}
	for id, summary := range out.Summaries {
		resources := map[string]model.SummaryResource{}
		for class, value := range summary.Resources {
			if microversion(r) >= 27 || requested[class] {
				resources[class] = value
			}
		}
		item := map[string]any{"resources": resources}
		if microversion(r) >= 17 {
			item["traits"] = summary.Traits
		}
		if microversion(r) >= 29 {
			item["parent_provider_uuid"] = nil
			item["root_provider_uuid"] = id
		}
		summaries[id] = item
	}
	h.modified(w, r, time.Now())
	h.json(w, 200, map[string]any{"allocation_requests": requests, "provider_summaries": summaries})
}
func queryFailure(detail, code string) error { return model.FailCode(400, detail, code) }
func parseTraits(value string, allowForbidden bool) ([]string, []string, error) {
	required, forbidden := []string{}, []string{}
	if strings.HasPrefix(value, "in:") {
		return nil, nil, model.Fail(400, "Invalid query string parameters: The format 'in:HW_CPU_X86_VMX,CUSTOM_MAGIC' only supported since microversion 1.39. Got: "+value)
	}
	for _, trait := range strings.Split(value, ",") {
		trait = strings.TrimSpace(trait)
		denied := strings.HasPrefix(trait, "!")
		name := strings.TrimLeft(trait, "!")
		if trait == "" || name == "" {
			form := "HW_CPU_X86_VMX,CUSTOM_MAGIC"
			if allowForbidden {
				form = "HW_CPU_X86_VMX,!CUSTOM_MAGIC"
			}
			return nil, nil, model.Fail(400, "Invalid query string parameters: Expected 'required' parameter value of the form: "+form+". Got an empty trait in: "+value)
		}
		if denied && !allowForbidden {
			return nil, nil, model.Fail(400, "Invalid query string parameters: Expected 'required' parameter value of the form: HW_CPU_X86_VMX,CUSTOM_MAGIC. Got: "+value)
		}
		if denied {
			forbidden = append(forbidden, name)
		} else {
			required = append(required, trait)
		}
	}
	return required, forbidden, nil
}
func parseAggregateGroups(r *http.Request, values []string, suffix string) ([][]string, []string, error) {
	if len(values) > 1 && microversion(r) < 24 {
		return nil, nil, model.Fail(400, "Multiple member_of"+suffix+" parameters are not supported")
	}
	groups := [][]string{}
	forbidden := []string{}
	for _, value := range values {
		if strings.Contains(value, ",") && !strings.HasPrefix(value, "in:") && !strings.HasPrefix(value, "!in:") {
			return nil, nil, model.Fail(400, "Multiple values for 'member_of' must be prefixed with the 'in:' or '!in:' keyword using the valid microversion. Got: "+value)
		}
		denied := strings.HasPrefix(value, "!")
		value = strings.TrimPrefix(value, "!")
		value = strings.TrimPrefix(value, "in:")
		ids := strings.Split(value, ",")
		for _, id := range ids {
			if _, err := model.NormalizeUUID(id); err != nil {
				return nil, nil, model.Fail(400, "Invalid query string parameters: Expected 'member_of' parameter to contain valid UUID(s). Got: "+id)
			}
		}
		if denied {
			if microversion(r) < 32 {
				return nil, nil, model.Fail(400, "Forbidden member_of"+suffix+" parameters are not supported in the specified microversion")
			}
			forbidden = append(forbidden, ids...)
		} else {
			groups = append(groups, ids)
		}
	}
	return groups, forbidden, nil
}
func candidateQuery(r *http.Request, values url.Values, object jsonObject) (model.CandidateQuery, error) {
	q := model.CandidateQuery{}
	if v := values.Get("limit"); v != "" {
		n, err := strconv.ParseUint(v, 10, 63)
		if err != nil || n > uint64(math.MaxInt) {
			q.Limit = math.MaxInt
		} else {
			q.Limit = int(n)
		}
	}
	q.GroupPolicy = values.Get("group_policy")
	if roots, ok := values["root_required"]; ok {
		if len(roots) > 1 {
			return q, queryFailure("Query parameter 'root_required' may be specified only once.", "placement.query.duplicate_key")
		}
		var err error
		q.RootRequired, q.RootForbidden, err = parseTraits(roots[0], true)
		if err != nil {
			return q, err
		}
		for _, name := range q.RootRequired {
			for _, denied := range q.RootForbidden {
				if name == denied {
					return q, queryFailure("Conflicting required and forbidden traits found in root_required: "+name, "placement.query.bad_value")
				}
			}
		}
	}
	for _, v := range values["same_subtree"] {
		group := []string{}
		for _, suffix := range strings.Split(v, ",") {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				return q, queryFailure("Empty string (unsuffixed group) can not be specified in `same_subtree` ", "placement.query.bad_value")
			}
			group = append(group, suffix)
		}
		q.SameSubtrees = append(q.SameSubtrees, group)
	}
	bySuffix := map[string]int{}
	pattern := regexp.MustCompile(`^(resources|required|member_of|in_tree)(.*)$`)
	for _, member := range object {
		parts := pattern.FindStringSubmatch(member.key)
		if parts == nil {
			continue
		}
		prefix, suffix := parts[1], parts[2]
		index, ok := bySuffix[suffix]
		if !ok {
			index = len(q.Groups)
			bySuffix[suffix] = index
			q.Groups = append(q.Groups, model.RequestGroup{Suffix: suffix, Resources: map[string]int64{}})
		}
		g := &q.Groups[index]
		value := member.value.(string)
		var err error
		switch prefix {
		case "resources":
			g.Resources, err = parseResources(value)
		case "required":
			g.Required, g.Forbidden, err = parseTraits(value, microversion(r) >= 22)
		case "member_of":
			g.MemberOf, g.ForbiddenAggregates, err = parseAggregateGroups(r, values[member.key], suffix)
		case "in_tree":
			if _, e := model.NormalizeUUID(strings.TrimSpace(value)); e != nil {
				err = model.Fail(400, "Invalid query string parameters: Expected 'in_tree' parameter to be a format of uuid. Got: "+value)
			} else {
				g.InTree = strings.TrimSpace(value)
			}
		}
		if err != nil {
			return q, err
		}
	}
	if microversion(r) >= 36 {
		resourceCount := 0
		for _, g := range q.Groups {
			if len(g.Resources) > 0 {
				resourceCount++
			}
		}
		if resourceCount == 0 {
			return q, queryFailure("There must be at least one resources or resources[$S] parameter.", "placement.query.missing_value")
		}
		subtree := map[string]bool{}
		for _, group := range q.SameSubtrees {
			for _, suffix := range group {
				subtree[suffix] = true
			}
		}
		for _, g := range q.Groups {
			if len(g.Resources) == 0 && !subtree[g.Suffix] {
				return q, queryFailure("Resourceless suffixed group request should be specified in `same_subtree` query param: bad group(s) - "+pythonRepr([]any{g.Suffix})+".", "placement.query.bad_value")
			}
		}
		for suffix := range subtree {
			if _, ok := bySuffix[suffix]; !ok {
				known := []any{}
				for _, g := range q.Groups {
					known = append(known, g.Suffix)
				}
				return q, queryFailure("Real suffixes should be specified in `same_subtree`: "+pythonRepr([]any{suffix})+" not found in "+pythonRepr(known)+".", "placement.query.bad_value")
			}
		}
	} else {
		if len(q.Groups) == 0 {
			return q, model.Fail(400, "At least one request group (`resources` or `resources{$S}`) is required.")
		}
		for _, g := range q.Groups {
			if len(g.Resources) == 0 {
				if len(g.Required) > 0 {
					return q, model.Fail(400, "All traits parameters must be associated with resources.  Found the following orphaned traits keys: required"+g.Suffix)
				}
				if len(g.MemberOf) > 0 || len(g.ForbiddenAggregates) > 0 {
					return q, model.Fail(400, "All member_of parameters must be associated with resources. Found the following orphaned member_of keys: member_of"+g.Suffix)
				}
				return q, model.Fail(400, "All request groups must specify resources.")
			}
		}
	}
	numbered := 0
	for _, g := range q.Groups {
		if g.Suffix != "" {
			numbered++
		}
		for _, name := range g.Required {
			for _, deny := range g.Forbidden {
				if name == deny {
					return q, model.Fail(400, fmt.Sprintf("Conflicting required and forbidden traits found in the following traits keys: required%s: (%s)", g.Suffix, name))
				}
			}
		}
	}
	if numbered > 1 && q.GroupPolicy == "" {
		return q, model.Fail(400, `The "group_policy" parameter is required when specifying more than one "resources{N}" parameter.`)
	}
	return q, nil
}
