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
