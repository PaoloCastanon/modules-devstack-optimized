package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

// StoreInfo is one enabled_backends entry for /v2/info/stores.
type StoreInfo struct {
	ID, Type, Description, DataDir string
	Weight                         int
	Default                        bool
}

// MultiStore is the multi-store part of service.UploadCoordinator.
type MultiStore interface {
	UploadTo(context.Context, auth.Identity, string, io.Reader, *int64, string) (model.DataContent, error)
	ImageStores(context.Context, string) ([]string, error)
	DeleteFromStore(context.Context, auth.Identity, string, string) error
}

// WithStores enables multi-store: /v2/info/stores lists the stores, uploads
// honor X-Image-Meta-Store, images show "stores" and DELETE
// /v2/stores/{store}/{image} removes one copy.
func WithStores(list []StoreInfo, ms MultiStore) Option {
	return func(h *Handler) error {
		if len(list) > 0 && ms != nil {
			h.storeList, h.multi = list, ms
		}
		return nil
	}
}

// storesView is the original's get_stores / get_stores_detail.
func (h *Handler) storesView(w http.ResponseWriter, r *http.Request) {
	detail := strings.HasSuffix(r.URL.Path, "/detail")
	if detail {
		who, _ := r.Context().Value(identityKey{}).(auth.Identity)
		def := model.Fail(model.Forbidden, "You are not authorized to complete stores_info_detail action.")
		if who.HasRole("admin") || who.HasRole("service") {
			def = nil
		}
		if err := service.Permit(who, "stores_info_detail", nil, def); err != nil {
			h.failure(w, err)
			return
		}
	}
	out := []map[string]any{}
	for _, s := range h.storeList {
		v := map[string]any{"id": s.ID}
		if s.Description != "" {
			v["description"] = s.Description
		}
		if s.Default {
			v["default"] = "true"
		}
		if detail {
			v["type"] = s.Type
			v["weight"] = s.Weight
			v["properties"] = map[string]any{"data_dir": s.DataDir, "chunk_size": 65536, "thin_provisioning": false}
		}
		out = append(out, v)
	}
	h.json(w, 200, map[string]any{"stores": out})
}

// deleteFromStore is DELETE /v2/stores/{store}/{id}.
func (h *Handler) deleteFromStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		h.status(w, 405, "Method not implemented.")
		return
	}
	if h.multi == nil {
		h.status(w, 400, "Multi backend is not supported at this site.")
		return
	}
	who := r.Context().Value(identityKey{}).(auth.Identity)
	if err := h.multi.DeleteFromStore(r.Context(), who, r.PathValue("store"), r.PathValue("id")); err != nil {
		h.failure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// withStores adds the original's "stores" (the image's backends, comma
// separated) to an image representation.
func (h *Handler) withStores(r *http.Request, rep map[string]any) map[string]any {
	if h.multi == nil {
		return rep
	}
	id, _ := rep["id"].(string)
	if list, err := h.multi.ImageStores(r.Context(), id); err == nil && len(list) > 0 {
		rep["stores"] = strings.Join(list, ",")
	}
	return rep
}
