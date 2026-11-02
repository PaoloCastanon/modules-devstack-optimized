package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

func bodySchema(r *http.Request) string {
	switch route(r.URL.Path) {
	case "/allocations/{consumer}":
		if microversion(r) >= 34 {
			return "allocations_1_34"
		}
		if microversion(r) >= 28 {
			return "allocations_1_28"
		}
		if microversion(r) >= 12 {
			return "allocations_1_12"
		}
		if microversion(r) >= 8 {
			return "allocations_1_8"
		}
		return "allocations_1_0"
	case "/resource_classes", "/resource_classes/{name}":
		return "class_body"
	case "/resource_providers/{id}/traits":
		return "traits_set"
	case "/resource_providers/{id}/aggregates":
		if microversion(r) >= 19 {
			return "aggregates_1_19"
		}
		return "aggregates_set"
	default:
		return "inventories"
	}
}
func providerQuerySchema(r *http.Request) string {
	if microversion(r) >= 18 {
		return "query_1_18"
	}
	if microversion(r) >= 14 {
		return "query_1_14"
	}
	if microversion(r) >= 4 {
		return "query_1_4"
	}
	if microversion(r) >= 3 {
		return "query_1_3"
	}
	return "query"
}
func catalogPermission(r *http.Request) string {
	path := route(r.URL.Path)
	prefix, action := "", ""
	switch path {
	case "/resource_classes":
		prefix = "placement:resource_classes:"
		if r.Method == "GET" {
			action = "list"
		}
		if r.Method == "POST" {
			action = "create"
		}
	case "/resource_classes/{name}":
		prefix = "placement:resource_classes:"
		action = map[string]string{"GET": "show", "PUT": "update", "DELETE": "delete"}[r.Method]
	case "/traits":
		prefix = "placement:traits:"
		if r.Method == "GET" {
			action = "list"
		}
	case "/traits/{name}":
		prefix = "placement:traits:"
		action = map[string]string{"GET": "show", "PUT": "update", "DELETE": "delete"}[r.Method]
	case "/resource_providers/{id}/traits":
		prefix = "placement:resource_providers:traits:"
		action = map[string]string{"GET": "list", "PUT": "update", "DELETE": "delete"}[r.Method]
	case "/resource_providers/{id}/aggregates":
		prefix = "placement:resource_providers:aggregates:"
		action = map[string]string{"GET": "list", "PUT": "update"}[r.Method]
	}
	if action == "" {
		return ""
	}
	return prefix + action
}
func (h *Handler) classRepresentation(name string) map[string]any {
	return map[string]any{"name": name, "links": []map[string]string{{"rel": "self", "href": strings.TrimRight(h.base.Path, "/") + "/resource_classes/" + name}}}
}
func (h *Handler) classes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		items, err := h.service.Classes(r.Context(), identity(r))
		if err != nil {
			h.failure(w, r, err)
			return
		}
		out := []map[string]any{}
		for _, item := range items {
			out = append(out, h.classRepresentation(item.Name))
		}
		h.json(w, 200, map[string]any{"resource_classes": out})
	case "POST":
		var input struct {
			Name string `json:"name"`
		}
		if err := h.decode(w, r, &input); err != nil {
			h.failure(w, r, err)
			return
		}
		if _, err := h.service.CreateClass(r.Context(), identity(r), input.Name, false); err != nil {
			h.failure(w, r, err)
			return
		}
		w.Header().Set("Location", h.base.JoinPath("resource_classes", input.Name).String())
		w.WriteHeader(201)
	default:
		h.method(w, r, "GET, POST")
	}
}
func (h *Handler) class(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	switch r.Method {
	case "GET":
		item, err := h.service.Class(r.Context(), identity(r), name)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.json(w, 200, h.classRepresentation(item.Name))
	case "DELETE":
		if err := h.service.DeleteClass(r.Context(), identity(r), name); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	case "PUT":
		if microversion(r) >= 7 {
			if err := validateRequest(jsonObject{{"name", name}}, "class_body", "JSON does not validate: "); err != nil {
				h.failure(w, r, err)
				return
			}
			created, err := h.service.CreateClass(r.Context(), identity(r), name, true)
			if err != nil {
				h.failure(w, r, err)
				return
			}
			w.Header().Set("Location", h.base.JoinPath("resource_classes", name).String())
			if created {
				w.WriteHeader(201)
			} else {
				w.WriteHeader(204)
			}
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := h.decode(w, r, &input); err != nil {
			h.failure(w, r, err)
			return
		}
		if err := h.service.RenameClass(r.Context(), identity(r), name, input.Name); err != nil {
			h.failure(w, r, err)
			return
		}
		h.json(w, 200, h.classRepresentation(input.Name))
	default:
		h.method(w, r, "GET, PUT, DELETE")
	}
}
func (h *Handler) trait(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	switch r.Method {
	case "GET":
		if err := h.service.Trait(r.Context(), identity(r), name); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	case "PUT":
		created, err := h.service.EnsureTrait(r.Context(), identity(r), name)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		w.Header().Set("Location", h.base.JoinPath("traits", name).String())
		if created {
			w.WriteHeader(201)
		} else {
			w.WriteHeader(204)
		}
	case "DELETE":
		if err := h.service.DeleteTrait(r.Context(), identity(r), name); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	default:
		h.method(w, r, "GET, PUT, DELETE")
	}
}
func (h *Handler) traits(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	query, object, err := queryObject(r)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err = validateRequest(object, "traits_query", "Invalid query string parameters: "); err != nil {
		h.failure(w, r, err)
		return
	}
	f := model.TraitFilter{}
	if values, ok := query["name"]; ok {
		v := values[len(values)-1]
		op, value, ok := strings.Cut(v, ":")
		if !ok {
			h.failure(w, r, model.Fail(400, fmt.Sprintf(`Badly formatted name parameter. Expected name query string parameter in form: ?name=[in|startswith]:[name1,name2|prefix]. Got: "%s"`, v)))
			return
		}
		switch op {
		case "in":
			f.Names = strings.Split(value, ",")
		case "startswith":
			f.Prefix = &value
		}
	}
	if values, ok := query["associated"]; ok {
		v := strings.ToLower(values[len(values)-1])
		if v != "true" && v != "false" {
			h.failure(w, r, model.Fail(400, `The query parameter "associated" only accepts "true" or "false"`))
			return
		}
		associated := v == "true"
		f.Associated = &associated
	}
	names, err := h.service.Traits(r.Context(), identity(r), f)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.json(w, 200, map[string]any{"traits": names})
}
func (h *Handler) providerTraits(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.Method {
	case "GET":
		out, err := h.service.ProviderTraits(r.Context(), identity(r), id)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.json(w, 200, out)
	case "PUT":
		var input struct {
			Generation int64    `json:"resource_provider_generation"`
			Traits     []string `json:"traits"`
		}
		if err := h.decode(w, r, &input); err != nil {
			h.failure(w, r, err)
			return
		}
		out, err := h.service.ReplaceTraits(r.Context(), identity(r), id, input.Generation, input.Traits)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.json(w, 200, out)
	case "DELETE":
		if err := h.service.DeleteTraits(r.Context(), identity(r), id); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	default:
		h.method(w, r, "GET, PUT, DELETE")
	}
}
func (h *Handler) aggregates(w http.ResponseWriter, r *http.Request) {
	if microversion(r) >= 19 {
		id := r.PathValue("id")
		var out model.AggregateSet
		var err error
		switch r.Method {
		case "GET":
			out, err = h.service.AggregateState(r.Context(), identity(r), id)
		case "PUT":
			var input model.AggregateSet
			if err = h.decodeExisting(w, r, &input); err == nil {
				out, err = h.service.ReplaceAggregatesGeneration(r.Context(), identity(r), id, input.Generation, input.Aggregates)
			}
		default:
			h.method(w, r, "GET, PUT")
			return
		}
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.modified(w, r, time.Now())
		h.json(w, 200, out)
		return
	}

	id := r.PathValue("id")
	var out []string
	var err error
	switch r.Method {
	case "GET":
		out, err = h.service.Aggregates(r.Context(), identity(r), id)
	case "PUT":
		var names []string
		if err = h.decodeExisting(w, r, &names); err == nil {
			out, err = h.service.ReplaceAggregates(r.Context(), identity(r), id, names)
		}
	default:
		h.method(w, r, "GET, PUT")
		return
	}
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.modified(w, r, time.Now())
	h.json(w, 200, map[string]any{"aggregates": out})
}

func queryObject(r *http.Request) (url.Values, jsonObject, error) {
	if len(r.URL.RawQuery) > 16384 {
		return nil, nil, model.Fail(400, "Query limit exceeded.")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, nil, model.Fail(400, "Invalid query.")
	}
	object := jsonObject{}
	seen := map[string]bool{}
	for _, part := range strings.Split(r.URL.RawQuery, "&") {
		key, _ := url.QueryUnescape(strings.SplitN(part, "=", 2)[0])
		values, ok := query[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		if !utf8.ValidString(key) || !utf8.ValidString(values[len(values)-1]) {
			return nil, nil, model.Fail(400, "Invalid query.")
		}
		object = append(object, jsonMember{key, values[len(values)-1]})
	}
	return query, object, nil
}
func parseMemberOf(values []string) ([]string, error) {
	if len(values) > 1 {
		return nil, model.Fail(400, "Multiple member_of parameters are not supported")
	}
	v := values[0]
	if strings.Contains(v, ",") && !strings.HasPrefix(v, "in:") && !strings.HasPrefix(v, "!in:") {
		return nil, model.Fail(400, "Multiple values for 'member_of' must be prefixed with the 'in:' or '!in:' keyword using the valid microversion. Got: "+v)
	}
	forbidden := strings.HasPrefix(v, "!")
	v = strings.TrimPrefix(v, "!")
	v = strings.TrimPrefix(v, "in:")
	ids := strings.Split(v, ",")
	for _, id := range ids {
		if _, err := model.NormalizeUUID(id); err != nil {
			return nil, model.Fail(400, "Invalid query string parameters: Expected 'member_of' parameter to contain valid UUID(s). Got: "+id)
		}
	}
	if forbidden {
		return nil, model.Fail(400, "Forbidden member_of parameters are not supported in the specified microversion")
	}
	return ids, nil
}
func parseResources(value string) (map[string]int64, error) {
	bad := "Badly formed resources parameter. Expected resources query string parameter in form: ?resources=VCPU:2,MEMORY_MB:1024. Got: "
	if strings.TrimSpace(value) == "" {
		return nil, model.Fail(400, bad+"empty string.")
	}
	out := map[string]int64{}
	for _, part := range strings.Split(value, ",") {
		pair := strings.Split(part, ":")
		if len(pair) != 2 {
			return nil, model.Fail(400, bad+part+".")
		}
		amount, err := strconv.ParseInt(strings.TrimSpace(pair[1]), 10, 64)
		if err != nil {
			return nil, model.Fail(400, fmt.Sprintf("Requested resource %s expected positive integer amount. Got: %s.", pair[0], pair[1]))
		}
		if amount < 1 {
			return nil, model.Fail(400, fmt.Sprintf("Requested resource %s requires amount >= 1. Got: %d.", pair[0], amount))
		}
		out[pair[0]] = amount
	}
	return out, nil
}
