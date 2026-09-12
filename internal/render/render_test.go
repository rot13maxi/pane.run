package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-surface/agent-surface/internal/schema"
)

func TestRenderEscapesContentAndEmitsRuntimeContract(t *testing.T) {
	p := Page{
		Spec: schema.Spec{Title: `<script>alert("title")</script>`, Actions: schema.Actions{Submit: &schema.Action{}}, Components: []schema.Component{
			{Kind: schema.KindText, Content: `<img src=x onerror=alert(1)>`},
			{ID: "name", Kind: schema.KindInputText, Label: "Name", Required: true},
		}},
		Result:   schema.Result{Status: schema.StatusActive, Revision: 4, Values: map[string]any{"name": `Ada "Ace"`}},
		StateURL: "/state?x=1&y=2", SubmitURL: "/submit", ResetURL: "/reset",
	}
	var out bytes.Buffer
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`<!doctype html>`, `&lt;script&gt;alert`, `&lt;img src=x onerror=alert(1)&gt;`, `value="Ada &#34;Ace&#34;"`, `"revision":4`, `fetch(url,options)`, `method:'PUT'`, `aria-live="polite"`} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(html, `<script>alert("title")</script>`) || strings.Contains(html, `<img src=x onerror`) {
		t.Error("untrusted markup rendered as HTML")
	}
}

func TestRenderAllPrimitivesAndPreservesValues(t *testing.T) {
	min, max := 1, 2
	components := []schema.Component{
		{Kind: schema.KindHeading, Content: "Heading", Level: 2}, {Kind: schema.KindText, Content: "Copy"},
		{Kind: schema.KindImage, Asset: "/a/img", Alt: "Preview"}, {Kind: schema.KindLink, URL: "https://example.test", Label: "Link"}, {Kind: schema.KindDivider},
		{Kind: schema.KindSection, Label: "Section", Components: []schema.Component{{ID: "notes", Kind: schema.KindTextarea, Label: "Notes"}}},
		{ID: "num", Kind: schema.KindNumber, Label: "Number"}, {ID: "check", Kind: schema.KindCheckbox, Label: "Check"}, {ID: "toggle", Kind: schema.KindToggle, Label: "Toggle"},
		{ID: "select", Kind: schema.KindSelect, Label: "Select", Options: []schema.Option{{Value: "a", Label: "A"}}},
		{ID: "multi", Kind: schema.KindMultiSelect, Label: "Multi", MinSelections: &min, MaxSelections: &max, Options: []schema.Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
		{ID: "list", Kind: schema.KindChecklist, Label: "List", Items: []schema.Item{{Value: "x", Label: "X"}}},
		{ID: "gallery", Kind: schema.KindGallery, Label: "Gallery", Items: []schema.Item{{Value: "one", Label: "One", Image: "/one.png"}}},
		{ID: "rank", Kind: schema.KindRanking, Label: "Rank", Items: []schema.Item{{Value: "one", Label: "One"}, {Value: "two", Label: "Two"}}},
		{ID: "decision", Kind: schema.KindApproval, Label: "Decide"},
		{Kind: schema.KindComparison, Label: "Compare", Columns: []schema.Column{{Key: "a", Label: "A"}}, Rows: []schema.ComparisonRow{{Label: "Cost", Values: map[string]string{"a": "$1"}}}},
	}
	p := Page{Spec: schema.Spec{Title: "Everything", Components: components, Actions: schema.Actions{Submit: &schema.Action{}, Reset: &schema.Action{}}}, Result: schema.Result{Values: map[string]any{"check": true, "select": "a", "multi": []any{"b"}, "rank": []any{"two", "one"}, "decision": "approved"}}}
	var out bytes.Buffer
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`<h2 class="component">Heading</h2>`, `<textarea`, `type="number"`, `class="gallery"`, `data-value="two"`, `value="approved" checked`, `<table class="comparison">`, `id="submit"`, `id="reset"`} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Index(html, `data-value="two"`) > strings.Index(html, `data-value="one"`) {
		t.Error("saved ranking order was not rendered")
	}
}

func TestClosedSurfaceDisablesControls(t *testing.T) {
	p := Page{Spec: schema.Spec{Title: "Closed", Components: []schema.Component{{ID: "x", Kind: schema.KindInputText, Label: "X"}}}, Result: schema.Result{Status: schema.StatusClosed, Values: map[string]any{}}}
	var out bytes.Buffer
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"read_only":true`) {
		t.Fatal("closed result was not made read-only")
	}
}

func TestVisualContractIsEditorialTactileAndMobileSafe(t *testing.T) {
	p := Page{Spec: schema.Spec{
		Title:        "Visual review",
		Presentation: schema.Presentation{Tone: "neutral", Density: "compact"},
		Components:   []schema.Component{{ID: "gallery", Kind: schema.KindGallery, Label: "Choose", Items: []schema.Item{{Value: "one", Label: "One", Image: "/one.png"}}}},
	}, Result: schema.Result{Values: map[string]any{"gallery": []any{"one"}}}}
	var out bytes.Buffer
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`class="tone-neutral density-compact theme-system"`,
		`.tone-neutral,.tone-default{--accent:#355d48`,
		`--heading-font:Palatino,"Book Antiqua",Palatino,serif`,
		`.tone-professional{--accent:#4b5f8f`,
		`--heading-font:ui-sans-serif,system-ui`,
		`.tone-warm{--accent:#74445f`,
		`--heading-font:Georgia,"Times New Roman",serif`,
		`.tone-playful{--accent:#9a6a1f`,
		`--heading-font:"Trebuchet MS",Trebuchet`,
		`color-mix(in srgb,var(--accent) 24%,transparent)`,
		`.gallery .choice:has(input:checked)`,
		`object-fit:contain`,
		`@media(max-width:520px)`,
		`@media(prefers-reduced-motion:reduce)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing visual contract %q", want)
		}
	}
	if strings.Contains(html, `#ad5038`) {
		t.Error("legacy terracotta accent remains in visual contract")
	}
	if strings.LastIndex(html, `object-fit:cover`) > strings.LastIndex(html, `object-fit:contain`) {
		t.Error("gallery crop rule overrides contain rule")
	}
}

func TestColorSchemeClassesAndDarkTokenContract(t *testing.T) {
	for _, tc := range []struct {
		name, scheme, class string
	}{
		{name: "omitted follows system", class: "theme-system"},
		{name: "explicit system", scheme: "system", class: "theme-system"},
		{name: "explicit light", scheme: "light", class: "theme-light"},
		{name: "explicit dark", scheme: "dark", class: "theme-dark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Page{Spec: schema.Spec{Title: "Theme", Presentation: schema.Presentation{Tone: "professional", ColorScheme: tc.scheme}}, Result: schema.Result{Values: map[string]any{}}}
			var out bytes.Buffer
			if err := Render(&out, p); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.class) {
				t.Fatalf("output missing body class %q", tc.class)
			}
		})
	}

	var out bytes.Buffer
	p := Page{Spec: schema.Spec{Title: "Dark", Presentation: schema.Presentation{ColorScheme: "dark"}}, Result: schema.Result{Values: map[string]any{}}}
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`.theme-dark{color-scheme:dark`,
		`.theme-light{color-scheme:light`,
		`.theme-system{color-scheme:light dark`,
		`@media(prefers-color-scheme:dark){.theme-system`,
		`--bg:#211f1c`,
		`--card:#302c27`,
		`--ink:#f1eadf`,
		`--accent:var(--dark-accent)`,
		`background:var(--panel)`,
		`background:var(--control)`,
		`background:var(--media)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing dark-mode contract %q", want)
		}
	}
}

func TestDarkModeUsesSemanticForegroundTokens(t *testing.T) {
	p := Page{Spec: schema.Spec{Title: "Readable", Presentation: schema.Presentation{ColorScheme: "dark"}, Components: []schema.Component{{ID: "choice", Kind: schema.KindApproval, Label: "Decision"}}}, Result: schema.Result{Values: map[string]any{}}}
	var out bytes.Buffer
	if err := Render(&out, p); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`.label{font-size:.97rem;color:var(--ink);`,
		`input::placeholder,textarea::placeholder{color:var(--muted);opacity:1}`,
		`.gallery-copy{display:grid;gap:4px;color:var(--ink)}`,
		`.approval .choice{min-height:58px;color:var(--ink);background:var(--control)`,
		`.button{min-height:52px;color:var(--accent-ink);`,
		`--accent-ink:#171916`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing semantic dark foreground contract %q", want)
		}
	}
	if strings.Contains(html, `#3f352d`) {
		t.Error("hardcoded light-only label color remains")
	}
}
