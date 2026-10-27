package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the real Make target only against temporary copies outside the
// repository. A formatter failure must not look like empty (formatted) output.
func TestFormatGateExitStatus(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Fatal("make is required to validate the format gate")
	}
	formatter, err := exec.LookPath("gofmt")
	if err != nil {
		t.Fatal("gofmt is required")
	}
	makefile, err := filepath.Abs("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, formatter string
		success                 bool
	}{
		{"formatted", "package main\n\nfunc fixture() {}\n", formatter, true},
		{"unformatted", "package main\nfunc fixture( ) { }\n", formatter, false},
		{"syntax error", "package main\nfunc {\n", formatter, false},
		{"command unavailable", "package main\n", "/nonexistent-glance-gofmt", false},
		{"formatter nonzero with no output", "package main\n", "fake", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, path := range []string{"cmd/glance", "internal"} {
				if err := os.MkdirAll(filepath.Join(dir, path), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "cmd/glance/fixture.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			command := tc.formatter
			if command == "fake" {
				command = filepath.Join(dir, "formatter")
				if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(makePath, "--no-print-directory", "-f", makefile, "glance-fmt-check", "GLANCE_GOFMT="+command)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "MAKEFLAGS=")
			output, err := cmd.CombinedOutput()
			if (err == nil) != tc.success {
				t.Fatalf("success=%v want=%v; output=%s", err == nil, tc.success, output)
			}
		})
	}
}
