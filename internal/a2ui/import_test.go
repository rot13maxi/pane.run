package a2ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-surface/agent-surface/internal/schema"
)

func TestImportBasicBatchAndInitialValues(t *testing.T) {
	batch := `{"version":"v0.9","createSurface":{"surfaceId":"review","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json","theme":{"primaryColor":"#fff"}}}
{"version":"v0.9","updateComponents":{"surfaceId":"review","components":[{"id":"root","component":"Column","children":["title","name","choice","ready","submit"]},{"id":"title","component":"Text","text":"Review","variant":"h2"},{"id":"name","component":"TextField","label":"Name","value":{"path":"/form/name"},"variant":"shortText"},{"id":"choice","component":"ChoicePicker","label":"Direction","variant":"mutuallyExclusive","options":[{"label":"One","value":"one"},{"label":"Two","value":"two"}],"value":{"path":"/form/choice"}},{"id":"ready","component":"CheckBox","label":"Ready","value":{"path":"/form/ready"}},{"id":"submit_label","component":"Text","text":"Finish"},{"id":"submit","component":"Button","child":"submit_label","action":{"event":{"name":"submit"}}}]}}
{"version":"v0.9","updateDataModel":{"surfaceId":"review","path":"/form","value":{"name":"Ada","choice":["two"],"ready":true}}}`
	got, err := Import([]byte(batch), Options{Title: "Decision", TTLSeconds: 600})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Title != "Decision" || got.Spec.TTLSeconds != 600 || got.Spec.Actions.Submit == nil || got.Spec.Actions.Submit.Label != "Finish" {
		t.Fatalf("unexpected spec metadata: %#v", got.Spec)
	}
	if len(got.Spec.Components) != 4 || got.Spec.Components[0].Kind != schema.KindHeading || got.Spec.Components[2].Kind != schema.KindSelect {
		t.Fatalf("components: %#v", got.Spec.Components)
	}
	if got.Values["name"] != "Ada" || got.Values["choice"] != "two" || got.Values["ready"] != true {
		t.Fatalf("values: %#v", got.Values)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("expected ignored-theme warning")
	}
}

func TestImportArrayAndIDNormalization(t *testing.T) {
	batch := `[{"version":"v0.9.1","createSurface":{"surfaceId":"s","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"}},{"version":"v0.9.1","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Column","children":["1 bad id"]},{"id":"1 bad id","component":"TextField","label":"Value","value":"ok"}]}}]`
	got, err := Import([]byte(batch), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Components[0].ID != "a2ui_1_bad_id" || got.Values["a2ui_1_bad_id"] != "ok" {
		t.Fatalf("normalization failed: %#v %#v", got.Spec.Components, got.Values)
	}
}

func TestImportRejectsUnsafeOrIncompleteA2UI(t *testing.T) {
	tests := []struct{ name, batch, want string }{
		{"custom catalog", `{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"https://example.test/custom"}}`, "unsupported catalogId"},
		{"function", `{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"}} {"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":{"call":"now"}}]}}`, "function calls are not supported"},
		{"external action", `{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"}} {"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Button","action":{"event":{"name":"sendEmail"}}}]}}`, "unsupported Button event"},
		{"cycle", `{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"}} {"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Column","children":["root"]}]}}`, "cycle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Import([]byte(tt.batch), Options{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestImportRejectsExponentialSharedComponentExpansion(t *testing.T) {
	components := make([]map[string]any, 0, 22)
	for i := 0; i < 20; i++ {
		child := fmt.Sprintf("node-%d", i+1)
		components = append(components, map[string]any{
			"id":        fmt.Sprintf("node-%d", i),
			"component": "Column",
			"children":  []string{child, child},
		})
	}
	components[0]["id"] = "root"
	components[0]["children"] = []string{"node-1", "node-1"}
	components = append(components, map[string]any{"id": "node-20", "component": "Text", "text": "leaf"})

	batch, err := json.Marshal([]any{
		map[string]any{"version": Version, "createSurface": map[string]any{"surfaceId": "s", "catalogId": BasicCatalog}},
		map[string]any{"version": Version, "updateComponents": map[string]any{"surfaceId": "s", "components": components}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Import(batch, Options{})
	if err == nil || !strings.Contains(err.Error(), "translated A2UI surface exceeds 200 components") {
		t.Fatalf("err=%v, want expanded component limit", err)
	}
}

func TestCompilerMemoizesSharedZeroOutputSubgraphs(t *testing.T) {
	all := make(map[string]map[string]any, MaxComponents)
	for i := 0; i < MaxComponents-1; i++ {
		id := fmt.Sprintf("node-%d", i)
		child := fmt.Sprintf("node-%d", i+1)
		all[id] = map[string]any{"component": "Column", "children": []any{child, child}}
	}
	all[fmt.Sprintf("node-%d", MaxComponents-1)] = map[string]any{
		"component": "Button",
		"action":    map[string]any{"event": map[string]any{"name": "submit"}},
	}
	c := compiler{all: all, compiled: map[string][]schema.Component{}, ids: map[string]string{}, used: map[string]bool{}}

	got, err := c.walk("node-0", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("components=%d, want zero", len(got))
	}
	if c.expansionWork > 2*MaxComponents {
		t.Fatalf("shared graph required %d expansion steps, want at most %d", c.expansionWork, 2*MaxComponents)
	}
}

func TestCompilerBoundsReferenceWork(t *testing.T) {
	children := make([]any, maxExpansionWork)
	for i := range children {
		children[i] = "button"
	}
	c := compiler{
		all: map[string]map[string]any{
			"root":   {"component": "Column", "children": children},
			"button": {"component": "Button", "action": map[string]any{"event": map[string]any{"name": "submit"}}},
		},
		compiled: map[string][]schema.Component{},
		ids:      map[string]string{},
		used:     map[string]bool{},
	}

	_, err := c.walk("root", map[string]bool{})
	if err == nil || !strings.Contains(err.Error(), "safe work limit") {
		t.Fatalf("err=%v, want safe work limit", err)
	}
}
