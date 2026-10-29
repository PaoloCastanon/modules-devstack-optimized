// Package api implements the selected Placement 1.0 routes; it contains no SQL.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/service"
	"github.com/google/uuid"
)

const Version = "1.36"
const MinVersion = "1.0"

type identityKey struct{}
type Handler struct {
	service   *service.Service
	auth      auth.Authenticator
	base      *url.URL
	challenge string
	limit     int64
	logger    *slog.Logger
}

func New(s *service.Service, a auth.Authenticator, publicURL, keystoneURL string, limit int64, logger *slog.Logger) (http.Handler, error) {
	if err := config.ValidateURL(publicURL); err != nil {
		return nil, err
	}
	if err := config.ValidateURL(keystoneURL); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.TrimRight(keystoneURL, "/"), "/v3") || limit < 1 || s == nil || a == nil || logger == nil {
		return nil, errors.New("invalid Placement handler configuration")
	}
	base, _ := url.Parse(strings.TrimRight(publicURL, "/"))
	h := &Handler{s, a, base, "Keystone uri=" + strconv.Quote(strings.TrimSuffix(strings.TrimRight(keystoneURL, "/"), "/v3")), limit, logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/allocation_candidates", h.candidates)
	mux.HandleFunc("/allocations/{consumer}", h.consumerAllocations)
	mux.HandleFunc("/resource_providers/{id}/allocations", h.providerAllocations)
	mux.HandleFunc("/resource_providers/{id}/usages", h.usages)
	mux.HandleFunc("/resource_classes", h.classes)
	mux.HandleFunc("/resource_classes/{name}", h.class)
	mux.HandleFunc("/traits", h.traits)
	mux.HandleFunc("/traits/{name}", h.trait)
	mux.HandleFunc("/resource_providers/{id}/traits", h.providerTraits)
	mux.HandleFunc("/resource_providers/{id}/aggregates", h.aggregates)
	mux.HandleFunc("/resource_providers", h.providers)
	mux.HandleFunc("/resource_providers/{id}", h.provider)
	mux.HandleFunc("/resource_providers/{id}/inventories", h.inventories)
	mux.HandleFunc("/resource_providers/{id}/inventories/{class}", h.inventory)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			h.discovery(w, r)
		} else {
			h.failure(w, r, model.Fail(404, "The resource could not be found."))
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id, err := uuid.NewRandom()
		if err != nil {
			h.failure(w, r, err)
			return
		}
		w.Header().Set("X-Openstack-Request-Id", "req-"+id.String())
		w.Header().Set("Vary", "OpenStack-API-Version")
		w.Header().Set("OpenStack-API-Version", "placement "+MinVersion)
		out := &observed{ResponseWriter: w}
		w = out
		defer func() {
			method := r.Method
			switch method {
			case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
			default:
				method = "OTHER"
			}
			h.logger.Info("request completed", "request_id", "req-"+id.String(), "method", method, "route", route(r.URL.Path), "status", out.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		if r.URL.Path != "/" {
			tokens := r.Header.Values("X-Auth-Token")
			if len(tokens) != 1 || tokens[0] == "" {
				h.failure(w, r, auth.ErrInvalidToken)
				return
			}
			who, err := h.auth.Authenticate(r.Context(), tokens[0])
			if err != nil {
				// A validated token without project scope still receives the
				// versioned policy denial. It never reaches the repository.
				if errors.Is(err, auth.ErrForbidden) {
					version, versionErr := negotiate(r.Header.Values("OpenStack-API-Version"))
					if versionErr != nil {
						h.failure(w, r, versionErr)
						return
					}
					r = r.WithContext(context.WithValue(r.Context(), versionKey{}, version))
					w.Header().Set("OpenStack-API-Version", fmt.Sprintf("placement 1.%d", version))
				}
				h.failure(w, r, err)
				return
			}
			if err = who.Check(time.Now()); err != nil {
				h.failure(w, r, err)
				return
			}
			if len(r.Header.Values("X-Service-Token")) != 0 {
				h.failure(w, r, model.Fail(403, "Composite service authentication is not supported."))
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, who))
		}
		version, err := negotiate(r.Header.Values("OpenStack-API-Version"))
		if err != nil {
			h.failure(w, r, err)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), versionKey{}, version))
		w.Header().Set("OpenStack-API-Version", fmt.Sprintf("placement 1.%d", version))
		if version > profileMaximum(r) {
			h.failure(w, r, model.Fail(406, fmt.Sprintf("This route currently supports microversions 1.0 - 1.%d.", profileMaximum(r))))
			return
		}

		if route(r.URL.Path) != "/resource_providers/{id}/aggregates" && version < routeMinimum(r) {
			h.failure(w, r, model.Fail(404, ""))
			return
		}
		if r.Method == "GET" && !acceptsJSON(r.Header.Get("Accept")) {
			h.notAcceptable(w, r)
			return
		}
		if requiresJSON(r) {
			if err := requireJSON(r); err != nil {
				h.failure(w, r, err)
				return
			}
		}
		if version < routeMinimum(r) {
			h.failure(w, r, model.Fail(404, ""))
			return
		}
		if r.URL.Path != "/" && permission(r) != "" {
			who := identity(r)
			if !who.HasRole("admin") && !who.HasRole("service") {
				h.failure(w, r, model.Fail(403, "Placement requires the admin or service role."))
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}

type versionKey struct{}

func microversion(r *http.Request) int { v, _ := r.Context().Value(versionKey{}).(int); return v }
func routeMinimum(r *http.Request) int {
	switch route(r.URL.Path) {
	case "/allocation_candidates":
		return 10
	case "/resource_classes", "/resource_classes/{name}":
		return 2
	case "/traits", "/traits/{name}", "/resource_providers/{id}/traits":
		return 6
	case "/resource_providers/{id}/aggregates":
		return 1
	}
	return 0
}
func negotiate(values []string) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 {
		return 0, model.Fail(400, "Invalid microversion header.")
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || parts[0] != "placement" {
		return 0, model.Fail(400, "Invalid microversion header.")
	}
	if parts[1] == "latest" {
		return 36, nil
	}
	v := strings.Split(parts[1], ".")
	if len(v) != 2 {
		return 0, model.Fail(400, "Invalid microversion.")
	}
	for _, part := range v {
		if part == "" {
			return 0, model.Fail(400, "Invalid microversion.")
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, model.Fail(400, "Invalid microversion.")
			}
		}
	}
	major, e1 := strconv.Atoi(v[0])
	minor, e2 := strconv.Atoi(v[1])
	if e1 != nil || e2 != nil || major != 1 || minor > 36 {
		return 0, model.Fail(406, "Requested microversion is outside the supported range 1.0 - "+Version+".")
	}
	return minor, nil
}
func acceptsJSON(header string) bool {
	if header == "" {
		return true
	}
	bestSpecificity := -1
	quality := 0.0
	for _, part := range strings.Split(header, ",") {
		m, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		q := 1.0
		if value, ok := params["q"]; ok {
			q, err = strconv.ParseFloat(value, 64)
			if err != nil || q < 0 || q > 1 {
				continue
			}
		}
		specificity := -1
		switch m {
		case "*/*":
			specificity = 0
		case "application/*":
			specificity = 1
		case "application/json":
			specificity = 2
		}
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			quality = q
		}
	}
	return bestSpecificity >= 0 && quality > 0
}
func permission(r *http.Request) string {
	if p := allocationPermission(r); p != "" {
		return p
	}
	if p := catalogPermission(r); p != "" {
		return p
	}
	prefix := "placement:resource_providers:"
	action := ""
	switch route(r.URL.Path) {
	case "/resource_providers":
		if r.Method == "GET" {
			action = "list"
		}
		if r.Method == "POST" {
			action = "create"
		}
	case "/resource_providers/{id}":
		switch r.Method {
		case "GET":
			action = "show"
		case "PUT":
			action = "update"
		case "DELETE":
			action = "delete"
		}
	case "/resource_providers/{id}/inventories":
		prefix += "inventories:"
		if r.Method == "DELETE" && microversion(r) >= 5 {
			action = "delete"
		}
		if r.Method == "GET" {
			action = "list"
		}
		if r.Method == "PUT" {
			action = "update"
		}
	case "/resource_providers/{id}/inventories/{class}":
		prefix += "inventories:"
		if r.Method == "GET" {
			action = "show"
		}
	}
	if action == "" {
		return ""
	}
	return prefix + action
}

func route(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if path == "/" {
		return "/"
	}
	if path == "/allocation_candidates" {
		return path
	}
	if parts[0] == "resource_classes" || parts[0] == "traits" {
		if len(parts) == 1 {
			return "/" + parts[0]
		}
		if len(parts) == 2 {
			return "/" + parts[0] + "/{name}"
		}
	}
	if parts[0] == "allocations" && len(parts) == 2 {
		return "/allocations/{consumer}"
	}
	if parts[0] != "resource_providers" {
		return "unmatched"
	}
	switch len(parts) {
	case 1:
		return "/resource_providers"
	case 2:
		return "/resource_providers/{id}"
	case 3:
		if parts[2] == "traits" || parts[2] == "aggregates" || parts[2] == "allocations" || parts[2] == "usages" {
			return "/resource_providers/{id}/" + parts[2]
		}
		if parts[2] == "inventories" {
			return "/resource_providers/{id}/inventories"
		}
	case 4:
		if parts[2] == "inventories" {
			return "/resource_providers/{id}/inventories/{class}"
		}
	}
	return "unmatched"
}

type observed struct {
	http.ResponseWriter
	status int
}

func (w *observed) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *observed) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *observed) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func (h *Handler) failure(w http.ResponseWriter, r *http.Request, err error) {
	status := model.Status(err)
	detail := "Placement service unavailable."
	var e *model.Error
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		status = 401
		w.Header().Del("OpenStack-API-Version")
		w.Header().Del("Vary")
		w.Header().Set("WWW-Authenticate", h.challenge)
		h.json(w, 401, map[string]any{"error": map[string]any{"code": 401, "title": "Unauthorized", "message": "The request you have made requires authentication."}})
		return
	case errors.Is(err, auth.ErrForbidden):
		status = 403
		detail = "Project scope is required."
		if policy := permission(r); policy != "" {
			detail = "Policy does not allow " + policy + " to be performed."
		}
	case errors.As(err, &e):
		detail = e.Detail
	}
	if status == 403 {
		w.Header().Del("Vary")
		if detail == "Placement requires the admin or service role." && permission(r) != "" {
			detail = "Policy does not allow " + permission(r) + " to be performed."
		}
	}
	if status == 404 && (route(r.URL.Path) == "/resource_providers/{id}" && (r.Method == "GET" || r.Method == "PUT") || route(r.URL.Path) == "/resource_providers/{id}/inventories" && r.Method == "PUT") {
		w.Header().Del("Vary")
	}
	// These lookups escape the versioned handler as domain NotFound in Python.
	// A version-floor 404 is raised inside the handler and retains Vary.
	if status == 404 && detail != "" && (route(r.URL.Path) == "/resource_classes/{name}" || route(r.URL.Path) == "/resource_providers/{id}/aggregates" || route(r.URL.Path) == "/resource_providers/{id}/traits" && r.Method != "GET") {
		w.Header().Del("Vary")
	}
	prefixes := map[int]string{400: "The server could not comply with the request since it is either malformed or otherwise incorrect.", 403: "Access was denied to this resource.", 404: "The resource could not be found.", 409: "There was a conflict when trying to complete your request."}
	if prefix, ok := prefixes[status]; ok {
		if detail == "" {
			detail = prefix + "\n\n   "
		} else {
			detail = prefix + "\n\n " + detail + "  "
		}
	}
	body := map[string]any{"status": status, "title": http.StatusText(status), "detail": detail}
	if microversion(r) >= 23 {
		code := "placement.undefined_code"
		if e != nil && e.Code != "" {
			code = e.Code
		}
		body["code"] = code
	}
	if id := w.Header().Get("X-Openstack-Request-Id"); id != "" {
		body["request_id"] = id
	}
	h.json(w, status, map[string]any{"errors": []any{body}})
}
func (h *Handler) method(w http.ResponseWriter, r *http.Request, allow string) {
	w.Header().Set("Allow", allow)
	h.failure(w, r, model.Fail(405, "Method not allowed."))
}

// Match the captured WebOb text error for an unsatisfied Accept request.
// Keep this separate from microversion 406 responses.
func (h *Handler) notAcceptable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusNotAcceptable)
	_, _ = fmt.Fprintf(w, "406 Not Acceptable\n\nThe resource could not be generated that was acceptable to your browser (content of type %s. \n\n Only application/json is provided", r.Header.Get("Accept"))
}

func requiresJSON(r *http.Request) bool {
	path := route(r.URL.Path)
	return r.Method == "POST" && (path == "/resource_providers" || path == "/resource_classes") || r.Method == "PUT" && (path == "/allocations/{consumer}" || path == "/resource_providers/{id}" || path == "/resource_providers/{id}/inventories" || path == "/resource_providers/{id}/traits" || path == "/resource_providers/{id}/aggregates" || path == "/resource_classes/{name}" && microversion(r) < 7)
}

func requireJSON(r *http.Request) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		if media == "" {
			media = strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])
		}
		if media == "" {
			media = "None"
		}
		return model.Fail(415, fmt.Sprintf("The request media type %s is not supported by this server. \n\n The media type %s is not supported, use application/json", media, media))
	}
	return nil
}

// Upstream checks media type, then provider existence, before reading/validating
// a PUT body. The repository still locks and checks generation at commit time.
func (h *Handler) decodeExisting(w http.ResponseWriter, r *http.Request, target any, order ...*[]string) error {
	if err := requireJSON(r); err != nil {
		return err
	}
	if _, err := h.service.Get(r.Context(), identity(r), r.PathValue("id")); err != nil {
		return err
	}
	return h.decode(w, r, target, order...)
}

func (h *Handler) decode(w http.ResponseWriter, r *http.Request, target any, inventoryOrder ...*[]string) error {
	if err := requireJSON(r); err != nil {
		return err
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.limit))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return model.Fail(413, "Request body limit exceeded.")
		}
		return model.Fail(400, "Cannot read request body.")
	}
	if !utf8.Valid(data) {
		return model.Fail(400, "Request body must be UTF-8.")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return model.Fail(400, "Expected a JSON object.")
	}
	raw, err := orderedJSON(data)
	if err != nil {
		return model.Fail(400, "Invalid JSON object.")
	}
	schema := bodySchema(r)
	if route(r.URL.Path) == "/resource_providers" {
		schema = "create"
		if microversion(r) >= 14 {
			schema = "create_1_14"
		}
	}
	if route(r.URL.Path) == "/resource_providers/{id}" {
		schema = "rename"
		if microversion(r) >= 14 {
			schema = "rename_1_14"
		}
	}
	if err := validateRequest(raw, schema, "JSON does not validate: "); err != nil {
		return err
	}
	normalized, err := schemaIntegers(plainJSON(raw), reflect.TypeOf(target))
	if err != nil {
		return model.Fail(400, "Invalid request fields or types.")
	}
	data, err = json.Marshal(normalized)
	if err != nil {
		return model.Fail(400, "Invalid request fields or types.")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return model.Fail(400, "Invalid request fields or types.")
	}
	if schema == "inventories" && len(inventoryOrder) > 0 {
		for _, member := range raw.(jsonObject).field("inventories").(jsonObject) {
			*inventoryOrder[0] = append(*inventoryOrder[0], member.key)
		}
	}
	return nil
}

func identity(r *http.Request) auth.Identity { return r.Context().Value(identityKey{}).(auth.Identity) }
func (h *Handler) discovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	h.json(w, 200, map[string]any{"versions": []any{map[string]any{"id": "v1.0", "max_version": Version, "min_version": MinVersion, "status": "CURRENT", "links": []any{map[string]string{"rel": "self", "href": ""}}}}})
}
func (h *Handler) representation(r *http.Request, p model.Provider) map[string]any {
	href := strings.TrimRight(h.base.Path, "/") + "/resource_providers/" + p.UUID
	links := []map[string]string{}
	rels := []string{"self", "inventories", "usages"}
	if microversion(r) >= 1 {
		rels = append(rels, "aggregates")
	}
	if microversion(r) >= 6 {
		rels = append(rels, "traits")
	}
	if microversion(r) >= 11 {
		rels = append(rels, "allocations")
	}
	for _, rel := range rels {
		link := href
		if rel != "self" {
			link += "/" + rel
		}
		links = append(links, map[string]string{"rel": rel, "href": link})
	}
	out := map[string]any{"uuid": p.UUID, "name": p.Name, "generation": p.Generation, "links": links}
	if microversion(r) >= 14 {
		out["parent_provider_uuid"] = nil
		out["root_provider_uuid"] = p.UUID
	}
	return out
}
func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		var input struct {
			Name   *string `json:"name"`
			UUID   *string `json:"uuid"`
			Parent *string `json:"parent_provider_uuid"`
		}
		if err := h.decode(w, r, &input); err != nil {
			h.failure(w, r, err)
			return
		}
		if input.Parent != nil {
			h.failure(w, r, model.Fail(400, "Nested providers are outside the flat-provider profile."))
			return
		}
		if input.Name == nil {
			h.failure(w, r, model.Fail(400, "Provider name is required."))
			return
		}
		id := ""
		if input.UUID != nil {
			id = *input.UUID
			if _, err := model.NormalizeUUID(id); err != nil {
				h.failure(w, r, err)
				return
			}
		}
		p, err := h.service.Create(r.Context(), identity(r), *input.Name, id)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		w.Header().Set("Location", h.base.JoinPath("resource_providers", p.UUID).String())
		if microversion(r) >= 20 {
			h.modified(w, r, p.LastModified)
			h.json(w, 200, h.representation(r, p))
		} else {
			w.WriteHeader(201)
		}
	case "GET":
		if len(r.URL.RawQuery) > 16384 {
			h.failure(w, r, model.Fail(400, "Query limit exceeded."))
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			h.failure(w, r, model.Fail(400, "Invalid query."))
			return
		}
		queryObject := jsonObject{}
		seen := map[string]bool{}
		for _, part := range strings.Split(r.URL.RawQuery, "&") {
			key, _ := url.QueryUnescape(strings.SplitN(part, "=", 2)[0])
			values, exists := query[key]
			if !exists || seen[key] {
				continue
			}
			seen[key] = true
			value := values[len(values)-1]
			if !utf8.ValidString(value) || !utf8.ValidString(key) {
				h.failure(w, r, model.Fail(400, "Invalid query."))
				return
			}
			queryObject = append(queryObject, jsonMember{key, value})
		}
		if err := validateRequest(queryObject, providerQuerySchema(r), "Invalid query string parameters: "); err != nil {
			h.failure(w, r, err)
			return
		}
		f := model.Filter{}
		for key, values := range query {
			if len(values) == 0 || !utf8.ValidString(values[len(values)-1]) {
				h.failure(w, r, model.Fail(400, "Invalid query."))
				return
			}
			v := values[len(values)-1]
			switch key {
			case "name":
				f.Name = &v
			case "uuid":
				f.UUID = &v
			case "in_tree":
				f.InTree = &v
			case "required":
				f.Required, f.Forbidden, err = parseTraits(v, microversion(r) >= 22)
				if err != nil {
					h.failure(w, r, err)
					return
				}
			case "member_of":
				f.Aggregates, err = parseMemberOf(values)
				if err != nil {
					h.failure(w, r, err)
					return
				}
			case "resources":
				f.Resources, err = parseResources(v)
				if err != nil {
					h.failure(w, r, err)
					return
				}
			default:
				h.failure(w, r, model.Fail(400, "Unsupported query parameter at microversion 1.0."))
				return
			}
		}
		providers, err := h.service.List(r.Context(), identity(r), f)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		out := []map[string]any{}
		for _, p := range providers {
			out = append(out, h.representation(r, p))
		}
		modified := time.Time{}
		for _, p := range providers {
			if p.LastModified.After(modified) {
				modified = p.LastModified
			}
		}
		h.modified(w, r, modified)
		h.json(w, 200, map[string]any{"resource_providers": out})
	default:
		h.method(w, r, "GET, POST")
	}
}
func (h *Handler) provider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.Method {
	case "GET":
		p, err := h.service.Get(r.Context(), identity(r), id)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.modified(w, r, p.LastModified)
		h.json(w, 200, h.representation(r, p))
	case "PUT":
		var input struct {
			Name   *string `json:"name"`
			Parent *string `json:"parent_provider_uuid"`
		}
		if err := h.decodeExisting(w, r, &input); err != nil {
			h.failure(w, r, err)
			return
		}
		if input.Parent != nil {
			h.failure(w, r, model.Fail(400, "Nested providers are outside the flat-provider profile."))
			return
		}
		if input.Name == nil {
			h.failure(w, r, model.Fail(400, "Provider name is required."))
			return
		}
		p, err := h.service.Rename(r.Context(), identity(r), id, *input.Name)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		h.modified(w, r, p.LastModified)
		h.json(w, 200, h.representation(r, p))
	case "DELETE":
		if err := h.service.Delete(r.Context(), identity(r), id); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	default:
		h.method(w, r, "GET, PUT, DELETE")
	}
}
func (h *Handler) inventories(w http.ResponseWriter, r *http.Request) {
	var result model.Inventories
	var err error
	switch r.Method {
	case "DELETE":
		if microversion(r) < 5 {
			h.failure(w, r, model.Fail(405, "The method DELETE is not allowed for this resource. \n\n "))
			return
		}
		if err := h.service.DeleteInventories(r.Context(), identity(r), r.PathValue("id")); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
		return
	case "GET":
		result, err = h.service.Inventories(r.Context(), identity(r), r.PathValue("id"))
	case "PUT":
		var input struct {
			Generation  *int64                          `json:"resource_provider_generation"`
			Inventories map[string]model.InventoryInput `json:"inventories"`
		}
		var classOrder []string
		if err = h.decodeExisting(w, r, &input, &classOrder); err == nil {
			if input.Generation == nil || input.Inventories == nil {
				err = model.Fail(400, "Generation and inventories are required.")
			} else {
				result, err = h.service.ReplaceInventoriesVersion(r.Context(), identity(r), r.PathValue("id"), *input.Generation, input.Inventories, microversion(r) >= 26, classOrder...)
			}
		}
	default:
		h.method(w, r, "GET, PUT")
		return
	}
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.modified(w, r, result.LastModified)
	h.json(w, 200, result)
}
func (h *Handler) inventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	invs, err := h.service.InventorySnapshot(r.Context(), identity(r), r.PathValue("id"), r.PathValue("class"))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	inv, ok := invs.Inventories[r.PathValue("class")]
	if !ok {
		h.failure(w, r, model.Fail(404, "No inventory of class "+r.PathValue("class")+" for "+r.PathValue("id")))
		return
	}
	data, _ := json.Marshal(inv)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	if invs.Generation != 0 {
		out["resource_provider_generation"] = invs.Generation
	}
	h.modified(w, r, invs.LastModified)
	h.json(w, 200, out)
}

// Version profiles are deliberately narrower than the entire upstream API.
// These cover the concrete versions used by the pinned Nova resource tracker.
func profileMaximum(r *http.Request) int {
	switch route(r.URL.Path) {
	case "/":
		return 36
	case "/allocations/{consumer}", "/resource_providers/{id}/allocations", "/allocation_candidates":
		return 36
	case "/resource_providers", "/resource_providers/{id}":
		return 20
	case "/resource_providers/{id}/inventories", "/resource_providers/{id}/inventories/{class}":
		return 26
	case "/resource_providers/{id}/aggregates":
		return 19
	}
	return 8
}
