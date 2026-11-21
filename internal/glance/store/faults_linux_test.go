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
