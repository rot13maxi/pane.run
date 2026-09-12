package server

import (
	"bytes"
	"encoding/json"
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
