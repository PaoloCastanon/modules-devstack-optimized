package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Replay the captured Python response, including Content-Type and every stable
// error field. Only request_id is volatile; details are never normalized.
func TestCapturedNegotiationAndCapacityErrors(t *testing.T) {
	for _, file := range []string{"20-reserved-total.json", "37-unsupported-media.json", "38-unacceptable.json"} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile("../../../evidence/sprint3/placement/b07-live-20261028/run/" + file)
			if err != nil {
				t.Fatal(err)
			}
			type response struct {
				Status  int               `json:"status"`
				Headers map[string]string `json:"headers"`
				Body    string            `json:"body_text"`
			}
			var captured struct {
				ID      string `json:"provider_uuid"`
				Request struct {
					Method, Path string
					Body         string `json:"body_text"`
					Headers      map[string]string
				}
				Responses map[string]response
			}
			if err := json.Unmarshal(data, &captured); err != nil {
				t.Fatal(err)
			}
			h, _, _ := harness(t)
			path := "/resource_providers/" + captured.ID
			if file != "37-unsupported-media.json" {
				if w := request(h, "POST", "/resource_providers", `{"uuid":"`+captured.ID+`","name":"contract-test"}`); w.Code != 201 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if file == "20-reserved-total.json" {
				w := request(h, "PUT", path+"/inventories", `{"resource_provider_generation":0,"inventories":{"VCPU":{"total":8}}}`)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			beforeProvider := request(h, "GET", path, "").Body.String()
			beforeInventory := request(h, "GET", path+"/inventories", "").Body.String()
			r := httptest.NewRequest(captured.Request.Method, captured.Request.Path, strings.NewReader(captured.Request.Body))
			r.Header.Set("X-Auth-Token", "test-token")
			for key, value := range captured.Request.Headers {
				r.Header.Set(key, value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if !strings.HasPrefix(w.Header().Get("X-Openstack-Request-Id"), "req-") {
				t.Fatal("missing request correlation", w.Header())
			}
			want := captured.Responses["python"]
			if w.Code != want.Status {
				t.Fatalf("status=%d want=%d", w.Code, want.Status)
			}
			for _, header := range []string{"content-type", "openstack-api-version", "vary"} {
				got, expected := w.Header().Get(header), want.Headers[header]
				if header == "vary" {
					got, expected = strings.ToLower(got), strings.ToLower(expected)
				}
				if got != expected {
					t.Errorf("%s=%q want=%q", header, got, expected)
				}
			}
			if want.Headers["content-type"] == "application/json" {
				compareErrorOrState(t, w.Body.String(), want.Body)
			} else if w.Body.String() != want.Body {
				t.Errorf("body=%q want=%q", w.Body.String(), want.Body)
			}
			// Rejections must retain both metadata and inventory generation/state.
			compareErrorOrState(t, request(h, "GET", path, "").Body.String(), beforeProvider)
			compareErrorOrState(t, request(h, "GET", path+"/inventories", "").Body.String(), beforeInventory)
		})
	}
}

func compareErrorOrState(t *testing.T, got, want string) {
	t.Helper()
	decode := func(raw string) map[string]any {
		t.Helper()
		var body map[string]any
		if json.Unmarshal([]byte(raw), &body) != nil {
			t.Fatal("invalid JSON", raw)
		}
		if errors, ok := body["errors"].([]any); ok {
			for _, item := range errors {
				delete(item.(map[string]any), "request_id")
			}
		}
		return body
	}
	if !reflect.DeepEqual(decode(got), decode(want)) {
		t.Errorf("JSON differs\ngot: %s\nwant: %s", got, want)
	}
}

func TestCapacityErrorsUseRequestedProviderAndClass(t *testing.T) {
	// The pinned 1.0 handler uses the same InvalidInventoryCapacity exception
	// for equal/excessive reserved, zero ratio and capacity truncated to zero.
	for _, tc := range []struct{ id, class, fields string }{
		{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "VCPU", `"total":4,"reserved":4`},
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "MEMORY_MB", `"total":4,"reserved":5`},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccccc", "DISK_GB", `"total":4,"allocation_ratio":0`},
		{"dddddddd-dddd-4ddd-8ddd-dddddddddddd", "VCPU", `"total":1,"allocation_ratio":0.5`},
	} {
		t.Run(tc.id, func(t *testing.T) {
			h, _, _ := harness(t)
			if w := request(h, "POST", "/resource_providers", `{"uuid":"`+tc.id+`","name":"capacity"}`); w.Code != 201 {
				t.Fatal(w.Code)
			}
			path := "/resource_providers/" + tc.id + "/inventories"
			before := request(h, "GET", path, "").Body.String()
			w := request(h, "PUT", path, fmt.Sprintf(`{"resource_provider_generation":0,"inventories":{"%s":{%s}}}`, tc.class, tc.fields))
			detail := fmt.Sprintf("The server could not comply with the request since it is either malformed or otherwise incorrect.\n\n Unable to update inventory for resource provider %s: Invalid inventory for '%s' on resource provider '%s'. The reserved value is greater than or equal to total.  ", tc.id, tc.class, tc.id)
			want, err := json.Marshal(map[string]any{"errors": []any{map[string]any{"status": 400, "title": "Bad Request", "detail": detail}}})
			if err != nil || w.Code != 400 {
				t.Fatal(w.Code, w.Body.String(), err)
			}
			compareErrorOrState(t, w.Body.String(), string(want))
			compareErrorOrState(t, request(h, "GET", path, "").Body.String(), before)
		})
	}
}

func TestUnsupportedMediaReportsParsedType(t *testing.T) {
	for _, media := range []string{"", "text/plain; charset=UTF-8", "application/xml"} {
		t.Run(media, func(t *testing.T) {
			h, _, _ := harness(t)
			r := httptest.NewRequest("POST", "/resource_providers", strings.NewReader(`{"name":"no-write"}`))
			r.Header.Set("X-Auth-Token", "test-token")
			r.Header.Set("Content-Type", media)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			badType := strings.SplitN(media, ";", 2)[0]
			if badType == "" {
				badType = "None"
			}
			detail := fmt.Sprintf("The request media type %s is not supported by this server. \n\n The media type %s is not supported, use application/json", badType, badType)
			want, err := json.Marshal(map[string]any{"errors": []any{map[string]any{"status": 415, "title": "Unsupported Media Type", "detail": detail}}})
			if err != nil || w.Code != 415 || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal(w.Code, w.Header(), err)
			}
			compareErrorOrState(t, w.Body.String(), string(want))
			w = request(h, "GET", "/resource_providers", "")
			sameJSON(t, w.Body.Bytes(), []byte(`{"resource_providers":[]}`))
		})
	}
}

func TestAcceptFailureIsSeparateFromMicroversionFailure(t *testing.T) {
	h, _, _ := harness(t)
	for _, tc := range []struct{ accept, version, media string }{
		{"application/json", "placement 1.1", "application/json"},
		{"text/plain", "placement 1.1", "application/json"},
		{"text/plain", "placement 1.0", "text/plain; charset=UTF-8"},
	} {
		r := httptest.NewRequest("GET", "/resource_providers", nil)
		r.Header.Set("X-Auth-Token", "test-token")
		r.Header.Set("Accept", tc.accept)
		r.Header.Set("OpenStack-API-Version", tc.version)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 406 || w.Header().Get("Content-Type") != tc.media {
			t.Fatal(tc, w.Code, w.Header())
		}
	}
}
