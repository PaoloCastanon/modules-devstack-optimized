package api

import (
	"net/http"
	"strings"
)

// These schemas describe only the selected metadata representation. They do
// not advertise import, locations, stores, metadefs or transfer operations.
func imageSchema() map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"id", "owner", "name", "disk_format", "container_format", "status", "visibility", "checksum", "os_hash_algo", "os_hash_value", "created_at", "updated_at", "self", "file", "schema"} {
		properties[key] = map[string]any{"type": "string"}
	}
	for _, key := range []string{"name", "disk_format", "container_format", "checksum", "os_hash_algo", "os_hash_value"} {
		properties[key] = map[string]any{"type": []string{"string", "null"}}
	}
	properties["name"].(map[string]any)["maxLength"] = 255
	properties["id"].(map[string]any)["pattern"] = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	for key, values := range map[string]string{"disk_format": "ami ari aki vhd vhdx vmdk raw qcow2 vdi iso ploop", "container_format": "ami ari aki bare ovf ova docker compressed", "visibility": "private shared public community", "status": "queued saving active killed deleted pending_delete deactivated uploading importing"} {
		enum := []any{}
		for _, v := range strings.Fields(values) {
			enum = append(enum, v)
		}
		if key == "disk_format" || key == "container_format" {
			enum = append(enum, nil)
		}
		properties[key].(map[string]any)["enum"] = enum
	}
	for _, key := range []string{"min_disk", "min_ram"} {
		properties[key] = map[string]any{"type": "integer", "minimum": 0, "maximum": 2147483647}
	}
	for _, key := range []string{"size", "virtual_size"} {
		properties[key] = map[string]any{"type": []string{"integer", "null"}, "minimum": 0}
	}
	for _, key := range []string{"protected", "os_hidden"} {
		properties[key] = map[string]any{"type": "boolean"}
	}
	properties["tags"] = map[string]any{"type": "array", "items": map[string]any{"type": "string", "maxLength": 255}, "uniqueItems": true, "maxItems": 128}
	for _, key := range []string{"status", "size", "virtual_size", "checksum", "os_hash_algo", "os_hash_value", "created_at", "updated_at", "self", "file", "schema"} {
		properties[key].(map[string]any)["readOnly"] = true
	}
	return map[string]any{"name": "image", "properties": properties, "additionalProperties": map[string]any{"type": "string"}, "links": []map[string]string{{"rel": "self", "href": "{self}"}, {"rel": "describedby", "href": "{schema}"}, {"rel": "enclosure", "href": "{file}"}}}
}

func (h *Handler) schema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.status(w, 405, "Method not implemented.")
		return
	}
	switch r.PathValue("name") {
	case "image":
		h.json(w, 200, imageSchema())
	case "images":
		h.json(w, 200, map[string]any{"name": "images", "properties": map[string]any{"images": map[string]any{"type": "array", "items": imageSchema()}, "first": map[string]string{"type": "string"}, "next": map[string]string{"type": "string"}, "schema": map[string]string{"type": "string"}}, "links": []map[string]string{{"rel": "first", "href": "{first}"}, {"rel": "next", "href": "{next}"}, {"rel": "describedby", "href": "{schema}"}}})
	default:
		h.status(w, 404, "Schema not implemented.")
	}
}
