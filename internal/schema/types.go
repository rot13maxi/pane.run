// Package schema defines the public Agent Surface document and state formats.
package schema

import "time"

const Version = "1"

type ComponentKind string

const (
	KindHeading     ComponentKind = "heading"
	KindText        ComponentKind = "text"
	KindImage       ComponentKind = "image"
	KindLink        ComponentKind = "link"
	KindDivider     ComponentKind = "divider"
	KindSection     ComponentKind = "section"
	KindInputText   ComponentKind = "input_text"
	KindTextarea    ComponentKind = "textarea"
	KindNumber      ComponentKind = "number"
	KindCheckbox    ComponentKind = "checkbox"
	KindToggle      ComponentKind = "toggle"
	KindSelect      ComponentKind = "select"
	KindMultiSelect ComponentKind = "multi_select"
	KindChecklist   ComponentKind = "checklist"
	KindGallery     ComponentKind = "gallery"
	KindRanking     ComponentKind = "ranking"
	KindApproval    ComponentKind = "approval"
	KindComparison  ComponentKind = "comparison"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSubmitted Status = "submitted"
	StatusClosed    Status = "closed"
)

type Spec struct {
	Version      string       `json:"version"`
	Title        string       `json:"title"`
	Description  string       `json:"description,omitempty"`
	TTLSeconds   int          `json:"ttl_seconds,omitempty"`
	Presentation Presentation `json:"presentation,omitempty"`
	Components   []Component  `json:"components"`
	Actions      Actions      `json:"actions,omitempty"`
}

type Presentation struct {
	Tone    string `json:"tone,omitempty"`
	Density string `json:"density,omitempty"`
}

type Actions struct {
	Submit *Action `json:"submit,omitempty"`
	Reset  *Action `json:"reset,omitempty"`
}

type Action struct {
	Label string `json:"label,omitempty"`
}

// Component is a tagged union. Validation restricts fields according to Kind.
type Component struct {
	ID            string          `json:"id,omitempty"`
	Kind          ComponentKind   `json:"kind"`
	Label         string          `json:"label,omitempty"`
	Content       string          `json:"content,omitempty"`
	Help          string          `json:"help,omitempty"`
	URL           string          `json:"url,omitempty"`
	Asset         string          `json:"asset,omitempty"`
	Alt           string          `json:"alt,omitempty"`
	Level         int             `json:"level,omitempty"`
	Required      bool            `json:"required,omitempty"`
	Placeholder   string          `json:"placeholder,omitempty"`
	Min           *float64        `json:"min,omitempty"`
	Max           *float64        `json:"max,omitempty"`
	Step          *float64        `json:"step,omitempty"`
	MinLength     *int            `json:"min_length,omitempty"`
	MaxLength     *int            `json:"max_length,omitempty"`
	MinSelections *int            `json:"min_selections,omitempty"`
	MaxSelections *int            `json:"max_selections,omitempty"`
	Options       []Option        `json:"options,omitempty"`
	Items         []Item          `json:"items,omitempty"`
	Components    []Component     `json:"components,omitempty"`
	Columns       []Column        `json:"columns,omitempty"`
	Rows          []ComparisonRow `json:"rows,omitempty"`
}

type Option struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type Item struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
}

type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type ComparisonRow struct {
	Label  string            `json:"label"`
	Values map[string]string `json:"values"`
}

type Result struct {
	Status      Status         `json:"status"`
	Revision    uint64         `json:"revision"`
	Values      map[string]any `json:"values"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	SubmittedAt *time.Time     `json:"submitted_at,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

type CreateResponse struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	ManagementToken string    `json:"management_token"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type SurfaceResponse struct {
	ID     string `json:"id"`
	Spec   Spec   `json:"spec"`
	Result Result `json:"result"`
}

type StateUpdate struct {
	Revision uint64         `json:"revision"`
	Values   map[string]any `json:"values"`
}
