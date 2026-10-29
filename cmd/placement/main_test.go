package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestCommandRejectsInvalidModeAndMissingSecrets(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, args := range [][]string{nil, {"invalid"}, {"serve", "extra"}} {
		if err := run(args, logger); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatal(args, err)
		}
	}
	t.Setenv("PLACEMENT_GO_DATABASE_DSN_FILE", "/nonexistent-placement-secret")
	if err := run([]string{"serve"}, logger); err == nil || strings.Contains(err.Error(), "nonexistent") {
		t.Fatal("missing secret accepted or leaked", err)
	}
}
