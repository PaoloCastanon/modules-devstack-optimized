package service

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/notify"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/store"
)

// The event sequences below are the ones official Glance emitted for the same
// operations (internal/glance/notify/testdata/official-cycle.json).
type sink struct {
	mu       sync.Mutex
	events   []string
	payloads []any
}

func (s *sink) Notify(event, priority string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event+"/"+priority)
	s.payloads = append(s.payloads, payload)
}

func (s *sink) snapshot() ([]string, []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.events...), append([]any{}, s.payloads...)
}

func TestCatalogEmitsCreateUpdateDeleteAfterSuccessOnly(t *testing.T) {
	ctx := context.Background()
	repo := &repositoryStub{}
	s := New(repo, time.Now)
	n := &sink{}
	s.SetNotifier(n)
	if _, err := s.Create(ctx, identity("reader"), model.CreateInput{}); err == nil {
		t.Fatal("reader created")
	}
	created, err := s.Create(ctx, identity("member"), model.CreateInput{Properties: map[string]string{"k1": "v1"}, Tags: []string{"t"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Patch(ctx, identity("member"), created.ID, []model.PatchOperation{{Op: "replace", Key: "name", Value: []byte(`"n2"`)}}); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, identity("member"), created.ID); err != nil {
		t.Fatal(err)
	}
	events, payloads := n.snapshot()
	if !reflect.DeepEqual(events, []string{"image.create/INFO", "image.update/INFO", "image.delete/INFO"}) {
		t.Fatal(events)
	}
	if p := payloads[1].(map[string]any); *(p["name"].(*string)) != "n2" || p["status"] != "queued" {
		t.Fatal("update must carry the changed image", p)
	}
	if p := payloads[2].(map[string]any); p["status"] != "deleted" || p["deleted"] != true || p["deleted_at"] == nil || p["id"] != created.ID {
		t.Fatal("delete payload", p)
	}
	if p := payloads[0].(map[string]any); p["properties"].(map[string]string)["k1"] != "v1" {
		t.Fatal("create payload", p)
	}
	repo.err = io.ErrUnexpectedEOF
	_, _ = s.Patch(ctx, identity("member"), created.ID, nil)
	_ = s.Delete(ctx, identity("member"), created.ID)
	if events, _ = n.snapshot(); len(events) != 3 {
		t.Fatal("failed changes must not notify", events)
	}
}

func TestUploadEmitsTheOfficialSequence(t *testing.T) {
	u, d, _, _ := uploadFixture(t)
	n := &sink{}
	u.SetNotifier(n)
	size := int64(len(uploadPayload))
	if _, err := u.Upload(context.Background(), identity("member"), uploadImageID, bytes.NewReader(uploadPayload), &size); err != nil {
		t.Fatal(err)
	}
	events, payloads := n.snapshot()
	want := []string{"image.update/INFO", "image.prepare/INFO", "image.upload/INFO", "image.activate/INFO", "image.update/INFO"}
	if !reflect.DeepEqual(events, want) {
		t.Fatal(events)
	}
	status := func(i int) any { return payloads[i].(map[string]any)["status"] }
	if status(0) != "saving" || status(1) != "saving" || status(2) != "active" || status(3) != "active" || status(4) != "active" {
		t.Fatal("statuses differ from official", status(0), status(1), status(2), status(3), status(4))
	}
	up := payloads[2].(map[string]any)
	if *(up["size"].(*int64)) != size || *(up["checksum"].(*string)) != uploadInfo(uploadPayload).Checksum {
		t.Fatal("upload payload lacks the stored content", up)
	}
	if _, ok := payloads[1].(map[string]any)["os_glance_importing_to_stores"]; !ok {
		t.Fatal("prepare must carry the import properties")
	}
	if _, ok := payloads[3].(map[string]any)["os_glance_importing_to_stores"]; ok {
		t.Fatal("activate is a plain image payload")
	}
	if d.image.Status != "active" {
		t.Fatal("upload not committed")
	}
}

func TestFailedUploadSendsAnErrorMessageAfterPrepare(t *testing.T) {
	u, _, fs, _ := uploadFixture(t)
	n := &sink{}
	u.SetNotifier(n)
	fs.faults["Stage"] = uploadStoreFault{err: store.ErrStoreFull}
	size := int64(len(uploadPayload))
	if _, err := u.Upload(context.Background(), identity("member"), uploadImageID, bytes.NewReader(uploadPayload), &size); err == nil {
		t.Fatal("store full accepted")
	}
	events, payloads := n.snapshot()
	if !reflect.DeepEqual(events, []string{"image.update/INFO", "image.prepare/INFO", "image.upload/ERROR"}) {
		t.Fatal(events)
	}
	if msg := payloads[2].(string); !strings.HasPrefix(msg, "Image storage media is full:") {
		t.Fatal("official message text", msg)
	}
	// Rejected before reservation (no prepare): nothing, as official Glance.
	u2, _, _, _ := uploadFixture(t)
	n2 := &sink{}
	u2.SetNotifier(n2)
	if _, err := u2.Upload(context.Background(), identity("reader"), uploadImageID, bytes.NewReader(uploadPayload), &size); err == nil {
		t.Fatal("reader uploaded")
	}
	if events, _ = n2.snapshot(); len(events) != 0 {
		t.Fatal("pre-reservation rejection notified", events)
	}
	for _, tc := range []struct {
		err  error
		text string
	}{{store.ErrSizeMismatch, "Cannot save data"}, {ErrUploadGone, "could not be found after upload"},
		{model.Fail(model.Conflict, "x"), "Unable to upload duplicate"}, {io.ErrUnexpectedEOF, "due to internal error"}} {
		if msg := uploadFailureMessage(uploadImageID, tc.err); !strings.Contains(msg, tc.text) || strings.Contains(msg, "EOF") {
			t.Fatal(tc.err, msg)
		}
	}
}

func TestDownloadReportsSendOnlyForCompleteDeliveries(t *testing.T) {
	u, d, fs, _ := uploadFixture(t)
	uploadSeed(d, model.DataCommitted)
	fs.blob = append([]byte{}, uploadPayload...)
	n := &sink{}
	u.SetNotifier(n)
	who := identity("reader")
	who.UserID = "receiver-user"
	out, err := u.OpenDownload(context.Background(), who, uploadImageID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(out.Body)
	_ = out.Body.Close()
	out.Sent(out.Size, out.Size)
	out.Sent(3, out.Size)
	events, payloads := n.snapshot()
	if !reflect.DeepEqual(events, []string{"image.send/INFO", "image.send/ERROR"}) {
		t.Fatal(events)
	}
	p := payloads[0].(map[string]any)
	if p["bytes_sent"] != out.Size || p["image_id"] != uploadImageID || p["owner_id"] != d.image.Owner ||
		p["receiver_tenant_id"] != who.ProjectID || p["receiver_user_id"] != "receiver-user" {
		t.Fatal("send payload", p)
	}
}

func TestDeleteWithDataEmitsDeleteOnceTheRowIsGone(t *testing.T) {
	u, d, fs, _ := deletionFixture(t)
	uploadSeed(d, model.DataCommitted)
	fs.blob = append([]byte{}, uploadPayload...)
	n := &sink{}
	u.SetNotifier(n)
	if err := u.DeleteImage(context.Background(), identity("member"), uploadImageID); err != nil {
		t.Fatal(err)
	}
	events, payloads := n.snapshot()
	if !reflect.DeepEqual(events, []string{"image.delete/INFO"}) {
		t.Fatal(events)
	}
	if p := payloads[0].(map[string]any); p["status"] != "deleted" || p["deleted"] != true || p["size"] == nil {
		t.Fatal("delete payload keeps the deleted image's data fields", p)
	}
	var _ notify.Notifier = n
}
