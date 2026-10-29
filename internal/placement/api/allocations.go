package api

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"net/http"
	"time"
)

func allocationPermission(r *http.Request) string {
	switch route(r.URL.Path) {
	case "/allocation_candidates":
		if r.Method == "GET" {
			return "placement:allocation_candidates:list"
		}
	case "/allocations/{consumer}":
		if action := map[string]string{"GET": "list", "PUT": "update", "DELETE": "delete"}[r.Method]; action != "" {
			return "placement:allocations:" + action
		}
	case "/resource_providers/{id}/allocations":
		if r.Method == "GET" {
			return "placement:resource_providers:allocations:list"
		}
	case "/resource_providers/{id}/usages":
		if r.Method == "GET" {
			return "placement:resource_providers:usages"
		}
	}
	return ""
}
func (h *Handler) consumerAllocations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("consumer")
	switch r.Method {
	case "GET":
		out, err := h.service.ConsumerAllocations(r.Context(), identity(r), id)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		if microversion(r) < 12 {
			out.ProjectID = nil
			out.UserID = nil
		}
		if microversion(r) < 28 {
			out.Generation = nil
		}
		h.modified(w, r, out.LastModified)
		h.json(w, 200, out)
	case "PUT":
		// The original validates the consumer path before decoding JSON.
		if _, err := model.NormalizeUUID(id); err != nil {
			h.failure(w, r, model.Fail(400, "Malformed consumer_uuid: "+id))
			return
		}
		if microversion(r) >= 12 {
			var body model.ModernAllocationRequest
			if err := h.decode(w, r, &body); err != nil {
				h.failure(w, r, err)
				return
			}
			if err := h.service.ReplaceModernAllocations(r.Context(), identity(r), id, body, microversion(r) >= 28); err != nil {
				h.failure(w, r, err)
				return
			}
			w.WriteHeader(204)
			return
		}
		var body model.AllocationRequest
		if err := h.decode(w, r, &body); err != nil {
			h.failure(w, r, err)
			return
		}
		if err := h.service.ReplaceAllocations(r.Context(), identity(r), id, body); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	case "DELETE":
		if err := h.service.DeleteAllocations(r.Context(), identity(r), id); err != nil {
			h.failure(w, r, err)
			return
		}
		w.WriteHeader(204)
	default:
		h.method(w, r, "GET, PUT, DELETE")
	}
}
func (h *Handler) providerAllocations(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	out, err := h.service.ProviderAllocations(r.Context(), identity(r), r.PathValue("id"))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if microversion(r) < 28 {
		for id, item := range out.Allocations {
			item.Generation = nil
			out.Allocations[id] = item
		}
	}
	h.modified(w, r, out.LastModified)
	h.json(w, 200, out)
}
func (h *Handler) usages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		h.method(w, r, "GET")
		return
	}
	out, err := h.service.Usages(r.Context(), identity(r), r.PathValue("id"))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	h.json(w, 200, out)
}

func (h *Handler) modified(w http.ResponseWriter, r *http.Request, stamp time.Time) {
	if microversion(r) < 15 {
		return
	}
	if stamp.IsZero() {
		stamp = time.Now()
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Last-Modified", stamp.UTC().Format(http.TimeFormat))
}
