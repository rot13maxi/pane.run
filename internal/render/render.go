// Package render produces the public, responsive surface page.
package render

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/agent-surface/agent-surface/internal/schema"
)

// Page contains the trusted routing information and untrusted surface document
// needed to render a public surface. URLs should be same-origin paths supplied by
// the server, not values from the surface specification.
type Page struct {
	Spec      schema.Spec
	Result    schema.Result
	PublicID  string
	StateURL  string
	SubmitURL string
	ResetURL  string
	ReadOnly  bool
}

type viewData struct {
	Page
	Initial template.JS
	Classes string
}

var pageTemplate = template.Must(template.New("surface").Funcs(template.FuncMap{
	"value": func(values map[string]any, id string) any { return values[id] },
	"str": func(v any) string {
		switch x := v.(type) {
		case string:
			return x
		case json.Number:
			return x.String()
		case float64:
			return fmt.Sprintf("%g", x)
		default:
			return ""
		}
	},
	"checked": func(v any) bool { b, _ := v.(bool); return b },
	"contains": func(v any, wanted string) bool {
		switch list := v.(type) {
		case []string:
			for _, item := range list {
				if item == wanted {
					return true
				}
			}
		case []any:
			for _, item := range list {
				if s, ok := item.(string); ok && s == wanted {
					return true
				}
			}
		}
		return false
	},
	"ranked": rankedItems,
	"dict":   func(values ...any) []any { return values },
}).Parse(pageHTML))

// Render writes a complete HTML document. html/template escapes all specification
// content; the only script value is JSON produced by encoding/json.
func Render(w io.Writer, page Page) error {
	if page.Result.Values == nil {
		page.Result.Values = map[string]any{}
	}
	initial, err := json.Marshal(struct {
		Revision uint64         `json:"revision"`
		Values   map[string]any `json:"values"`
		Status   schema.Status  `json:"status"`
		State    string         `json:"state_url"`
		Submit   string         `json:"submit_url"`
		Reset    string         `json:"reset_url"`
		ReadOnly bool           `json:"read_only"`
	}{page.Result.Revision, page.Result.Values, page.Result.Status, page.StateURL, page.SubmitURL, page.ResetURL, page.ReadOnly || page.Result.Status == schema.StatusClosed})
	if err != nil {
		return fmt.Errorf("encode initial state: %w", err)
	}
	classes := "tone-" + token(page.Spec.Presentation.Tone) + " density-" + token(page.Spec.Presentation.Density) + " theme-" + token(page.Spec.EffectiveColorScheme())
	return pageTemplate.Execute(w, viewData{Page: page, Initial: template.JS(initial), Classes: classes})
}

func token(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func rankedItems(items []schema.Item, v any) []schema.Item {
	order := map[string]int{}
	switch list := v.(type) {
	case []string:
		for i, x := range list {
			order[x] = i + 1
		}
	case []any:
		for i, x := range list {
			if s, ok := x.(string); ok {
				order[s] = i + 1
			}
		}
	}
	if len(order) == 0 {
		return items
	}
	out := make([]schema.Item, 0, len(items))
	used := map[string]bool{}
	for i := 1; i <= len(order); i++ {
		for _, item := range items {
			if order[item.Value] == i {
				out = append(out, item)
				used[item.Value] = true
			}
		}
	}
	for _, item := range items {
		if !used[item.Value] {
			out = append(out, item)
		}
	}
	return out
}
