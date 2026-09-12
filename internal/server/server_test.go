package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-surface/agent-surface/internal/schema"
	"github.com/agent-surface/agent-surface/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "", nil))
	return ts
}

func requestJSON(t *testing.T, client *http.Client, method, url string, body any, token string) (*http.Response, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return resp, out
}

func TestHTTPCreateRenderAndState(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	spec := schema.Spec{Version: schema.Version, Title: "Choose <style>", Components: []schema.Component{{ID: "choice", Kind: schema.KindSelect, Label: "Choice", Options: []schema.Option{{Value: "a", Label: "A"}}}}, Actions: schema.Actions{Submit: &schema.Action{}}}
	resp, created := requestJSON(t, client, "POST", ts.URL+"/api/v1/surfaces", spec, "")
	if resp.StatusCode != 201 {
		t.Fatalf("create=%d %#v", resp.StatusCode, created)
	}
	publicURL := ts.URL + created["url"].(string)
	page, err := client.Get(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(html), "Choose &lt;style&gt;") {
		t.Fatalf("unsafe or absent title: %s", html)
	}
	publicID := created["public_id"].(string)
	resp, state := requestJSON(t, client, "PUT", ts.URL+"/api/v1/public/"+publicID+"/state", map[string]any{"revision": 0, "values": map[string]any{"choice": "a"}}, "")
	if resp.StatusCode != 200 {
		t.Fatalf("state=%d %#v", resp.StatusCode, state)
	}
	resp, conflict := requestJSON(t, client, "PUT", ts.URL+"/api/v1/public/"+publicID+"/state", map[string]any{"revision": 0, "values": map[string]any{}}, "")
	if resp.StatusCode != 409 || conflict["result"] == nil {
		t.Fatalf("conflict=%d %#v", resp.StatusCode, conflict)
	}
	id, token := created["id"].(string), created["management_token"].(string)
	resp, read := requestJSON(t, client, "GET", ts.URL+"/api/v1/surfaces/"+id, nil, token)
	if resp.StatusCode != 200 || read["result"] == nil {
		t.Fatalf("read=%d %#v", resp.StatusCode, read)
	}
	resp, results := requestJSON(t, client, "GET", ts.URL+"/api/v1/surfaces/"+id+"/results", nil, token)
	if resp.StatusCode != 200 || results["status"] != "active" || results["revision"] != float64(1) {
		t.Fatalf("results=%d %#v", resp.StatusCode, results)
	}
	if _, exists := results["spec"]; exists {
		t.Fatalf("results included specification: %#v", results)
	}
	if results["values"].(map[string]any)["choice"] != "a" {
		t.Fatalf("results values=%#v", results["values"])
	}
}

func TestAssetUpload(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	spec := schema.Spec{Version: schema.Version, Title: "Asset", Components: []schema.Component{{Kind: schema.KindText, Content: "hello"}}}
	_, created := requestJSON(t, client, "POST", ts.URL+"/api/v1/surfaces", spec, "")
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/surfaces/"+created["id"].(string)+"/assets", strings.NewReader("image"))
	req.Header.Set("Authorization", "Bearer "+created["management_token"].(string))
	req.Header.Set("X-Filename", "test.txt")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var asset map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&asset)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload=%d %#v", resp.StatusCode, asset)
	}
	got, err := client.Get(ts.URL + asset["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if string(b) != "image" {
		t.Fatalf("asset=%q", b)
	}
}

func TestA2UIImportCreatesCanonicalSurfaceWithInitialState(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	batch := `{"version":"v0.9","createSurface":{"surfaceId":"review","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"}}
{"version":"v0.9","updateComponents":{"surfaceId":"review","components":[{"id":"root","component":"Column","children":["field"]},{"id":"field","component":"TextField","label":"Answer","value":{"path":"/answer"},"variant":"shortText"}]}}
{"version":"v0.9","updateDataModel":{"surfaceId":"review","path":"/answer","value":"yes"}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/imports/a2ui?protocol=v0.9.1&title=Review", strings.NewReader(batch))
	req.Header.Set("Content-Type", "application/a2ui+json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %#v", resp.StatusCode, created)
	}
	imp := created["import"].(map[string]any)
	if imp["surface_id"] != "review" || imp["protocol"] != "v0.9.1" {
		t.Fatalf("import metadata=%#v", imp)
	}
	_, read := requestJSON(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/surfaces/"+created["id"].(string), nil, created["management_token"].(string))
	spec := read["spec"].(map[string]any)
	result := read["result"].(map[string]any)
	if spec["title"] != "Review" || result["values"].(map[string]any)["field"] != "yes" {
		t.Fatalf("surface=%#v", read)
	}
}

func TestA2UIImportRejectsUnsupportedProtocol(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, body := requestJSON(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/imports/a2ui?protocol=v1.0", map[string]any{}, "")
	if resp.StatusCode != http.StatusBadRequest || body["error"].(map[string]any)["code"] != "unsupported_protocol" {
		t.Fatalf("response=%d %#v", resp.StatusCode, body)
	}
}

type memoryPages struct {
	pages     map[string][]byte
	deleteErr error
}

func (m *memoryPages) PutPage(s store.Surface, page []byte) error {
	if m.pages == nil {
		m.pages = make(map[string][]byte)
	}
	m.pages[s.PublicID] = append([]byte(nil), page...)
	return nil
}
func (m *memoryPages) DeletePage(publicID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.pages, publicID)
	return nil
}

func TestHostedServerPublishesCompletePage(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	pages := &memoryPages{}
	ts := httptest.NewServer(NewHosted(st, "https://surface.example", nil, pages))
	defer ts.Close()
	spec := schema.Spec{Version: schema.Version, Title: "Hosted page", Components: []schema.Component{{ID: "name", Kind: schema.KindInputText, Label: "Name"}}}
	resp, created := requestJSON(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/surfaces", spec, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %#v", resp.StatusCode, created)
	}
	publicID := created["public_id"].(string)
	page := string(pages.pages[publicID])
	for _, want := range []string{"<!doctype html>", "Hosted page", "/api/v1/public/" + publicID + "/state", "function hydrate(next)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("published page missing %q", want)
		}
	}
	id, token := created["id"].(string), created["management_token"].(string)
	resp, _ = requestJSON(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/surfaces/"+id, nil, token)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete=%d", resp.StatusCode)
	}
	if _, ok := pages.pages[publicID]; ok {
		t.Fatal("published page was not deleted")
	}
}

func TestHostedDeleteRetainsManagementRecordWhenPageDeletionFails(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	pages := &memoryPages{}
	ts := httptest.NewServer(NewHosted(st, "https://surface.example", nil, pages))
	defer ts.Close()
	spec := schema.Spec{Version: schema.Version, Title: "Retry deletion", Components: []schema.Component{{Kind: schema.KindText, Content: "Content"}}}
	resp, created := requestJSON(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/surfaces", spec, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %#v", resp.StatusCode, created)
	}
	id, token := created["id"].(string), created["management_token"].(string)
	publicID := created["public_id"].(string)
	pages.deleteErr = errors.New("temporary object-store failure")

	resp, _ = requestJSON(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/surfaces/"+id, nil, token)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("first delete=%d, want 500", resp.StatusCode)
	}
	if _, ok := pages.pages[publicID]; !ok {
		t.Fatal("page removed despite page-store failure")
	}
	resp, _ = requestJSON(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/surfaces/"+id, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("management record lost after failed delete: status=%d", resp.StatusCode)
	}

	pages.deleteErr = nil
	resp, _ = requestJSON(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/surfaces/"+id, nil, token)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("retry delete=%d, want 204", resp.StatusCode)
	}
}
