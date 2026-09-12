package a2ui

import (
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
