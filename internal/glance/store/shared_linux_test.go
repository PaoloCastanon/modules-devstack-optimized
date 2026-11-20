//go:build linux

package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// sharedFixture: a private root plus an "official" objects directory laid
// out as glance_store does (0755 directory, files named by image ID).
func sharedFixture(t *testing.T) (*Filesystem, string) {
	t.Helper()
	base := t.TempDir()
	root, official, scope := filepath.Join(base, "glance-go"), filepath.Join(base, "images"), uuid.NewString()
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(official, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(root, scope); err != nil {
		t.Fatal(err)
	}
	s, err := NewSharedFilesystem(root, scope, official)
	if err != nil {
		t.Fatal("open shared store", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, official
}

func TestSharedStorePublishesUnderTheImageIDLikeGlanceStore(t *testing.T) {
	s, official := sharedFixture(t)
	k, data := key(), []byte("shared bytes")
	info := publish(t, s, k, data)
	got, err := os.ReadFile(filepath.Join(official, k.ImageID))
	if err != nil || string(got) != string(data) {
		t.Fatalf("official path holds %q, %v", got, err)
	}
	f, measured, err := s.Open(context.Background(), k)
	if err != nil || measured != info {
		t.Fatal("open published", err)
	}
	_ = f.Close()
	if err = s.Delete(context.Background(), k); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(official, k.ImageID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("delete must remove the official file", err)
	}
}

func TestSharedStoreServesFilesTheOriginalWrote(t *testing.T) {
	s, official := sharedFixture(t)
	k := Key{ImageID: uuid.NewString(), AttemptID: uuid.NewString()} // any attempt: name is the image ID
	if err := os.WriteFile(filepath.Join(official, k.ImageID), []byte("from python"), 0644); err != nil {
		t.Fatal(err)
	}
	f, size, err := s.OpenStream(context.Background(), k)
	if err != nil || size != 11 {
		t.Fatal("stream original file", err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "from python" {
		t.Fatalf("got %q", b)
	}
	// Writable by others is never served.
	if err = os.Chmod(filepath.Join(official, k.ImageID), 0666); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.OpenStream(context.Background(), k); !errors.Is(err, ErrUnsafe) {
		t.Fatal("world-writable object must be refused", err)
	}
}

func TestSharedStoreRefusesUnsafeOfficialDirectory(t *testing.T) {
	base := t.TempDir()
	root, official, scope := filepath.Join(base, "glance-go"), filepath.Join(base, "images"), uuid.NewString()
	_ = os.Mkdir(root, 0700)
	_ = os.Mkdir(official, 0777)
	_ = os.Chmod(official, 0777)
	if err := Initialize(root, scope); err != nil {
		t.Fatal(err)
	}
	if s, err := NewSharedFilesystem(root, scope, official); !errors.Is(err, ErrUnsafe) {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal("group/other-writable official directory must be refused", err)
	}
	if s, err := NewSharedFilesystem(root, scope, "relative/images"); err == nil {
		_ = s.Close()
		t.Fatal("relative path accepted")
	}
}

func TestSharedStoreWritesDirectlyLikeGlanceStore(t *testing.T) {
	s, official := sharedFixture(t)
	k, data := key(), []byte("direct bytes")
	info, err := s.Stage(context.Background(), k, bytes.NewReader(data), int64(len(data)), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Already under its final name before Publish, as the original writes it.
	if got, err := os.ReadFile(filepath.Join(official, k.ImageID)); err != nil || string(got) != string(data) {
		t.Fatalf("direct write: %q %v", got, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(filepath.Dir(official), "glance-go", "staging")); len(entries) != 0 {
		t.Fatal("no staging copy expected", entries)
	}
	if err = s.Publish(context.Background(), k, info); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Stage(context.Background(), k, bytes.NewReader(data), int64(len(data)), nil); !errors.Is(err, ErrExists) {
		t.Fatal("existing image file must not be overwritten", err)
	}
	if err = os.WriteFile(filepath.Join(official, k.ImageID), []byte("tampered!!!!"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = s.Publish(context.Background(), k, info); !errors.Is(err, ErrIntegrity) {
		t.Fatal("changed bytes must fail publication", err)
	}
}

func TestSharedStoreRemovesPartialDirectWrite(t *testing.T) {
	s, official := sharedFixture(t)
	k := key()
	declared := int64(100)
	if _, err := s.Stage(context.Background(), k, bytes.NewReader([]byte("short")), 1000, &declared); err == nil {
		t.Fatal("short upload accepted")
	}
	if _, err := os.Stat(filepath.Join(official, k.ImageID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial file must be removed, as glance_store does", err)
	}
}
