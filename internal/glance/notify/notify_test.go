package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

// testdata/official-cycle.json: the 9 messages official Glance sent for one
// POST, PATCH, PUT 1 MiB, GET and DELETE, captured from RabbitMQ (IDs replaced).
type capture struct {
	Messages []struct {
		RoutingKey  string         `json:"routing_key"`
		Properties  map[string]any `json:"properties"`
		Envelope    []string       `json:"envelope_keys"`
		OsloVersion string         `json:"oslo.version"`
		Message     map[string]any `json:"message"`
		ContextKeys []string       `json:"context_keys"`
	} `json:"messages"`
}

func official(t *testing.T) capture {
	t.Helper()
	b, err := os.ReadFile("testdata/official-cycle.json")
	if err != nil {
		t.Fatal(err)
	}
	var c capture
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func roundTrip(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// imageFrom rebuilds the Go image an official payload describes.
func imageFrom(t *testing.T, p map[string]any) model.Image {
	t.Helper()
	at := func(k string) time.Time {
		v, err := time.Parse("2006-01-02T15:04:05Z", p[k].(string))
		if err != nil {
			t.Fatal(k, err)
		}
		return v
	}
	str := func(k string) *string {
		if s, ok := p[k].(string); ok {
			return &s
		}
		return nil
	}
	num := func(k string) *int64 {
		if f, ok := p[k].(float64); ok {
			n := int64(f)
			return &n
		}
		return nil
	}
	i := model.Image{ID: p["id"].(string), Owner: p["owner"].(string), Status: p["status"].(string), Visibility: p["visibility"].(string),
		Name: str("name"), DiskFormat: str("disk_format"), ContainerFormat: str("container_format"),
		MinDisk: int64(p["min_disk"].(float64)), MinRAM: int64(p["min_ram"].(float64)), Protected: p["protected"].(bool),
		Size: num("size"), VirtualSize: num("virtual_size"), Checksum: str("checksum"),
		CreatedAt: at("created_at"), UpdatedAt: at("updated_at"), Properties: map[string]string{}}
	for k, v := range p["properties"].(map[string]any) {
		i.Properties[k] = v.(string)
	}
	for _, v := range p["tags"].([]any) {
		i.Tags = append(i.Tags, v.(string))
	}
	return i
}

func TestPayloadsMatchOfficialGlanceExactly(t *testing.T) {
	seen := map[string]bool{}
	for n, m := range official(t).Messages {
		event, want := m.Message["event_type"].(string), m.Message["payload"].(map[string]any)
		var got map[string]any
		switch event {
		case "image.create", "image.update", "image.activate":
			got = roundTrip(t, ImagePayload(imageFrom(t, want)))
		case "image.prepare", "image.upload":
			got = roundTrip(t, UploadPayload(imageFrom(t, want)))
		case "image.delete":
			before := imageFrom(t, want)
			before.Status = "queued" // DeletePayload must set it, not the caller
			at, _ := time.Parse("2006-01-02T15:04:05Z", want["deleted_at"].(string))
			got = roundTrip(t, DeletePayload(before, at))
		case "image.send":
			got = roundTrip(t, SendPayload(want["image_id"].(string), want["owner_id"].(string),
				want["receiver_tenant_id"].(string), want["receiver_user_id"].(string), int64(want["bytes_sent"].(float64))))
		default:
			t.Fatalf("message %d: unexpected official event %s", n, event)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("message %d %s differs from official Glance:\n got %v\nwant %v", n, event, got, want)
		}
		seen[event] = true
	}
	if len(seen) != 7 {
		t.Fatal("official capture must cover create/update/prepare/upload/activate/send/delete", seen)
	}
}

func TestEnvelopeIsOsloMessagingV2(t *testing.T) {
	c := official(t)
	ref := c.Messages[0]
	body, err := Envelope("image.localhost", "image.create", Info, map[string]any{"id": "x"}, time.Date(2026, 11, 19, 18, 7, 47, 47579000, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]string
	if err = json.Unmarshal(body, &outer); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for k := range outer {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, ref.Envelope) || outer["oslo.version"] != ref.OsloVersion {
		t.Fatal("envelope", keys, outer["oslo.version"])
	}
	var inner map[string]any
	if err = json.Unmarshal([]byte(outer["oslo.message"]), &inner); err != nil {
		t.Fatal(err)
	}
	gotKeys, wantKeys := []string{}, []string{}
	for k := range inner {
		gotKeys = append(gotKeys, k)
	}
	for k := range ref.Message {
		wantKeys = append(wantKeys, k)
	}
	sort.Strings(gotKeys)
	sort.Strings(wantKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) || len(ref.ContextKeys) != 0 {
		t.Fatal("message keys", gotKeys, wantKeys)
	}
	if inner["timestamp"] != "2026-11-19 18:07:47.047579" || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(inner["_unique_id"].(string)) ||
		!regexp.MustCompile(`^[0-9a-f-]{36}$`).MatchString(inner["message_id"].(string)) || inner["priority"] != "INFO" {
		t.Fatal("message fields", inner)
	}
	if ref.Message["timestamp"] == nil || !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{6}$`).MatchString(ref.Message["timestamp"].(string)) {
		t.Fatal("reference timestamp format changed")
	}
	for _, m := range c.Messages {
		if m.RoutingKey != "notifications.info" || m.Properties["delivery_mode"] != float64(2) || m.Properties["content_type"] != "application/json" {
			t.Fatal("official delivery properties changed", m.RoutingKey, m.Properties)
		}
	}
}

type record struct {
	mu     sync.Mutex
	events []string
}

func (r *record) Notify(event, priority string, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event+"/"+priority)
}

func TestDisabledNotificationsByTypeOrGroup(t *testing.T) {
	r := &record{}
	f := Filtered{Next: r, Disabled: map[string]bool{"image.send": true, "task": true}}
	for _, e := range []string{"image.send", "image.upload", "task.success", "image.create"} {
		f.Notify(e, Info, nil)
	}
	if !reflect.DeepEqual(r.events, []string{"image.upload/INFO", "image.create/INFO"}) {
		t.Fatal(r.events)
	}
	f = Filtered{Next: r, Disabled: map[string]bool{"image": true}}
	f.Notify("image.create", Info, nil)
	if len(r.events) != 2 {
		t.Fatal("group 'image' must disable all image events")
	}
	OrNoop(nil).Notify("image.create", Info, nil)
}

type fakeTransport struct {
	mu       sync.Mutex
	fail     int
	sent     []string
	closed   bool
	block    chan struct{}
	attempts int
}

func (f *fakeTransport) Publish(ctx context.Context, topic, priority string, body []byte) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.fail > 0 {
		f.fail--
		return errors.New("dial tcp 10.0.0.1:5672: connect: connection refused")
	}
	var outer map[string]string
	_ = json.Unmarshal(body, &outer)
	var inner map[string]any
	_ = json.Unmarshal([]byte(outer["oslo.message"]), &inner)
	f.sent = append(f.sent, topic+"."+strings.ToLower(priority)+" "+inner["event_type"].(string))
	return nil
}

func (f *fakeTransport) Close() error { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }

func (f *fakeTransport) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.sent...)
}

func TestMessagingDeliversInOrderAndRetriesTheBroker(t *testing.T) {
	var logs strings.Builder
	tr := &fakeTransport{fail: 3}
	m := NewMessaging(tr, MessagingOptions{Retry: time.Millisecond, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	for _, e := range []string{"image.create", "image.update", "image.prepare", "image.upload", "image.activate"} {
		m.Notify(e, Info, map[string]any{})
	}
	m.Notify("image.send", Error, map[string]any{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"notifications.info image.create", "notifications.info image.update", "notifications.info image.prepare",
		"notifications.info image.upload", "notifications.info image.activate", "notifications.error image.send"}
	if got := tr.snapshot(); !reflect.DeepEqual(got, want) || !tr.closed {
		t.Fatal("order or delivery lost across broker retries", got)
	}
	if strings.Contains(logs.String(), "10.0.0.1") || !strings.Contains(logs.String(), "connection_refused") {
		t.Fatal("broker address leaked or outage not reported", logs.String())
	}
	m.Notify("image.delete", Info, nil) // after Close: ignored, never panics
}

func TestMessagingNeverBlocksAndBoundsItsQueue(t *testing.T) {
	tr := &fakeTransport{block: make(chan struct{})}
	m := NewMessaging(tr, MessagingOptions{QueueSize: 4})
	started := time.Now()
	for n := 0; n < 50; n++ {
		m.Notify("image.update", Info, map[string]any{"n": n})
	}
	if time.Since(started) > time.Second {
		t.Fatal("Notify blocked on a stalled broker")
	}
	if d := m.Dropped(); d < 45 || d > 46 { // 4 queued + at most 1 in flight
		t.Fatal("bounded queue did not drop the overflow", d)
	}
	close(tr.block)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(tr.snapshot()); n < 4 || n > 5 {
		t.Fatal("queued notifications not delivered on close", n)
	}
}

func TestMessagingCloseHonoursItsDeadline(t *testing.T) {
	tr := &fakeTransport{fail: 1 << 30}
	m := NewMessaging(tr, MessagingOptions{Retry: time.Millisecond})
	m.Notify("image.create", Info, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := m.Close(ctx); err == nil || time.Since(started) > 2*time.Second || !tr.closed {
		t.Fatal("shutdown must not wait forever for an absent broker", err)
	}
}

func TestTransportURLIsValidatedWithoutEchoingSecrets(t *testing.T) {
	uri, err := ParseTransportURL("rabbit://stackrabbit:s3cret@192.168.122.10:5672/\n")
	if err != nil || uri != "amqp://stackrabbit:s3cret@192.168.122.10:5672/" {
		t.Fatal(uri, err)
	}
	if _, err = ParseTransportURL("amqps://u:p@broker:5671/glance"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "http://u:s3cret@h/", "rabbit://u:s3cret@h1:5672,u:s3cret@h2:5672/", "rabbit:///"} {
		if _, err = ParseTransportURL(bad); err == nil || strings.Contains(err.Error(), "s3cret") {
			t.Fatal("accepted or echoed", bad, err)
		}
	}
}
