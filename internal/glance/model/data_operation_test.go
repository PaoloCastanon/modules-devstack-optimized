package model

import (
	"strings"
	"testing"
)

func TestDataReceiptValidation(t *testing.T) {
	c := DataContent{Size: 20, Checksum: strings.Repeat("a", 32), SHA512: strings.Repeat("b", 128)}
	for _, tc := range []struct {
		name   string
		change func(*DataContent)
		valid  bool
	}{
		{"valid", func(*DataContent) {}, true},
		{"empty-file", func(c *DataContent) { c.Size = 0 }, true},
		{"unknown-virtual-size", func(c *DataContent) { c.VirtualSize = nil }, true},
		{"negative", func(c *DataContent) { c.Size = -1 }, false},
		{"negative-virtual", func(c *DataContent) { n := int64(-1); c.VirtualSize = &n }, false},
		{"short-md5", func(c *DataContent) { c.Checksum = "a" }, false},
		{"bad-md5", func(c *DataContent) { c.Checksum = strings.Repeat("z", 32) }, false},
		{"uppercase-md5", func(c *DataContent) { c.Checksum = strings.Repeat("A", 32) }, false},
		{"bad-sha512", func(c *DataContent) { c.SHA512 = strings.Repeat("g", 128) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := c
			tc.change(&v)
			if (v.Validate(nil) == nil) != tc.valid {
				t.Fatal(v)
			}
		})
	}
	expected := int64(21)
	if c.Validate(&expected) == nil {
		t.Fatal("mismatch accepted")
	}
	expected = -1
	if c.Validate(&expected) == nil {
		t.Fatal("negative expected accepted")
	}
}
func TestDataOperationInvariants(t *testing.T) {
	n := int64(20)
	md5, sha := strings.Repeat("a", 32), strings.Repeat("b", 128)
	o := DataOperation{ImageID: "00000000-0000-4000-8000-000000000001", AttemptID: "00000000-0000-4000-8000-000000000002", Phase: DataStaged, Size: &n, Checksum: &md5, SHA512: &sha}
	for _, phase := range []DataPhase{DataStaged, DataPublished, DataCommitted} {
		v := o
		v.Phase = phase
		if v.Validate() != nil {
			t.Fatal(phase)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*DataOperation)
	}{
		{"bad-image", func(o *DataOperation) { o.ImageID = "bad" }},
		{"bad-attempt", func(o *DataOperation) { o.AttemptID = "bad" }},
		{"bad-phase", func(o *DataOperation) { o.Phase = "bad" }},
		{"missing-size", func(o *DataOperation) { o.Size = nil }},
		{"missing-md5", func(o *DataOperation) { o.Checksum = nil }},
		{"missing-sha512", func(o *DataOperation) { o.SHA512 = nil }},
		{"cleanup-needs-target", func(o *DataOperation) { o.Phase = DataCleanup }},
		{"cleaned-needs-target", func(o *DataOperation) { o.Phase = DataCleaned }},
		{"bad-target", func(o *DataOperation) { s := "active"; o.CleanupTarget = &s }},
		{"private-error-text", func(o *DataOperation) { s := "password sql:123"; o.LastErrorCode = &s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := o
			tc.change(&v)
			if v.Validate() == nil {
				t.Fatal("invalid operation accepted")
			}
		})
	}
}
