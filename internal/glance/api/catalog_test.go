package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/service"
)

func patchRequest(h http.Handler, path, body, media string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PATCH", path, strings.NewReader(body))
	r.Header.Set("X-Auth-Token", "fixture-token")
	r.Header.Set("Content-Type", media)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestWWWAuthenticateFromConfiguredKeystone(t *testing.T) {
	data, err := os.ReadFile("../../../tests/compatibility/fixtures/glance-python-401.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Headers map[string]string `json:"headers"`
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("fixture")
	}
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "invalid"}[invalid], func(t *testing.T) {
			h, _, a, _ := harness()
			r := httptest.NewRequest("GET", "/v2/images", nil)
			if invalid {
				a.err = auth.ErrInvalidToken
				r.Header.Set("X-Auth-Token", "not-valid")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 401 || w.Header().Get("WWW-Authenticate") != fixture.Headers["www-authenticate"] || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal(w.Code, w.Header())
			}
		})
	}
	for _, endpoint := range []string{"http://host/v2", "https://user:secret@host/v3", "https://host/v3?secret=x", "https://host/v3#x", "https://host/identity\r\nX-Token: secret/v3"} {
		if _, err := New(service.New(&repoFake{}, time.Now), &authFake{}, "http://127.0.0.1:9293", endpoint, 100, slog.Default()); err == nil {
			t.Fatal("unsafe challenge endpoint accepted", endpoint)
		}
	}
	h, err := New(service.New(&repoFake{}, time.Now), &authFake{}, "http://127.0.0.1:9293", "https://identity.example:8443/identity/v3/", 100, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v2/images", nil))
	if w.Header().Get("WWW-Authenticate") != `Keystone uri="https://identity.example:8443/identity"` {
		t.Fatal(w.Header())
	}
}

func TestListHTTPEmptyMultipleAndLinks(t *testing.T) {
	h, r, a, _ := harness()
	w := request(h, "GET", "/v2/images", "")
	var empty map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &empty) != nil || !reflect.DeepEqual(empty["images"], []any{}) || empty["first"] != "/v2/images" || empty["schema"] != "/v2/schemas/images" {
		t.Fatal(w.Body.String())
	}
	for n := 0; n < 2; n++ {
		i, _ := model.New(model.CreateInput{}, "p", time.Now())
		r.images = append(r.images, i)
	}
	w = request(h, "GET", "/v2/images?limit=2&sort=name:asc&tag=a&tag=b&name=example&description=custom", "")
	var page map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page["images"].([]any)) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	next, _ := url.Parse(page["next"].(string))
	if next.Query().Get("marker") != r.images[1].ID || !reflect.DeepEqual(next.Query()["tag"], []string{"a", "b"}) || next.Query().Get("limit") != "2" {
		t.Fatal(next)
	}
	if r.listOptions.ProjectID != "p" || r.listOptions.Admin || r.listOptions.Filters["name"] != "example" || r.listOptions.Filters["description"] != "custom" || r.listOptions.Sort[0].Key != "name" {
		t.Fatal(r.listOptions)
	}
	// Last full page still has next; links preserve repeated filters, first omits marker.
	w = request(h, "GET", "/v2/images?limit=2&marker="+r.images[0].ID, "")
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if strings.Contains(page["first"].(string), "marker") || page["next"] == nil {
		t.Fatal(page)
	}
	a.identity.Roles = []string{"member"}
	if w = request(h, "GET", "/v2/images", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestListQueryValidationAndSorting(t *testing.T) {
	for _, query := range []string{"limit=-1", "limit=x", "limit=1&limit=2", "marker=", "marker=bad", "sort=id:asc&sort_key=name", "sort=id:wrong", "sort=wrong", "sort=id,id", "sort_key=id&sort_dir=asc&sort_dir=desc", "protected=1", "os_hidden=all", "visibility=unknown", "min_ram=-1", "tag=%FF", "deleted=true", "bad=%ZZ", "name=x;limit=1"} {
		t.Run(query, func(t *testing.T) {
			h, _, _, _ := harness()
			w := request(h, "GET", "/v2/images?"+query, "")
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, query := range []string{"limit=0", "limit=99999", "sort=name:asc,size:desc", "sort_key=name&sort_key=size&sort_dir=asc", "tag=a&tag=b", "os_hidden=TRUE&visibility=all", "member_status=accepted"} {
		if _, err := parseList(query); err != nil {
			t.Fatal(query, err)
		}
	}
	o, err := parseList("sort_key=name&sort_key=size&sort_dir=asc")
	if err != nil || o.Sort[len(o.Sort)-1].Direction != "asc" {
		t.Fatal(o, err)
	}
	if o, err = parseList("sort=name:asc,size:asc"); err != nil || o.Sort[len(o.Sort)-1].Direction != "desc" {
		t.Fatal(o, err)
	}
	if _, err = parseList(strings.Repeat("x", 16385)); err == nil {
		t.Fatal("unbounded query accepted")
	}
}

func TestPatchHTTPAndAtomicFailure(t *testing.T) {
	h, r, a, _ := harness()
	if w := request(h, "POST", "/v2/images", `{"name":"original","description":"before","tags":["a"]}`); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	path := "/v2/images/" + r.image.ID
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`[{"op":"add","path":"/description","value":"after"}]`, 200},
		{`[{"op":"replace","path":"/description","value":"changed"}]`, 200},
		{`[{"op":"add","path":"/a~1b~0c","value":"escaped"}]`, 200},
		{`[{"op":"remove","path":"/description"}]`, 200},
		{`[{"op":"replace","path":"/absent","value":"x"}]`, 409},
		{`[{"op":"remove","path":"/absent"}]`, 409},
		{`[{"op":"replace","path":"/owner","value":"other"}]`, 403},
		{`[{"op":"replace","path":"/status","value":"active"}]`, 403},
		{`[{"op":"remove","path":"/name"}]`, 403},
		{`[{"op":"replace","path":"/visibility","value":"public"}]`, 403},
		{`[{"op":"replace","path":"/name","value":"partial"},{"op":"replace","path":"/min_ram","value":-1}]`, 400},
	} {
		before := r.image.UpdatedAt
		w := patchRequest(h, path, tc.body, patchMediaType)
		if w.Code != tc.status {
			t.Fatal(tc.body, w.Code, w.Body.String())
		}
		if tc.status >= 400 && (!r.image.UpdatedAt.Equal(before) || *r.image.Name != "original") {
			t.Fatal("partial patch persisted")
		}
	}
	if r.image.Properties["a/b~c"] != "escaped" {
		t.Fatal("JSON pointer decoded incorrectly")
	}
	a.identity.Roles = []string{"reader"}
	if w := patchRequest(h, path, `[]`, patchMediaType); w.Code != 403 {
		t.Fatal(w.Code)
	}
	a.identity.Roles = []string{"member"}
	a.identity.ProjectID = "other"
	if w := patchRequest(h, path, `[]`, patchMediaType); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestPatchMalformedAndBounds(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `[null]`, `[] []`, `[{"op":"copy","path":"/x","value":"v"}]`, `[{"op":"add","path":"/x"}]`, `[{"op":"add","op":"remove","path":"/x","value":"v"}]`, `[{"op":"add","path":"/x","value":"v","from":"/y"}]`, `[{"op":null,"path":"/x","value":"v"}]`, `[{"op":"add","path":null,"value":"v"}]`, `[{"op":"add","path":"x","value":"v"}]`, `[{"op":"add","path":"/x/0","value":"v"}]`, `[{"op":"add","path":"/tags/-","value":"v"}]`, `[{"op":"add","path":"/x~2","value":"v"}]`, `[{"op":"add","path":"/x~","value":"v"}]`} {
		t.Run(body, func(t *testing.T) {
			h, _, _, _ := harness()
			w := patchRequest(h, "/v2/images/00000000-0000-4000-8000-000000000000", body, patchMediaType)
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	h, repo, _, _ := harness()
	w := patchRequest(h, "/v2/images/id", `[]`, "application/json")
	if w.Code != 415 || w.Header().Get("Accept-Patch") != patchMediaType {
		t.Fatal(w.Code, w.Header())
	}
	request(h, "POST", "/v2/images", `{}`)
	invalidUTF8 := `[{"op":"add","path":"/description","value":"` + string([]byte{255}) + `"}]`
	if w = patchRequest(h, "/v2/images/"+repo.image.ID, invalidUTF8, patchMediaType); w.Code != 400 {
		t.Fatal(w.Code)
	}
	h2, err := New(service.New(&repoFake{}, time.Now), &authFake{identity: identityForAPI()}, "http://127.0.0.1:9293", "http://host/identity/v3", 4, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if w = patchRequest(h2, "/v2/images/id", strings.Repeat("x", 5), patchMediaType); w.Code != 413 {
		t.Fatal(w.Code)
	}
}

func identityForAPI() auth.Identity {
	return auth.Identity{UserID: "u", ProjectID: "p", Roles: []string{"member", "reader"}, ExpiresAt: time.Now().Add(time.Hour)}
}
func TestDeleteLifecycleAndSchemas(t *testing.T) {
	h, r, a, _ := harness()
	w := request(h, "POST", "/v2/images", `{"protected":true,"description":"keep","tags":["retain"]}`)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	path := "/v2/images/" + r.image.ID
	if w = request(h, "DELETE", path, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = patchRequest(h, path, `[{"op":"replace","path":"/protected","value":false}]`, patchMediaType); w.Code != 200 {
		t.Fatal(w.Code)
	}
	a.identity.Roles = []string{"reader"}
	if w = request(h, "DELETE", path, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	a.identity.Roles = []string{"member", "reader"}
	if w = request(h, "DELETE", path, ""); w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if r.image.Properties["description"] != "keep" || len(r.image.Tags) != 1 {
		t.Fatal("tombstone children were destroyed")
	}
	for _, method := range []string{"GET", "DELETE"} {
		if w = request(h, method, path, ""); w.Code != 404 {
			t.Fatal(method, w.Code)
		}
	}
	w = request(h, "GET", "/v2/images", "")
	var list struct {
		Images []any `json:"images"`
	}
	if json.Unmarshal(w.Body.Bytes(), &list) != nil || list.Images == nil || len(list.Images) != 0 {
		t.Fatal(w.Body.String())
	}
	for _, name := range []string{"image", "images"} {
		if w = request(h, "GET", "/v2/schemas/"+name, ""); w.Code != 200 {
			t.Fatal(w.Code)
		}
		var schema map[string]any
		if json.Unmarshal(w.Body.Bytes(), &schema) != nil || schema["name"] != name || schema["properties"] == nil {
			t.Fatal(w.Body.String())
		}
	}
	if w = request(h, "GET", "/v2/schemas/metadefs", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = request(h, "POST", "/v2/schemas/image", `{}`); w.Code != 405 {
		t.Fatal(w.Code)
	}
}
