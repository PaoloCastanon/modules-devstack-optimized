//go:build linux

package store

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
)

// Only these tests inject failures into actual temporary filesystem operations.
// ENOSPC here is SIMULATED, not a full-disk integration or power-loss test.
func TestInjectedWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"write EIO", syscall.EIO, syscall.EIO},
		{"disk full", syscall.ENOSPC, ErrStoreFull},
		{"quota full", syscall.EDQUOT, ErrStoreFull},
		{"short write", nil, io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := fixture(t)
			k := key()
			s.ops.write = func(f *os.File, b []byte) (int, error) {
				n, err := f.Write(b[:1])
				return n, errors.Join(err, tc.err)
			}
			if _, err := s.Stage(context.Background(), k, strings.NewReader("partial"), 100, nil); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if err := s.Publish(context.Background(), k, digest([]byte("partial"))); !errors.Is(err, ErrIncomplete) {
				t.Fatal(err)
			}
			if err := s.Delete(context.Background(), k); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInjectedStageAndPublicationBoundaries(t *testing.T) {
	for _, point := range []string{"stage-file-sync", "stage-link", "stage-dir-sync", "stage-unlink", "stage-cleanup-sync", "publish-file-sync", "publish-link", "publish-dir-sync", "publish-unlink", "publish-cleanup-sync"} {
		t.Run(point, func(t *testing.T) {
			s, _, _ := fixture(t)
			k := key()
			i := digest([]byte("complete"))
			publishing := strings.HasPrefix(point, "publish-")
			if publishing {
				if _, err := s.Stage(context.Background(), k, strings.NewReader("complete"), 20, nil); err != nil {
					t.Fatal(err)
				}
			}
			real := realOps()
			fired := false
			dirSyncs := 0
			s.ops.sync = func(f *os.File) error {
				if f == s.staging {
					dirSyncs++
				}
				fail := (point == "stage-file-sync" || point == "publish-file-sync") && f != s.staging && f != s.objects
				fail = fail || point == "stage-dir-sync" && f == s.staging && dirSyncs == 1
				fail = fail || point == "stage-cleanup-sync" && f == s.staging && dirSyncs == 2
				fail = fail || point == "publish-dir-sync" && f == s.objects
				fail = fail || point == "publish-cleanup-sync" && f == s.staging
				if fail {
					fired = true
					return syscall.EIO
				}
				return real.sync(f)
			}
			s.ops.link = func(f, d *os.File, name string) error {
				if point == "stage-link" || point == "publish-link" {
					fired = true
					return syscall.EIO
				}
				return real.link(f, d, name)
			}
			s.ops.unlink = func(d *os.File, name string) error {
				if point == "stage-unlink" || point == "publish-unlink" {
					fired = true
					return syscall.EIO
				}
				return real.unlink(d, name)
			}
			var err error
			if publishing {
				err = s.Publish(context.Background(), k, i)
			} else {
				_, err = s.Stage(context.Background(), k, strings.NewReader("complete"), 20, nil)
			}
			if !fired || !errors.Is(err, syscall.EIO) {
				t.Fatal("fault not exercised", fired, err)
			}
			s.ops = real
			f, got, openErr := s.Open(context.Background(), k)
			if point == "publish-dir-sync" || point == "publish-unlink" || point == "publish-cleanup-sync" {
				if openErr != nil || got != i {
					t.Fatal("uncertain outcome lost complete file", got, openErr)
				}
				f.Close()
				if err = s.Publish(context.Background(), k, i); !errors.Is(err, ErrExists) {
					t.Fatal("uncertain publish overwrote object", err)
				}
			} else if !errors.Is(openErr, ErrNotFound) {
				t.Fatal("failure published bytes", openErr)
			}
			// Recovery policy is NOT implemented: test explicitly authorizes Delete
			// only for its own fixture, then verifies repeated cleanup is harmless.
			if err = s.Delete(context.Background(), k); err != nil {
				t.Fatal(err)
			}
			if err = s.Delete(context.Background(), k); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInjectedDeleteFailureAndDirectorySyncRetry(t *testing.T) {
	for _, point := range []string{"unlink", "objects-sync", "staging-sync"} {
		t.Run(point, func(t *testing.T) {
			s, _, _ := fixture(t)
			k := key()
			publish(t, s, k, []byte("content"))
			real := realOps()
			s.ops.unlink = func(d *os.File, name string) error {
				if point == "unlink" && d == s.objects {
					return syscall.EIO
				}
				return real.unlink(d, name)
			}
			s.ops.sync = func(f *os.File) error {
				if point == "objects-sync" && f == s.objects || point == "staging-sync" && f == s.staging {
					return syscall.EIO
				}
				return real.sync(f)
			}
			if err := s.Delete(context.Background(), k); !errors.Is(err, syscall.EIO) {
				t.Fatal(err)
			}
			s.ops = real
			var synced int
			s.ops.sync = func(f *os.File) error { synced++; return real.sync(f) }
			if err := s.Delete(context.Background(), k); err != nil || synced != 2 {
				t.Fatal("retry did not sync both directories", synced, err)
			}
		})
	}
}
