package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
)

func TestReorderFlagsAllowsAgentFriendlyTrailingFlags(t *testing.T) {
	got := reorderFlags([]string{"abc", "spec.json", "--server", "https://example.test", "--token", "secret"})
	want := []string{"--server", "https://example.test", "--token", "secret", "abc", "spec.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reorderFlags() = %#v, want %#v", got, want)
	}
}

func TestVersionReportsBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	t.Cleanup(func() { version, commit, buildDate = oldVersion, oldCommit, oldBuildDate })
	version, commit, buildDate = "v1.2.3", "abc123", "2026-09-12T00:00:00Z"

	var out bytes.Buffer
	if err := run([]string{"version"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "pane v1.2.3 (commit abc123, built 2026-09-12T00:00:00Z)\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestDefaultServerIsHostedService(t *testing.T) {
	if defaultServer != "https://pane.run" {
		t.Fatalf("defaultServer = %q, want hosted service", defaultServer)
	}
}

func TestRequestFormatsJSONErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"code":"invalid_spec","message":"title is required"}}`)
	}))
	defer server.Close()

	_, err := newClient(server.URL).request(http.MethodPost, "/api/v1/surfaces", "", "application/json", nil)
	want := "service returned 400 Bad Request: invalid_spec: title is required"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestRequestDoesNotDumpHTMLResponse(t *testing.T) {
	const html = `<!doctype html><html><body><h1>Not Found</h1><p>sensitive noisy proxy response</p></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, html)
	}))
	defer server.Close()

	_, err := newClient(server.URL).request(http.MethodGet, "/api/v1/surfaces/missing", "", "application/json", nil)
	want := "service returned 404 Not Found with a non-JSON error response; check --server or PANE_SERVER"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
	if strings.Contains(err.Error(), "<!doctype") || strings.Contains(err.Error(), "sensitive noisy proxy response") {
		t.Fatalf("HTML response leaked into error: %q", err)
	}
}

func TestReplaceAssetsRecursesWithoutChangingUnknownPlaceholders(t *testing.T) {
	var input any
	if err := json.Unmarshal([]byte(`{"hero":"asset:cover","items":[{"src":"asset:thumb"}],"other":"asset:missing"}`), &input); err != nil {
		t.Fatal(err)
	}
	got := replaceAssets(input, map[string]string{"cover": "/a/1", "thumb": "/a/2"})
	encoded, _ := json.Marshal(got)
	want := `{"hero":"/a/1","items":[{"src":"/a/2"}],"other":"asset:missing"}`
	if string(encoded) != want {
		t.Fatalf("replaceAssets() = %s, want %s", encoded, want)
	}
}

func TestResolveReceiptUsesPrivateFile(t *testing.T) {
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	want := receipt{ID: "surface-1", Server: "https://surface.test", ManagementToken: "secret"}
	if err := saveReceipt(want); err != nil {
		t.Fatal(err)
	}
	got, err := resolveReceipt(want.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveReceipt() = %#v, want %#v", got, want)
	}
}

func TestParseFlagsSupportsTrailingBooleanFlags(t *testing.T) {
	fs := flagSetForTest(t)
	multi := fs.Bool("multi", false, "")
	title := fs.String("title", "", "")
	if err := parseFlags(fs, []string{"one", "two", "--multi", "--title", "Choices"}); err != nil {
		t.Fatal(err)
	}
	if !*multi || *title != "Choices" || !reflect.DeepEqual(fs.Args(), []string{"one", "two"}) {
		t.Fatalf("multi=%v title=%q args=%v", *multi, *title, fs.Args())
	}
}

func flagSetForTest(t *testing.T) *flag.FlagSet {
	t.Helper()
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func TestPickRecipeBuildsExpectedSpec(t *testing.T) {
	var got schema.Spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/surfaces" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"s1","url":"http://example/s/p1","management_token":"secret"}`)
	}))
	defer server.Close()
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	var out bytes.Buffer
	err := recipe("pick", []string{"First choice", "Second choice", "--title", "Pick one", "--server", server.URL, "--theme", "dark"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Pick one" || got.Presentation.ColorScheme != "dark" || len(got.Components) != 1 || got.Components[0].Kind != schema.KindSelect || !got.Components[0].Required {
		t.Fatalf("unexpected spec: %#v", got)
	}
	if len(got.Components[0].Options) != 2 || got.Components[0].Options[0].Label != "First choice" {
		t.Fatalf("unexpected options: %#v", got.Components[0].Options)
	}
}

func TestGalleryRecipeUploadsAndRewritesAssets(t *testing.T) {
	image := filepath.Join(t.TempDir(), "Warm card.png")
	if err := os.WriteFile(image, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	var created, updated schema.Spec
	var id string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/surfaces":
			json.NewDecoder(r.Body).Decode(&created)
			id = "s2"
			io.WriteString(w, `{"id":"s2","url":"http://example/s/p2","management_token":"secret"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/surfaces/s2/assets":
			io.WriteString(w, `{"url":"http://example/a/image"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/surfaces/s2":
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Fatal("missing management token")
			}
			json.NewDecoder(r.Body).Decode(&updated)
			io.WriteString(w, `{}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	if err := recipe("gallery", []string{"Warm=" + image, "--multi", "--server", server.URL}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if id == "" || created.Components[0].Items[0].Image != "asset:image_1" {
		t.Fatalf("create asset=%q", created.Components[0].Items[0].Image)
	}
	if updated.Components[0].Items[0].Image != "http://example/a/image" {
		t.Fatalf("updated asset=%q", updated.Components[0].Items[0].Image)
	}
	if updated.Components[0].MaxSelections != nil {
		t.Fatal("multi gallery unexpectedly capped")
	}
}

func TestComponentFromAddUsesStableUniqueIDsAndBounds(t *testing.T) {
	existing := []schema.Component{{Kind: schema.KindNumber, ID: "weight", Label: "Old"}}
	c, asset, err := componentFromAdd("number", nil, "weight", "Weight", "kg", "", true, 2, true, 0, true, 500, existing)
	if err != nil {
		t.Fatal(err)
	}
	if asset != "" || c.ID != "weight_2" || c.Kind != schema.KindNumber || c.Min == nil || *c.Min != 0 || c.Max == nil || *c.Max != 500 {
		t.Fatalf("component=%#v asset=%q", c, asset)
	}
}

func TestRecipeHelpWorksWithoutServiceCall(t *testing.T) {
	for _, name := range []string{"gallery", "pick", "rank", "checklist", "approve"} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := recipe(name, []string{"--help"}, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Usage:") || strings.Contains(out.String(), "flag needs an argument") {
				t.Fatalf("bad help: %s", out.String())
			}
		})
	}
}

func TestRecipeInteractionsRequiredByDefaultWithOptionalEscape(t *testing.T) {
	var specs []schema.Spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var s schema.Spec
		json.NewDecoder(r.Body).Decode(&s)
		specs = append(specs, s)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, fmt.Sprintf(`{"id":"s%d","url":"http://example/s/p","management_token":"secret"}`, len(specs)))
	}))
	defer server.Close()
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	if err := recipe("pick", []string{"One", "--server", server.URL}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := recipe("pick", []string{"One", "--optional", "--server", server.URL}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !specs[0].Components[0].Required || specs[1].Components[0].Required {
		t.Fatalf("required defaults: %v %v", specs[0].Components[0].Required, specs[1].Components[0].Required)
	}
}

func TestAppendComponentRemovesBootstrapAndAddsSubmit(t *testing.T) {
	spec := schema.Spec{Version: schema.Version, Title: "Draft", Components: []schema.Component{{Kind: schema.KindDivider}}}
	input := schema.Component{Kind: schema.KindInputText, ID: "answer", Label: "Answer"}
	appendComponent(&spec, input)
	if len(spec.Components) != 1 || spec.Components[0].Kind != schema.KindInputText {
		t.Fatalf("components = %#v", spec.Components)
	}
	if spec.Actions.Submit == nil || spec.Actions.Submit.Label != "Done" {
		t.Fatalf("submit = %#v", spec.Actions.Submit)
	}
}

func TestAppendComponentPreservesExplicitDocumentAndAction(t *testing.T) {
	custom := &schema.Action{Label: "Send response"}
	spec := schema.Spec{Components: []schema.Component{{Kind: schema.KindText, Content: "Context"}}, Actions: schema.Actions{Submit: custom}}
	appendComponent(&spec, schema.Component{Kind: schema.KindCheckbox, ID: "ready", Label: "Ready"})
	if len(spec.Components) != 2 || spec.Components[0].Kind != schema.KindText {
		t.Fatalf("components = %#v", spec.Components)
	}
	if spec.Actions.Submit != custom {
		t.Fatalf("explicit submit was replaced: %#v", spec.Actions.Submit)
	}
}

func TestAddAliases(t *testing.T) {
	tests := []struct {
		name string
		want schema.ComponentKind
	}{
		{"pick", schema.KindSelect}, {"sort", schema.KindRanking}, {"approve", schema.KindApproval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, err := componentFromAdd(tt.name, []string{"One", "Two"}, "", "Priority", "", "", false, 2, false, 0, false, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			if c.Kind != tt.want {
				t.Fatalf("kind = %q, want %q", c.Kind, tt.want)
			}
		})
	}
}

type failIfRead struct{ read bool }

func (r *failIfRead) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("stdin was consumed")
}

func TestAddSurfaceIDPipelineAndExplicitID(t *testing.T) {
	var stage bytes.Buffer
	if err := writeAddEnvelope(&stage, addEnvelope{ID: "surface-7", URL: "https://example/s/7", Revision: 2, Status: schema.StatusActive}); err != nil {
		t.Fatal(err)
	}
	got, err := addSurfaceID("-", &stage)
	if err != nil {
		t.Fatal(err)
	}
	if got != "surface-7" {
		t.Fatalf("id=%q", got)
	}

	unread := &failIfRead{}
	got, err = addSurfaceID("explicit-id", unread)
	if err != nil {
		t.Fatal(err)
	}
	if got != "explicit-id" || unread.read {
		t.Fatalf("id=%q stdin read=%v", got, unread.read)
	}
}

func TestAddSurfaceIDRejectsBadPipelineInput(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"empty", "", "empty stdin"},
		{"malformed", "not json", "expected surface JSON"},
		{"missing id", `{"url":"https://example"}`, "omitted id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := addSurfaceID("-", strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAddEnvelopeIsCompactStableAndPrivate(t *testing.T) {
	var out bytes.Buffer
	value := addEnvelope{ID: "s1", URL: "https://example/s/p1", Revision: 3, Status: schema.StatusSubmitted}
	if err := writeAddEnvelope(&out, value); err != nil {
		t.Fatal(err)
	}
	want := `{"id":"s1","url":"https://example/s/p1","revision":3,"status":"submitted"}` + "\n"
	if out.String() != want {
		t.Fatalf("envelope=%q, want %q", out.String(), want)
	}
	if strings.Contains(out.String(), "token") || strings.Contains(out.String(), "spec") {
		t.Fatalf("private/full data leaked: %s", out.String())
	}
	id, err := addSurfaceID("-", strings.NewReader(out.String()))
	if err != nil {
		t.Fatal(err)
	}
	if id != "s1" {
		t.Fatalf("chained id=%q", id)
	}
}

func TestAddExplicitIDReturnsChainableEnvelope(t *testing.T) {
	config := t.TempDir()
	t.Setenv("PANE_CONFIG_DIR", config)
	spec := schema.Spec{Version: schema.Version, Title: "Draft", Components: []schema.Component{{Kind: schema.KindDivider}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing token")
		}
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"id": "s9", "url": "https://public.test/s/p9", "spec": spec, "result": schema.Result{Status: schema.StatusActive}})
		case http.MethodPut:
			var updated schema.Spec
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatal(err)
			}
			if len(updated.Components) != 1 || updated.Components[0].Kind != schema.KindInputText || updated.Actions.Submit == nil {
				t.Fatalf("updated spec=%#v", updated)
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "s9", "url": "https://public.test/s/p9", "management_token": "must-not-leak", "spec": updated, "result": schema.Result{Status: schema.StatusActive, Revision: 4}})
		default:
			t.Fatalf("method=%s", r.Method)
		}
	}))
	defer server.Close()
	if err := saveReceipt(receipt{ID: "s9", Server: server.URL, ManagementToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	unread := &failIfRead{}
	var out bytes.Buffer
	if err := add([]string{"s9", "input", "--label", "Answer"}, unread, &out); err != nil {
		t.Fatal(err)
	}
	if unread.read {
		t.Fatal("explicit id consumed stdin")
	}
	want := `{"id":"s9","url":"https://public.test/s/p9","revision":4,"status":"active"}` + "\n"
	if out.String() != want {
		t.Fatalf("output=%q want=%q", out.String(), want)
	}
}

func TestCreateA2UIUsesImportEndpoint(t *testing.T) {
	input := filepath.Join(t.TempDir(), "surface.jsonl")
	if err := os.WriteFile(input, []byte(`{"version":"v0.9"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var path, contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, contentType = r.URL.RequestURI(), r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"s-a2ui","url":"http://example/s/p","management_token":"secret"}`)
	}))
	defer server.Close()
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	if err := create([]string{input, "--format", "a2ui", "--title", "Review", "--ttl", "10m", "--server", server.URL}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/imports/a2ui?protocol=v0.9.1&title=Review&ttl_seconds=600" || contentType != "application/a2ui+json" {
		t.Fatalf("request path=%q content-type=%q", path, contentType)
	}
}

func TestCreateWithoutSpecSupportsTrailingTheme(t *testing.T) {
	var got schema.Spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"theme-create","url":"https://example/s/theme","management_token":"secret"}`)
	}))
	defer server.Close()
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	if err := create([]string{"--title", "Dark review", "--server", server.URL, "--theme", "dark"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Dark review" || got.Presentation.ColorScheme != "dark" {
		t.Fatalf("spec=%#v", got)
	}
}

func TestResultsReturnsOnlyAgentFriendlyResult(t *testing.T) {
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/surfaces/s-results/results" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing management token")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"submitted","revision":3,"values":{"decision":"approve"},"created_at":"2026-09-12T12:00:00Z","updated_at":"2026-09-12T12:05:00Z","submitted_at":"2026-09-12T12:05:00Z","expires_at":"2026-09-13T12:00:00Z"}`)
	}))
	defer server.Close()
	if err := saveReceipt(receipt{ID: "s-results", Server: server.URL, ManagementToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"results", "s-results"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "submitted" || got["revision"] != float64(3) || got["values"].(map[string]any)["decision"] != "approve" {
		t.Fatalf("results=%#v", got)
	}
	if _, exists := got["spec"]; exists {
		t.Fatalf("results leaked authoring data: %#v", got)
	}
}

func TestWaitPollsUntilSubmittedAndReturnsResult(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/surfaces/s-wait/results" {
			t.Fatalf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing management token")
		}
		status := schema.StatusActive
		if requests == 2 {
			status = schema.StatusSubmitted
		}
		json.NewEncoder(w).Encode(schema.Result{Status: status, Revision: uint64(requests), Values: map[string]any{"decision": "approve"}})
	}))
	defer server.Close()

	var out bytes.Buffer
	err := waitForSubmission(context.Background(), newClient(server.URL), receipt{ID: "s-wait", ManagementToken: "secret"}, time.Millisecond, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got schema.Result
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || got.Status != schema.StatusSubmitted || got.Revision != 2 || got.Values["decision"] != "approve" {
		t.Fatalf("requests=%d result=%#v", requests, got)
	}
}

func TestWaitHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(schema.Result{Status: schema.StatusActive, Values: map[string]any{}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := waitForSubmission(ctx, newClient(server.URL), receipt{ID: "s-wait", ManagementToken: "secret"}, time.Hour, io.Discard)
	if err == nil || err.Error() != "timed out waiting for surface submission" {
		t.Fatalf("error=%v", err)
	}
}

func TestWaitRejectsClosedSurface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(schema.Result{Status: schema.StatusClosed, Values: map[string]any{}})
	}))
	defer server.Close()
	err := waitForSubmission(context.Background(), newClient(server.URL), receipt{ID: "s-wait", ManagementToken: "secret"}, time.Hour, io.Discard)
	if err == nil || err.Error() != "surface was closed before submission" {
		t.Fatalf("error=%v", err)
	}
}

func TestWaitCommandSupportsTrailingTimeout(t *testing.T) {
	t.Setenv("PANE_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(schema.Result{Status: schema.StatusSubmitted, Values: map[string]any{}})
	}))
	defer server.Close()
	if err := saveReceipt(receipt{ID: "s-wait", Server: server.URL, ManagementToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"wait", "s-wait", "--timeout", "1s"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"wait", "s-wait", "--timeout", "-1s"}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("negative timeout error=%v", err)
	}
}

func TestRecipeRejectsInvalidThemeBeforeNetwork(t *testing.T) {
	err := recipe("pick", []string{"One", "--theme", "sepia", "--server", "http://should-not-connect.invalid"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "color_scheme") {
		t.Fatalf("err=%v", err)
	}
}

func TestSkillInstallCodex(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	var out bytes.Buffer
	if err := run([]string{"skill", "install", "codex"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(codexHome, "skills", "pane")
	content, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "name: pane") {
		t.Fatalf("unexpected skill: %s", content)
	}
	if err := run([]string{"skill", "install", "codex"}, io.Discard, io.Discard); err == nil {
		t.Fatal("second install should require --force")
	}
	if err := run([]string{"skill", "install", "--force", "codex"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestSkillPrintAndPath(t *testing.T) {
	t.Setenv("CODEX_HOME", "/tmp/codex-test")
	var out bytes.Buffer
	if err := run([]string{"skill", "print"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "name: pane") {
		t.Fatalf("print=%q", out.String())
	}
	out.Reset()
	if err := run([]string{"skill", "path", "codex"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "/tmp/codex-test/skills/pane" {
		t.Fatalf("path=%q", out.String())
	}
}

func TestSkillHarnessPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	tests := map[string]string{
		"claude":     filepath.Join(home, ".claude", "skills", "pane"),
		"pi":         filepath.Join(home, ".pi", "agent", "skills", "pane"),
		"omp":        filepath.Join(home, ".omp", "agent", "skills", "pane"),
		"hermes":     filepath.Join(home, ".hermes", "skills", "pane"),
		"opencode":   filepath.Join(home, ".config", "opencode", "skills", "pane"),
		"gemini-cli": filepath.Join(home, ".gemini", "skills", "pane"),
		"openclaw":   filepath.Join(home, ".openclaw", "skills", "pane"),
		"aider-desk": filepath.Join(home, ".aider-desk", "skills", "pane"),
	}
	for harness, want := range tests {
		t.Run(harness, func(t *testing.T) {
			var out bytes.Buffer
			if err := run([]string{"skill", "path", harness}, &out, io.Discard); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(out.String()); got != want {
				t.Fatalf("path=%q, want %q", got, want)
			}
		})
	}
}

func TestSkillInstallHarnessAndList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := run([]string{"skill", "install", "claude"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "pane", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Claude Code") {
		t.Fatalf("install output=%q", out.String())
	}
	out.Reset()
	if err := run([]string{"skill", "list"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"codex", "claude-code", "pi", "omp", "hermes-agent", "opencode"} {
		if !strings.Contains(out.String(), harness+"\t") {
			t.Fatalf("list missing %q: %s", harness, out.String())
		}
	}
	if err := run([]string{"skill", "path", "unknown"}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "pane skill list") {
		t.Fatalf("unknown harness error=%v", err)
	}
}
