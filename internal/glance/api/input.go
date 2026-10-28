package api

import (
	"bytes"
	"encoding/json"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

// Decoding the object as RawMessage retains absent vs explicit null. In the
// domain, the two deliberately converge only for the nullable string fields.
func field[T any](raw map[string]json.RawMessage, key string, nullable bool) (*T, error) {
	value, exists := raw[key]
	if !exists {
		return nil, nil
	}
	delete(raw, key)
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		if nullable {
			return nil, nil
		}
		return nil, model.Fail(model.Invalid, key+" cannot be null.")
	}
	var target T
	if json.Unmarshal(value, &target) != nil {
		return nil, model.Fail(model.Invalid, "Invalid JSON type for "+key+".")
	}
	return &target, nil
}

func decodeCreate(raw map[string]json.RawMessage) (model.CreateInput, error) {
	input := model.CreateInput{Properties: map[string]string{}}
	for _, key := range []string{"status", "size", "virtual_size", "checksum", "os_hash_algo", "os_hash_value", "created_at", "updated_at", "self", "file", "schema", "direct_url", "stores", "location", "locations", "deleted", "deleted_at"} {
		if _, exists := raw[key]; exists {
			return input, model.Fail(model.Forbidden, "Attribute is read-only or reserved: "+key)
		}
	}
	var err error
	for _, item := range []struct {
		key      string
		target   **string
		nullable bool
	}{
		{"id", &input.ID, false}, {"name", &input.Name, true}, {"disk_format", &input.DiskFormat, true},
		{"container_format", &input.ContainerFormat, true}, {"visibility", &input.Visibility, false}, {"owner", &input.Owner, false},
	} {
		*item.target, err = field[string](raw, item.key, item.nullable)
		if err != nil {
			return input, err
		}
	}
	input.MinDisk, err = field[int64](raw, "min_disk", false)
	if err != nil {
		return input, err
	}
	input.MinRAM, err = field[int64](raw, "min_ram", false)
	if err != nil {
		return input, err
	}
	input.Protected, err = field[bool](raw, "protected", false)
	if err != nil {
		return input, err
	}
	input.Hidden, err = field[bool](raw, "os_hidden", false)
	if err != nil {
		return input, err
	}
	tags, err := field[[]json.RawMessage](raw, "tags", false)
	if err != nil {
		return input, err
	}
	if tags != nil {
		for _, value := range *tags {
			var tag string
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &tag) != nil {
				return input, model.Fail(model.Invalid, "Tags must be strings.")
			}
			input.Tags = append(input.Tags, tag)
		}
	}
	for key, value := range raw {
		if model.Reserved(key) {
			return input, model.Fail(model.Forbidden, "Property is reserved: "+key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return input, model.Fail(model.Invalid, "Additional properties must be strings.")
		}
		var property string
		if json.Unmarshal(value, &property) != nil {
			return input, model.Fail(model.Invalid, "Additional properties must be strings.")
		}
		input.Properties[key] = property
	}
	return input, nil
}
