package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
)

const remoteID = "5a6c8c55-3b0c-4f1e-9e0a-0f1d2c3b4a59"

// remoteGlance is another region's Glance and the Keystone that lists it.
type remoteGlance struct {
	glance, keystone *httptest.Server
	status           string
	size             int64
	payload          []byte
	tokens           []string
}

func newRemoteGlance(t *testing.T) *remoteGlance {
	r := &remoteGlance{status: "active", payload: bytes.Repeat([]byte("remote-image "), 1000)}
	r.size = int64(len(r.payload))
	r.glance = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.tokens = append(r.tokens, q.Header.Get("X-Auth-Token"))
		switch q.URL.Path {
		case "/image/v2/images/" + remoteID:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": remoteID, "status": r.status, "size": r.size,
				"disk_format": "qcow2", "container_format": "bare", "hw_disk_bus": "scsi", "os_distro": "cirros",
				"trait:CUSTOM_X": "required", "owner_specific": "not copied", "os_glance_import_task": "reserved", "min_ram": 0})
		case "/image/v2/images/" + remoteID + "/file":
			_, _ = w.Write(r.payload)
		default:
			w.WriteHeader(404)
		}
	}))
	r.keystone = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.tokens = append(r.tokens, q.Header.Get("X-Auth-Token"))
		if q.URL.Path != "/v3/auth/catalog" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"catalog": []any{
			map[string]any{"type": "compute", "endpoints": []any{map[string]any{"interface": "public", "region": "RegionTwo", "url": "http://nova.invalid"}}},
			map[string]any{"type": "image", "endpoints": []any{
				map[string]any{"interface": "public", "region": "RegionOne", "url": "http://local.invalid/image"},
				map[string]any{"interface": "internal", "region": "RegionTwo", "url": "http://internal.invalid/image"},
				map[string]any{"interface": "public", "region": "RegionTwo", "url": r.glance.URL + "/image/"},
			}},
		}})
	}))
	t.Cleanup(func() { r.glance.Close(); r.keystone.Close() })
	return r
}

func downloadFixture(t *testing.T, r *remoteGlance) (*Importer, *memberStub) {
	t.Helper()
	s, repo := sharedFixture("private", "importing")
	raw, bare := "raw", "ovf"
	repo.image.DiskFormat, repo.image.ContainerFormat = &raw, &bare
	repo.image.Properties = map[string]string{"os_distro": "mine", "kept": "yes"}
	port, _ := strconv.Atoi(strings.Split(r.glance.Listener.Addr().String(), ":")[1])
	m := &Importer{cfg: config.ImportSettings{StagingDir: t.TempDir(), AllowedSchemes: []string{"http"}, AllowedPorts: []int{port},
		DownloadProperties: []string{"hw_", "trait:", "os_distro", "os_secure_boot", "os_type", "os_glance"}},
		svc: s, now: time.Now, client: http.DefaultClient, data: &UploadCoordinator{limits: UploadLimits{MaxBytes: 1 << 20}}}
	m.SetKeystone(r.keystone.URL+"/v3/", http.DefaultClient)
	return m, repo
}

func downloadJob() importJob {
	return importJob{method: "glance-download", token: "user-token", region: "regiontwo", remoteID: remoteID, iface: "public"}
}

func TestGlanceDownloadStagesTheRemoteImage(t *testing.T) {
	r := newRemoteGlance(t)
	m, repo := downloadFixture(t, r)
	if err := m.glanceDownload(context.Background(), sharedID, "task", downloadJob()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(m.staged(sharedID)); !bytes.Equal(got, r.payload) {
		t.Fatal("staged bytes differ")
	}
	i := repo.image
	if *i.DiskFormat != "qcow2" || *i.ContainerFormat != "bare" {
		t.Fatal("formats not taken from the remote image", *i.DiskFormat, *i.ContainerFormat)
	}
	want := map[string]string{"os_distro": "cirros", "kept": "yes", "hw_disk_bus": "scsi", "trait:CUSTOM_X": "required"}
	if len(i.Properties) != len(want) {
		t.Fatal("properties", i.Properties)
	}
	for k, v := range want {
		if i.Properties[k] != v {
			t.Fatal("properties", i.Properties)
		}
	}
	for _, tok := range r.tokens {
		if tok != "user-token" {
			t.Fatal("the user's token was not used", r.tokens)
		}
	}
	if len(r.tokens) != 3 { // catalog, metadata, data
		t.Fatal("calls", len(r.tokens))
	}
}

func TestGlanceDownloadFailuresRevertTheImage(t *testing.T) {
	for _, tc := range []string{"queued-remote", "no-region", "internal-only", "size-mismatch", "too-large", "filtered", "keystone-down", "missing-remote"} {
		t.Run(tc, func(t *testing.T) {
			r := newRemoteGlance(t)
			m, repo := downloadFixture(t, r)
			job := downloadJob()
			want := ""
			switch tc {
			case "queued-remote":
				r.status, want = "queued", "Source image status should be active instead of queued"
			case "no-region":
				job.region, want = "RegionThree", "public glance endpoint not found for region RegionThree"
			case "internal-only":
				job.region, job.iface, want = "RegionOne", "internal", "internal glance endpoint not found for region RegionOne"
			case "size-mismatch":
				r.size, want = r.size+1, "Task task failed because downloaded data size 13000 is different from expected 13001"
			case "too-large":
				m.data.limits.MaxBytes = 100
			case "filtered":
				m.cfg.AllowedPorts = []int{443}
				want = "does not pass filtering"
			case "keystone-down":
				m.keystone = "http://127.0.0.1:1/v3"
				want = "the service catalog could not be read from Keystone"
			case "missing-remote":
				job.remoteID, want = "00000000-0000-4000-8000-000000000000", "glance-download returned HTTP 404"
			}
			err := m.glanceDownload(context.Background(), sharedID, "task", job)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatal("error", err, "want", want)
			}
			i := repo.image
			if *i.DiskFormat != "raw" || *i.ContainerFormat != "ovf" || len(i.Properties) != 2 || i.Properties["os_distro"] != "mine" || i.Properties["kept"] != "yes" {
				t.Fatal("not reverted", *i.DiskFormat, *i.ContainerFormat, i.Properties)
			}
		})
	}
}

func TestGlanceDownloadRequestIsValidatedAsTheOriginal(t *testing.T) {
	parse := func(body string) ImportRequest {
		var req ImportRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		req.Raw = json.RawMessage(body)
		return req
	}
	for body, want := range map[string]string{
		`{"method":{"name":"glance-download","glance_image_id":"` + remoteID + `"}}`:      "'glance_region' needs to be set for glance-download import method",
		`{"method":{"name":"glance-download","glance_region":"RegionTwo"}}`:               "'glance_image_id' needs to be set for glance-download import method",
		`{"method":{"name":"glance-download","glance_region":"R","glance_image_id":"x"}}`: "Remote image id does not look like a UUID: x",
	} {
		req := parse(body)
		err := checkGlanceDownload(&req)
		if domainKind(err) != model.Invalid || err.Error() != want {
			t.Fatal(body, err)
		}
	}
	// The interface defaults to public, also in the task's import_req.
	req := parse(`{"method":{"name":"glance-download","glance_region":"RegionTwo","glance_image_id":"` + remoteID + `"}}`)
	if err := checkGlanceDownload(&req); err != nil || req.Method.GlanceServiceInterface != "public" ||
		!strings.Contains(string(req.Raw), `"glance_service_interface":"public"`) {
		t.Fatal("default interface", err, req.Method.GlanceServiceInterface, string(req.Raw))
	}
	req = parse(`{"method":{"name":"glance-download","glance_region":"RegionTwo","glance_image_id":"` + strings.ReplaceAll(remoteID, "-", "") + `","glance_service_interface":"internal"}}`)
	if err := checkGlanceDownload(&req); err != nil || req.Method.GlanceServiceInterface != "internal" {
		t.Fatal("explicit interface or hex UUID", err)
	}
}

// The token goes only to Keystone and the remote Glance named in the catalog.
func TestGlanceDownloadEndpointMatchesRegionCaseInsensitively(t *testing.T) {
	r := newRemoteGlance(t)
	m, _ := downloadFixture(t, r)
	got, err := m.glanceEndpoint(context.Background(), "user-token", "REGIONTWO", "public")
	u, _ := url.Parse(got)
	if err != nil || u.Path != "/image" {
		t.Fatal(got, err)
	}
}
