package schema

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func validSpec() Spec {
	return Spec{
		Version: Version,
		Title:   "Pick a direction",
		Components: []Component{
			{Kind: KindHeading, Content: "Mockups", Level: 2},
			{ID: "choices", Kind: KindGallery, Label: "Choose designs", Required: true,
				MinSelections: ptr(1), MaxSelections: ptr(2), Items: []Item{
					{Value: "one", Label: "One", Image: "/assets/one.png"},
					{Value: "two", Label: "Two", Image: "/assets/two.png"},
				}},
			{ID: "notes", Kind: KindTextarea, Label: "Notes", MaxLength: ptr(200)},
		},
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Spec)
		path string
	}{
		{"valid", func(*Spec) {}, ""},
		{"version", func(s *Spec) { s.Version = "2" }, "version"},
		{"ttl low", func(s *Spec) { s.TTLSeconds = 59 }, "ttl_seconds"},
		{"duplicate id", func(s *Spec) {
			s.Components = append(s.Components, Component{ID: "notes", Kind: KindCheckbox, Label: "Again"})
		}, "components[3].id"},
		{"bad selection bounds", func(s *Spec) { s.Components[1].MinSelections = ptr(3) }, "components[1]"},
		{"duplicate item", func(s *Spec) { s.Components[1].Items[1].Value = "one" }, "components[1].items[1].value"},
		{"gallery image", func(s *Spec) { s.Components[1].Items[0].Image = "" }, "components[1].items[0].image"},
		{"bad url", func(s *Spec) { s.Components = []Component{{Kind: KindLink, Label: "No", URL: "javascript:alert(1)"}} }, "components[0]"},
		{"interactive id", func(s *Spec) { s.Components[2].ID = "bad id" }, "components[2].id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec()
			tt.edit(&s)
			err := ValidateSpec(&s)
			if tt.path == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Path != tt.path {
				t.Fatalf("got %#v, want path %q", err, tt.path)
			}
		})
	}
}

func TestEffectiveTTL(t *testing.T) {
	if got := (Spec{}).EffectiveTTLSeconds(); got != DefaultTTLSeconds {
		t.Fatalf("got %d", got)
	}
	if got := (Spec{TTLSeconds: 90}).EffectiveTTLSeconds(); got != 90 {
		t.Fatalf("got %d", got)
	}
}

func TestValidateValuesPartialAndSubmission(t *testing.T) {
	spec := validSpec()
	partial := map[string]any{"notes": "still deciding", "choices": []any{"one"}}
	if err := ValidateValues(spec, partial); err != nil {
		t.Fatalf("partial: %v", err)
	}
	if err := ValidateSubmission(spec, partial); err != nil {
		t.Fatalf("submission: %v", err)
	}
	if err := ValidateSubmission(spec, map[string]any{"notes": "no selection"}); err == nil {
		t.Fatal("missing required value accepted")
	}
	if err := ValidateValues(spec, map[string]any{"choices": []any{"one", "two", "three"}}); err == nil {
		t.Fatal("too many values accepted")
	}
	if err := ValidateValues(spec, map[string]any{"unknown": true}); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestValueTypesAndConstraints(t *testing.T) {
	spec := Spec{Version: Version, Title: "Inputs", Components: []Component{
		{ID: "n", Kind: KindNumber, Label: "Number", Min: ptr(1.0), Max: ptr(3.0)},
		{ID: "b", Kind: KindToggle, Label: "Toggle"},
		{ID: "s", Kind: KindSelect, Label: "Select", Options: []Option{{Value: "a", Label: "A"}}},
		{ID: "r", Kind: KindRanking, Label: "Rank", Items: []Item{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
		{ID: "a", Kind: KindApproval, Label: "Approve"},
	}}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"n": float64(2), "b": true, "s": "a", "r": []any{"b", "a"}, "a": "approved"}
	if err := ValidateValues(spec, valid); err != nil {
		t.Fatal(err)
	}
	bad := []map[string]any{
		{"n": 4.0}, {"b": "yes"}, {"s": "z"}, {"r": []any{"a", "a"}},
		{"r": []any{"a", "b", "arbitrary_free_text_inserted_at_3"}}, {"a": "maybe"},
	}
	for _, values := range bad {
		if err := ValidateValues(spec, values); err == nil {
			t.Fatalf("accepted %#v", values)
		}
	}
}

func TestInitializeValuesUsesAuthoredRankingOrder(t *testing.T) {
	spec := Spec{Components: []Component{{ID: "rank", Kind: KindRanking, Items: []Item{{Value: "a"}, {Value: "b"}}}}}
	got := InitializeValues(spec, map[string]any{"other": true})
	if !reflect.DeepEqual(got["rank"], []string{"a", "b"}) || got["other"] != true {
		t.Fatalf("initialized values=%#v", got)
	}
	existing := InitializeValues(spec, map[string]any{"rank": []any{"b", "a"}})
	if !reflect.DeepEqual(existing["rank"], []any{"b", "a"}) {
		t.Fatalf("existing ranking replaced: %#v", existing)
	}
}

func TestFilterCompatibleValues(t *testing.T) {
	old := Spec{Version: Version, Title: "Old", Components: []Component{
		{ID: "text", Kind: KindInputText, Label: "Text"},
		{ID: "bool", Kind: KindCheckbox, Label: "Bool"},
		{ID: "pick", Kind: KindSelect, Label: "Pick", Options: []Option{{Value: "a", Label: "A"}}},
		{ID: "gone", Kind: KindNumber, Label: "Gone"},
	}}
	newSpec := Spec{Version: Version, Title: "New", Components: []Component{
		{ID: "text", Kind: KindTextarea, Label: "Text"},                                            // compatible string shape
		{ID: "bool", Kind: KindInputText, Label: "Bool"},                                           // incompatible shape
		{ID: "pick", Kind: KindSelect, Label: "Pick", Options: []Option{{Value: "b", Label: "B"}}}, // old value invalid
	}}
	values := map[string]any{"text": "kept", "bool": true, "pick": "a", "gone": 2.0}
	got := FilterCompatibleValues(old, newSpec, values)
	if len(got) != 1 || got["text"] != "kept" {
		t.Fatalf("got %#v", got)
	}
}

func TestNestedSectionIDs(t *testing.T) {
	spec := Spec{Version: Version, Title: "Nested", Components: []Component{{
		Kind: KindSection, Label: "Section", Components: []Component{{ID: "inside", Kind: KindCheckbox, Label: "Inside"}},
	}}}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if err := ValidateValues(spec, map[string]any{"inside": true}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectComponentsOutsideSections(t *testing.T) {
	spec := Spec{Version: Version, Title: "Hidden input", Components: []Component{{
		Kind: KindText, Content: "Visible", Components: []Component{{
			ID: "hidden", Kind: KindCheckbox, Label: "Hidden",
		}},
	}}}
	err := ValidateSpec(&spec)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Path != "components[0].components" {
		t.Fatalf("ValidateSpec() error = %#v, want components field error", err)
	}
	if err := ValidateValues(spec, map[string]any{"hidden": true}); err == nil {
		t.Fatal("hidden interactive child was accepted as state")
	}
}

func TestNestedComponentLimitsCannotBeBypassed(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		children := make([]Component, MaxComponents)
		for i := range children {
			children[i] = Component{Kind: KindDivider}
		}
		spec := Spec{Version: Version, Title: "Too many", Components: []Component{{
			Kind: KindSection, Components: children,
		}}}
		err := ValidateSpec(&spec)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Path != "components" {
			t.Fatalf("ValidateSpec() error = %#v, want component count error", err)
		}
	})

	t.Run("depth", func(t *testing.T) {
		tooDeep := Component{Kind: KindText, Content: "Too deep"}
		for i := 0; i < MaxNestingDepth; i++ {
			tooDeep = Component{Kind: KindSection, Components: []Component{tooDeep}}
		}
		spec := Spec{Version: Version, Title: "Too deep", Components: []Component{tooDeep}}
		err := ValidateSpec(&spec)
		var validation *ValidationError
		if !errors.As(err, &validation) || !strings.HasSuffix(validation.Path, ".components") {
			t.Fatalf("ValidateSpec() error = %#v, want nesting depth error", err)
		}
	})
}

func TestValidNestedSectionsAtMaximumDepth(t *testing.T) {
	component := Component{ID: "inside", Kind: KindCheckbox, Label: "Inside"}
	for i := 1; i < MaxNestingDepth; i++ {
		component = Component{Kind: KindSection, Label: "Section", Components: []Component{component}}
	}
	spec := Spec{Version: Version, Title: "Nested", Components: []Component{component}}
	if err := ValidateSpec(&spec); err != nil {
		t.Fatalf("ValidateSpec() error = %v", err)
	}
	if err := ValidateValues(spec, map[string]any{"inside": true}); err != nil {
		t.Fatalf("ValidateValues() error = %v", err)
	}
}

func TestRejectFieldsFromOtherComponentKinds(t *testing.T) {
	tests := []struct {
		name      string
		component Component
		path      string
	}{
		{name: "options", component: Component{Kind: KindText, Content: "Text", Options: []Option{{Value: "a", Label: "A"}}}, path: "components[0].options"},
		{name: "items", component: Component{Kind: KindDivider, Items: []Item{}}, path: "components[0].items"},
		{name: "numeric bound", component: Component{Kind: KindCheckbox, ID: "check", Label: "Check", Min: ptr(0.0)}, path: "components[0].min"},
		{name: "comparison", component: Component{Kind: KindText, Content: "Text", Rows: []ComparisonRow{}}, path: "components[0].rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Version: Version, Title: "Invalid union", Components: []Component{tt.component}}
			err := ValidateSpec(&spec)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Path != tt.path {
				t.Fatalf("ValidateSpec() error = %#v, want path %q", err, tt.path)
			}
		})
	}
}

func TestColorSchemeValidationAndDefault(t *testing.T) {
	if got := (Spec{}).EffectiveColorScheme(); got != "system" {
		t.Fatalf("EffectiveColorScheme() = %q, want system", got)
	}
	if got := (Spec{Presentation: Presentation{ColorScheme: "dark"}}).EffectiveColorScheme(); got != "dark" {
		t.Fatalf("EffectiveColorScheme() = %q, want dark", got)
	}
	spec := validSpec()
	spec.Presentation.ColorScheme = "sepia"
	if err := ValidateSpec(&spec); err == nil || !strings.Contains(err.Error(), "presentation.color_scheme") {
		t.Fatalf("ValidateSpec() error = %v, want color scheme error", err)
	}
}
