package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func ptr[T any](value T) *T { return &value }

func TestDefaultsAndUUID(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.FixedZone("local", -10800))
	image, err := New(CreateInput{}, "project", now)
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuid.Parse(image.ID)
	if err != nil || id.Version() != 4 {
		t.Fatalf("generated ID %q: %v", image.ID, err)
	}
	if image.Status != "queued" || image.Visibility != "shared" || image.Owner != "project" || image.MinDisk != 0 || image.MinRAM != 0 || image.Protected || image.Hidden {
		t.Fatalf("wrong defaults: %#v", image)
	}
	if image.Name != nil || image.DiskFormat != nil || image.ContainerFormat != nil || image.Size != nil || image.VirtualSize != nil || image.Checksum != nil || image.HashAlgorithm != nil || image.HashValue != nil {
		t.Fatal("unavailable metadata must be null")
	}
	if image.Tags == nil || len(image.Tags) != 0 || image.Properties == nil || image.CreatedAt.Location() != time.UTC || !image.UpdatedAt.Equal(image.CreatedAt) {
		t.Fatal("wrong collections or timestamps")
	}
}

func TestProvidedValues(t *testing.T) {
	image, err := New(CreateInput{ID: ptr("B2173DD3-7AD6-4362-BAA6-A68BCE3565CB"), Name: ptr(""), DiskFormat: ptr("raw"), ContainerFormat: ptr("bare"), Visibility: ptr("private"), Protected: ptr(true), Hidden: ptr(true), MinDisk: ptr(int64(2)), Tags: []string{"x", "x", "X"}, Properties: map[string]string{"description": "", "hw_rng_model": "virtio"}}, "project", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if image.ID != "b2173dd3-7ad6-4362-baa6-a68bce3565cb" || image.Name == nil || *image.Name != "" || len(image.Tags) != 2 || image.Properties["description"] != "" || !image.Protected || !image.Hidden || image.MinDisk != 2 {
		t.Fatalf("values lost: %#v", image)
	}
}

func TestDomainValidation(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		kind  Kind
	}{
		{"compact UUID", CreateInput{ID: ptr("b2173dd37ad64362baa6a68bce3565cb")}, Invalid},
		{"empty UUID", CreateInput{ID: ptr("")}, Invalid},
		{"long name", CreateInput{Name: ptr(strings.Repeat("界", 256))}, Invalid},
		{"disk format", CreateInput{DiskFormat: ptr("")}, Invalid},
		{"container format", CreateInput{ContainerFormat: ptr("no")}, Invalid},
		{"visibility", CreateInput{Visibility: ptr("")}, Invalid},
		{"negative minimum", CreateInput{MinRAM: ptr(int64(-1))}, Invalid},
		{"overflow minimum", CreateInput{MinDisk: ptr(int64(1 << 32))}, Invalid},
		{"foreign owner", CreateInput{Owner: ptr("other")}, Forbidden},
		{"reserved property", CreateInput{Properties: map[string]string{"os_glance_task": "x"}}, Forbidden},
		{"base collision", CreateInput{Properties: map[string]string{"status": "active"}}, Forbidden},
		{"long property", CreateInput{Properties: map[string]string{strings.Repeat("x", 256): "v"}}, Invalid},
		{"long value", CreateInput{Properties: map[string]string{"x": strings.Repeat("x", 65536)}}, Invalid},
		{"long tag", CreateInput{Tags: []string{strings.Repeat("x", 256)}}, Invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.input, "project", time.Now())
			var domain *Error
			if !errors.As(err, &domain) || domain.Kind != tc.kind {
				t.Fatalf("got %v, expected kind %v", err, tc.kind)
			}
		})
	}
	properties := map[string]string{}
	for i := 0; i < 129; i++ {
		properties[strings.Repeat("x", i)] = "v"
	}
	_, err := New(CreateInput{Properties: properties}, "project", time.Now())
	var domain *Error
	if !errors.As(err, &domain) || domain.Kind != LimitExceeded {
		t.Fatalf("quota: %v", err)
	}
}
