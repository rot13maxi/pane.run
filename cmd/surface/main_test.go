package main

import (
	"bytes"
	"encoding/json"
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

	"github.com/agent-surface/agent-surface/internal/schema"
)

func TestReorderFlagsAllowsAgentFriendlyTrailingFlags(t *testing.T) {
	got := reorderFlags([]string{"abc", "spec.json", "--server", "https://example.test", "--token", "secret"})
	want := []string{"--server", "https://example.test", "--token", "secret", "abc", "spec.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reorderFlags() = %#v, want %#v", got, want)
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
	t.Setenv("SURFACE_CONFIG_DIR", t.TempDir())
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
	t.Setenv("SURFACE_CONFIG_DIR", t.TempDir())
	var out bytes.Buffer
	err := recipe("pick", []string{"First choice", "Second choice", "--title", "Pick one", "--server", server.URL}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Pick one" || len(got.Components) != 1 || got.Components[0].Kind != schema.KindSelect || !got.Components[0].Required {
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
	t.Setenv("SURFACE_CONFIG_DIR", t.TempDir())
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
	t.Setenv("SURFACE_CONFIG_DIR", t.TempDir())
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
		{"pick", schema.KindSelect}, {"sort", schema.KindRanking},
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
