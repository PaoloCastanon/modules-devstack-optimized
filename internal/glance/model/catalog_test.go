package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func patch(op, key, value string) PatchOperation {
	return PatchOperation{Op: op, Key: key, Value: json.RawMessage(value)}
}
func TestPatchOperationsAndAtomicValidation(t *testing.T) {
	name := "original"
	now := time.Now().UTC().Truncate(time.Microsecond)
	image, err := New(CreateInput{Name: &name, Properties: map[string]string{"x": "one"}, Tags: []string{"old"}}, "owner", now)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		ops  []PatchOperation
		kind Kind
	}{
		{"add", []PatchOperation{patch("add", "new", `"value"`)}, 0},
		{"add replaces existing v2.1", []PatchOperation{patch("add", "x", `"two"`)}, 0},
		{"replace", []PatchOperation{patch("replace", "x", `"two"`)}, 0},
		{"remove", []PatchOperation{patch("remove", "x", ``)}, 0},
		{"nullable name", []PatchOperation{patch("replace", "name", `null`)}, 0},
		{"empty name", []PatchOperation{patch("replace", "name", `""`)}, 0},
		{"whole tags", []PatchOperation{patch("replace", "tags", `["new","new"]`)}, 0},
		{"zero false", []PatchOperation{patch("replace", "min_ram", `0`), patch("replace", "protected", `false`)}, 0},
		{"empty patch", nil, 0},
		{"missing replace", []PatchOperation{patch("replace", "absent", `"v"`)}, Conflict},
		{"missing remove", []PatchOperation{patch("remove", "absent", ``)}, Conflict},
		{"base removal", []PatchOperation{patch("remove", "name", ``)}, Forbidden},
		{"immutable id", []PatchOperation{patch("replace", "id", `"bad"`)}, Forbidden},
		{"immutable owner", []PatchOperation{patch("replace", "owner", `"bad"`)}, Forbidden},
		{"calculated", []PatchOperation{patch("add", "size", `5`)}, Forbidden},
		{"reserved", []PatchOperation{patch("add", "os_glance_secret", `"x"`)}, Forbidden},
		{"unsupported", []PatchOperation{patch("test", "x", `"one"`)}, Invalid},
		{"null property", []PatchOperation{patch("add", "x", `null`)}, Invalid},
		{"missing value", []PatchOperation{patch("add", "x", ``)}, Invalid},
		{"wrong numeric type", []PatchOperation{patch("replace", "min_ram", `"1"`)}, Invalid},
		{"fractional number", []PatchOperation{patch("replace", "min_ram", `1.2`)}, Invalid},
		{"negative", []PatchOperation{patch("replace", "min_ram", `-1`)}, Invalid},
		{"invalid visibility", []PatchOperation{patch("replace", "visibility", `"other"`)}, Invalid},
		{"invalid format", []PatchOperation{patch("replace", "disk_format", `"other"`)}, Invalid},
		{"null bool", []PatchOperation{patch("replace", "protected", `null`)}, Invalid},
		{"null tags", []PatchOperation{patch("replace", "tags", `null`)}, Invalid},
		{"null tag", []PatchOperation{patch("replace", "tags", `[null]`)}, Invalid},
		{"wrong tag", []PatchOperation{patch("replace", "tags", `[4]`)}, Invalid},
		{"atomic property", []PatchOperation{patch("replace", "x", `"changed"`), patch("remove", "absent", ``)}, Conflict},
		{"atomic nullable pointer", []PatchOperation{patch("replace", "name", `"changed"`), patch("replace", "min_ram", `-1`)}, Invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyPatch(image, tc.ops, now)
			if tc.kind == 0 {
				if err != nil {
					t.Fatal(err)
				}
				if !got.UpdatedAt.After(image.UpdatedAt) || got.Status != "queued" || got.ID != image.ID || !got.CreatedAt.Equal(image.CreatedAt) {
					t.Fatal("identity/state/time changed incorrectly")
				}
			} else {
				var e *Error
				if !errors.As(err, &e) || e.Kind != tc.kind {
					t.Fatalf("want %v: %v", tc.kind, err)
				}
			}
			if *image.Name != "original" || !reflect.DeepEqual(image.Tags, []string{"old"}) || !reflect.DeepEqual(image.Properties, map[string]string{"x": "one"}) {
				t.Fatal("input mutated, including after failure")
			}
		})
	}
	image.Status = "active"
	if _, err = ApplyPatch(image, []PatchOperation{patch("replace", "disk_format", `"raw"`)}, now); err == nil {
		t.Fatal("changed immutable active format")
	}
}

func TestListOptionsValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		o     ListOptions
		valid bool
	}{
		{"default", ListOptions{Limit: 25}, true}, {"zero", ListOptions{Limit: 0}, true}, {"clamped", ListOptions{Limit: 99999}, true},
		{"negative", ListOptions{Limit: -1}, false}, {"marker", ListOptions{Marker: "bad"}, false},
		{"sort injection", ListOptions{Sort: []Sort{{"id; DROP TABLE images", "asc"}}}, false},
		{"direction injection", ListOptions{Sort: []Sort{{"id", "asc;SELECT"}}}, false},
		{"duplicate sort", ListOptions{Sort: []Sort{{"id", "asc"}, {"id", "desc"}}}, false},
		{"filter all", ListOptions{Filters: map[string]string{"visibility": "all", "protected": "true", "os_hidden": "TRUE", "min_ram": "0", "description": "x"}}, true},
		{"hidden default", ListOptions{Filters: map[string]string{}}, true},
		{"visibility", ListOptions{Filters: map[string]string{"visibility": "unknown"}}, false},
		{"bool", ListOptions{Filters: map[string]string{"protected": "1"}}, false},
		{"hidden", ListOptions{Filters: map[string]string{"os_hidden": "invalid"}}, false},
		{"status", ListOptions{Filters: map[string]string{"status": "invalid"}}, false},
		{"format", ListOptions{Filters: map[string]string{"disk_format": "invalid"}}, false},
		{"container", ListOptions{Filters: map[string]string{"container_format": "invalid"}}, false},
		{"numeric", ListOptions{Filters: map[string]string{"min_disk": "-1"}}, false},
		{"unsupported", ListOptions{Filters: map[string]string{"deleted": "true"}}, false},
		{"custom property", ListOptions{Filters: map[string]string{"custom' OR 1=1": "literal"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := tc.o.Validate()
			if (err == nil) != tc.valid {
				t.Fatal(err)
			}
			if err == nil {
				again, e := o.Validate()
				if e != nil || !reflect.DeepEqual(o, again) {
					t.Fatal("normalization is not idempotent")
				}
				if o.Filters["os_hidden"] == "" || o.Limit > MaxLimit {
					t.Fatal(o)
				}
			}
		})
	}
}
