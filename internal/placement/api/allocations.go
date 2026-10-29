package api

import (
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
	"net/http"
)

func allocationPermission(r *http.Request) string {
	switch route(r.URL.Path) {
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
		h.json(w, 200, out)
	case "PUT":
		// The original validates the consumer path before decoding JSON.
		if _, err := model.NormalizeUUID(id); err != nil {
			h.failure(w, r, model.Fail(400, "Malformed consumer_uuid: "+id))
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
