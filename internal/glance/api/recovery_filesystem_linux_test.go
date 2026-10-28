//go:build linux

package api

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
	"github.com/google/uuid"
)

type failedSource struct{}

func (failedSource) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Reopen COMPONENTS, not a process restart or physical crash. SQL is a double;
// file writes/hashes/unlink and exclusive reopening use the real temporary FS.
func TestStartupRecoveryRealFilesystemBoundaries(t *testing.T) {
	for _, boundary := range []string{"part", "ready-before-SQL", "staged", "published-before-SQL", "published", "committed", "missing committed", "corrupt committed", "tombstone"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := context.Background()
			root, scope := t.TempDir(), uuid.NewString()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := store.Initialize(root, scope); err != nil {
				t.Fatal(err)
			}
			fs, err := store.NewFilesystem(root, scope)
			if err != nil {
				t.Fatal(err)
			}
			raw, bare := "raw", "bare"
			i, err := model.New(model.CreateInput{DiskFormat: &raw, ContainerFormat: &bare}, "p", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			d := &operationalSQL{&deletionPipelineSQL{pipelineSQL: &pipelineSQL{repoFake: &repoFake{image: i}}}}
			attempt := uuid.NewString()
			key := store.Key{ImageID: i.ID, AttemptID: attempt}
			if _, err = d.ReserveData(ctx, i.ID, attempt, nil, time.Now()); err != nil {
				t.Fatal(err)
			}
			if boundary == "part" {
				if _, err = fs.Stage(ctx, key, io.MultiReader(strings.NewReader("partial"), failedSource{}), 1024, nil); err == nil {
					t.Fatal("source did not fail")
				}
			} else {
				info, err := fs.Stage(ctx, key, strings.NewReader("abcdef"), 1024, nil)
				if err != nil {
					t.Fatal(err)
				}
				c := model.DataContent{Size: info.Size, VirtualSize: &info.Size, Checksum: info.Checksum, SHA512: info.SHA512}
				if boundary != "ready-before-SQL" {
					if err = d.StageData(ctx, i.ID, attempt, c, time.Now()); err != nil {
						t.Fatal(err)
					}
				}
				if boundary != "staged" && boundary != "ready-before-SQL" {
					if err = fs.Publish(ctx, key, info); err != nil {
						t.Fatal(err)
					}
					if boundary != "published-before-SQL" {
						if err = d.PublishData(ctx, i.ID, attempt, time.Now()); err != nil {
							t.Fatal(err)
						}
					}
					if strings.Contains(boundary, "committed") || boundary == "tombstone" {
						if err = d.CommitData(ctx, i.ID, attempt, time.Now()); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if boundary == "tombstone" {
				if _, err = d.TombstoneData(ctx, i.ID, &attempt, time.Now(), func(model.Image) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err = fs.Close(); err != nil {
				t.Fatal(err)
			}
			blob := root + "/objects/" + i.ID + "." + attempt + ".blob"
			// Use the actual name convention only for deliberate external corruption.
			entries, err := os.ReadDir(root + "/objects")
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) > 0 {
				blob = root + "/objects/" + entries[0].Name()
			}
			if boundary == "missing committed" {
				if err = os.Remove(blob); err != nil {
					t.Fatal(err)
				}
			}
			if boundary == "corrupt committed" {
				if err = os.WriteFile(blob, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			fs, err = store.NewFilesystem(root, scope)
			if err != nil {
				t.Fatal(err)
			}
			u, err := service.NewUploadCoordinator(d, d, fs, service.UploadLimits{MaxBytes: 1024, MaxConcurrent: 2, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			report, err := u.RecoverUploads(ctx)
			if strings.HasPrefix(boundary, "missing") || strings.HasPrefix(boundary, "corrupt") {
				if !errors.Is(err, service.ErrUploadReconciliation) || d.image.Status != "active" || d.op.Phase != model.DataCommitted {
					t.Fatal("unsafe repair", err)
				}
			} else if boundary == "committed" {
				if err != nil || report.Kept != 1 || d.image.Status != "active" {
					t.Fatal(err, report)
				}
			} else {
				status := "queued"
				if boundary == "tombstone" {
					status = "deleted"
				}
				if err != nil || report.Cleaned != 1 || d.image.Status != status || d.op.Phase != model.DataCleaned {
					t.Fatal("recovery", err, report, d.image.Status)
				}
				for _, dir := range []string{"objects", "staging"} {
					entries, err := os.ReadDir(root + "/" + dir)
					if err != nil || len(entries) != 0 {
						t.Fatal("incomplete cleanup", dir, err)
					}
				}
			}
			if err = u.Close(); err != nil {
				t.Fatal(err)
			}
			if err = fs.Close(); err != nil {
				t.Fatal(err)
			}
			// Repeated recovery uses a NEW exclusive owner and is idempotent.
			if boundary == "tombstone" || boundary == "staged" {
				fs, err = store.NewFilesystem(root, scope)
				if err != nil {
					t.Fatal(err)
				}
				u, err = service.NewUploadCoordinator(d, d, fs, service.UploadLimits{MaxBytes: 1024, MaxConcurrent: 2, ResolutionTimeout: time.Second, RecoveryTimeout: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				report, err = u.RecoverUploads(ctx)
				if err != nil || report.Cleaned != 0 || report.Kept != 1 {
					t.Fatal(err, report)
				}
				if err = u.Close(); err != nil {
					t.Fatal(err)
				}
				if err = fs.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
