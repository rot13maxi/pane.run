package schema

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	DefaultTTLSeconds = 24 * 60 * 60
	MinTTLSeconds     = 60
	MaxTTLSeconds     = 7 * 24 * 60 * 60
	MaxComponents     = 200
	MaxNestingDepth   = 4
	MaxStateKeys      = 200
	MaxTextLength     = 16 * 1024
)

var idPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// ValidationError identifies a bad field in a declarative specification or state.
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string { return e.Path + ": " + e.Message }

func invalid(path, format string, args ...any) error {
	return &ValidationError{Path: path, Message: fmt.Sprintf(format, args...)}
}

// EffectiveTTLSeconds applies the V1 default when ttl_seconds is omitted.
func (s Spec) EffectiveTTLSeconds() int {
	if s.TTLSeconds == 0 {
		return DefaultTTLSeconds
	}
	return s.TTLSeconds
}

func ValidateSpec(s *Spec) error {
	if s == nil {
		return invalid("spec", "is required")
	}
	if s.Version != Version {
		return invalid("version", "must be %q", Version)
	}
	if err := boundedRequired("title", s.Title, 200); err != nil {
		return err
	}
	if utf8.RuneCountInString(s.Description) > 2000 {
		return invalid("description", "must be at most 2000 characters")
	}
	if s.TTLSeconds != 0 && (s.TTLSeconds < MinTTLSeconds || s.TTLSeconds > MaxTTLSeconds) {
		return invalid("ttl_seconds", "must be between %d and %d", MinTTLSeconds, MaxTTLSeconds)
	}
	if s.Presentation.Tone != "" && !oneOf(s.Presentation.Tone, "neutral", "warm", "playful", "professional") {
		return invalid("presentation.tone", "must be neutral, warm, playful, or professional")
	}
	if s.Presentation.Density != "" && !oneOf(s.Presentation.Density, "comfortable", "compact") {
		return invalid("presentation.density", "must be comfortable or compact")
	}
	if len(s.Components) == 0 {
		return invalid("components", "must contain at least one component")
	}
	seen := make(map[string]string)
	count := 0
	for i := range s.Components {
		if err := validateComponent(&s.Components[i], fmt.Sprintf("components[%d]", i), 1, seen, &count); err != nil {
			return err
		}
	}
	for name, action := range map[string]*Action{"submit": s.Actions.Submit, "reset": s.Actions.Reset} {
		if action != nil && utf8.RuneCountInString(action.Label) > 80 {
			return invalid("actions."+name+".label", "must be at most 80 characters")
		}
	}
	return nil
}

func validateComponent(c *Component, path string, depth int, seen map[string]string, count *int) error {
	*count++
	if *count > MaxComponents {
		return invalid("components", "must contain at most %d components", MaxComponents)
	}
	if !isKnownKind(c.Kind) {
		return invalid(path+".kind", "unknown component kind %q", c.Kind)
	}
	interactive := IsInteractive(c.Kind)
	if interactive {
		if !idPattern.MatchString(c.ID) {
			return invalid(path+".id", "must match %s", idPattern)
		}
		if previous, exists := seen[c.ID]; exists {
			return invalid(path+".id", "duplicates %s", previous)
		}
		seen[c.ID] = path
		if err := boundedRequired(path+".label", c.Label, 200); err != nil {
			return err
		}
	} else if c.ID != "" {
		if !idPattern.MatchString(c.ID) {
			return invalid(path+".id", "must match %s", idPattern)
		}
		if previous, exists := seen[c.ID]; exists {
			return invalid(path+".id", "duplicates %s", previous)
		}
		seen[c.ID] = path
	}
	for field, value := range map[string]string{"label": c.Label, "content": c.Content, "help": c.Help, "placeholder": c.Placeholder, "alt": c.Alt} {
		if utf8.RuneCountInString(value) > MaxTextLength {
			return invalid(path+"."+field, "must be at most %d characters", MaxTextLength)
		}
	}
	if c.Min != nil && (math.IsNaN(*c.Min) || math.IsInf(*c.Min, 0)) || c.Max != nil && (math.IsNaN(*c.Max) || math.IsInf(*c.Max, 0)) {
		return invalid(path, "numeric bounds must be finite")
	}
	if c.Min != nil && c.Max != nil && *c.Min > *c.Max {
		return invalid(path, "min must not exceed max")
	}
	if c.Step != nil && (*c.Step <= 0 || math.IsNaN(*c.Step) || math.IsInf(*c.Step, 0)) {
		return invalid(path+".step", "must be a positive finite number")
	}
	if err := validateRange(path, c.MinLength, c.MaxLength, "length"); err != nil {
		return err
	}
	if err := validateRange(path, c.MinSelections, c.MaxSelections, "selections"); err != nil {
		return err
	}

	switch c.Kind {
	case KindHeading:
		if c.Content == "" || (c.Level != 0 && (c.Level < 1 || c.Level > 3)) {
			return invalid(path, "heading requires content and level must be 1 through 3")
		}
	case KindText:
		if c.Content == "" {
			return invalid(path+".content", "is required")
		}
	case KindImage:
		if (c.URL == "") == (c.Asset == "") {
			return invalid(path, "image requires exactly one of url or asset")
		}
		if c.URL != "" && !validHTTPURL(c.URL) {
			return invalid(path+".url", "must be an http or https URL")
		}
	case KindLink:
		if c.Label == "" || !validHTTPURL(c.URL) {
			return invalid(path, "link requires a label and an http or https url")
		}
	case KindSection:
		if depth >= MaxNestingDepth {
			return invalid(path+".components", "exceeds maximum nesting depth %d", MaxNestingDepth)
		}
		if len(c.Components) == 0 {
			return invalid(path+".components", "must not be empty")
		}
		for i := range c.Components {
			if err := validateComponent(&c.Components[i], fmt.Sprintf("%s.components[%d]", path, i), depth+1, seen, count); err != nil {
				return err
			}
		}
	case KindSelect, KindMultiSelect:
		if err := validateOptions(path+".options", c.Options); err != nil {
			return err
		}
	case KindChecklist, KindGallery, KindRanking:
		if err := validateItems(path+".items", c.Items, c.Kind == KindGallery); err != nil {
			return err
		}
	case KindApproval:
		// Approval has fixed values: "approved" and "rejected".
	case KindComparison:
		if err := validateComparison(c, path); err != nil {
			return err
		}
	}
	return nil
}

func validateOptions(path string, options []Option) error {
	if len(options) < 1 || len(options) > 100 {
		return invalid(path, "must contain between 1 and 100 options")
	}
	seen := map[string]bool{}
	for i, o := range options {
		p := fmt.Sprintf("%s[%d]", path, i)
		if o.Value == "" || o.Label == "" {
			return invalid(p, "value and label are required")
		}
		if seen[o.Value] {
			return invalid(p+".value", "must be unique")
		}
		seen[o.Value] = true
	}
	return nil
}

func validateItems(path string, items []Item, images bool) error {
	if len(items) < 1 || len(items) > 100 {
		return invalid(path, "must contain between 1 and 100 items")
	}
	seen := map[string]bool{}
	for i, item := range items {
		p := fmt.Sprintf("%s[%d]", path, i)
		if item.Value == "" || item.Label == "" {
			return invalid(p, "value and label are required")
		}
		if seen[item.Value] {
			return invalid(p+".value", "must be unique")
		}
		seen[item.Value] = true
		if images && item.Image == "" {
			return invalid(p+".image", "is required for gallery items")
		}
	}
	return nil
}

func validateComparison(c *Component, path string) error {
	if len(c.Columns) < 2 || len(c.Columns) > 12 || len(c.Rows) < 1 || len(c.Rows) > 100 {
		return invalid(path, "comparison requires 2-12 columns and 1-100 rows")
	}
	keys := map[string]bool{}
	for i, col := range c.Columns {
		if !idPattern.MatchString(col.Key) || col.Label == "" || keys[col.Key] {
			return invalid(fmt.Sprintf("%s.columns[%d]", path, i), "requires a unique key and label")
		}
		keys[col.Key] = true
	}
	for i, row := range c.Rows {
		if row.Label == "" {
			return invalid(fmt.Sprintf("%s.rows[%d].label", path, i), "is required")
		}
		for key := range row.Values {
			if !keys[key] {
				return invalid(fmt.Sprintf("%s.rows[%d].values", path, i), "contains unknown column %q", key)
			}
		}
	}
	return nil
}

func validateRange(path string, min, max *int, name string) error {
	if min != nil && *min < 0 || max != nil && *max < 0 {
		return invalid(path, "%s bounds must be non-negative", name)
	}
	if min != nil && max != nil && *min > *max {
		return invalid(path, "min_%s must not exceed max_%s", name, name)
	}
	return nil
}

func boundedRequired(path, value string, max int) error {
	if strings.TrimSpace(value) == "" {
		return invalid(path, "is required")
	}
	if utf8.RuneCountInString(value) > max {
		return invalid(path, "must be at most %d characters", max)
	}
	return nil
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func isKnownKind(kind ComponentKind) bool {
	switch kind {
	case KindHeading, KindText, KindImage, KindLink, KindDivider, KindSection,
		KindInputText, KindTextarea, KindNumber, KindCheckbox, KindToggle, KindSelect,
		KindMultiSelect, KindChecklist, KindGallery, KindRanking, KindApproval, KindComparison:
		return true
	default:
		return false
	}
}

func IsInteractive(kind ComponentKind) bool {
	switch kind {
	case KindInputText, KindTextarea, KindNumber, KindCheckbox, KindToggle, KindSelect,
		KindMultiSelect, KindChecklist, KindGallery, KindRanking, KindApproval:
		return true
	default:
		return false
	}
}

func interactiveComponents(s Spec) map[string]Component {
	result := make(map[string]Component)
	var walk func([]Component)
	walk = func(components []Component) {
		for _, component := range components {
			if IsInteractive(component.Kind) {
				result[component.ID] = component
			}
			walk(component.Components)
		}
	}
	walk(s.Components)
	return result
}

// ValidateValues validates a partial autosaved state. Required constraints are
// intentionally enforced only by ValidateSubmission.
func ValidateValues(spec Spec, values map[string]any) error {
	return validateValues(spec, values, false)
}

func ValidateSubmission(spec Spec, values map[string]any) error {
	return validateValues(spec, values, true)
}

func validateValues(spec Spec, values map[string]any, submission bool) error {
	if len(values) > MaxStateKeys {
		return invalid("values", "must contain at most %d keys", MaxStateKeys)
	}
	components := interactiveComponents(spec)
	for key, value := range values {
		component, ok := components[key]
		if !ok {
			return invalid("values."+key, "does not name an interactive component")
		}
		if err := validateValue(component, value, "values."+key); err != nil {
			return err
		}
	}
	if submission {
		for id, component := range components {
			value, exists := values[id]
			if component.Required && (!exists || isEmptyValue(value)) {
				return invalid("values."+id, "is required")
			}
		}
	}
	return nil
}

func validateValue(c Component, value any, path string) error {
	switch c.Kind {
	case KindInputText, KindTextarea:
		v, ok := value.(string)
		if !ok {
			return invalid(path, "must be a string")
		}
		length := utf8.RuneCountInString(v)
		if length > MaxTextLength || c.MinLength != nil && length < *c.MinLength || c.MaxLength != nil && length > *c.MaxLength {
			return invalid(path, "does not satisfy length constraints")
		}
	case KindNumber:
		v, ok := asFloat(value)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			return invalid(path, "must be a finite number")
		}
		if c.Min != nil && v < *c.Min || c.Max != nil && v > *c.Max {
			return invalid(path, "does not satisfy numeric bounds")
		}
	case KindCheckbox, KindToggle:
		if _, ok := value.(bool); !ok {
			return invalid(path, "must be a boolean")
		}
	case KindSelect:
		v, ok := value.(string)
		if !ok || !containsOption(c.Options, v) {
			return invalid(path, "must be an allowed option")
		}
	case KindApproval:
		v, ok := value.(string)
		if !ok || !oneOf(v, "approved", "rejected") {
			return invalid(path, "must be approved or rejected")
		}
	case KindMultiSelect:
		return validateStringList(value, c.Options, nil, c.MinSelections, c.MaxSelections, false, path)
	case KindChecklist, KindGallery:
		return validateStringList(value, nil, c.Items, c.MinSelections, c.MaxSelections, false, path)
	case KindRanking:
		return validateStringList(value, nil, c.Items, nil, nil, true, path)
	}
	return nil
}

func validateStringList(value any, options []Option, items []Item, min, max *int, complete bool, path string) error {
	values, ok := stringSlice(value)
	if !ok {
		return invalid(path, "must be an array of strings")
	}
	if min != nil && len(values) < *min || max != nil && len(values) > *max {
		return invalid(path, "does not satisfy selection bounds")
	}
	if complete && len(values) != len(items) {
		return invalid(path, "must contain every item exactly once")
	}
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] || options != nil && !containsOption(options, v) || items != nil && !containsItem(items, v) {
			return invalid(path, "contains a duplicate or unknown value %q", v)
		}
		seen[v] = true
	}
	return nil
}

func stringSlice(value any) ([]string, bool) {
	switch list := value.(type) {
	case []string:
		return list, true
	case []any:
		result := make([]string, len(list))
		for i, entry := range list {
			var ok bool
			result[i], ok = entry.(string)
			if !ok {
				return nil, false
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func containsOption(options []Option, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func containsItem(items []Item, value string) bool {
	for _, item := range items {
		if item.Value == value {
			return true
		}
	}
	return false
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	if list, ok := stringSlice(value); ok {
		return len(list) == 0
	}
	return false
}

// FilterCompatibleValues retains only values whose component ID and JSON value
// shape survive a definition replacement and which remain valid in the new spec.
func FilterCompatibleValues(oldSpec, newSpec Spec, values map[string]any) map[string]any {
	oldComponents := interactiveComponents(oldSpec)
	newComponents := interactiveComponents(newSpec)
	filtered := make(map[string]any)
	for id, value := range values {
		oldComponent, oldOK := oldComponents[id]
		newComponent, newOK := newComponents[id]
		if !oldOK || !newOK || valueShape(oldComponent.Kind) != valueShape(newComponent.Kind) {
			continue
		}
		if err := validateValue(newComponent, value, "values."+id); err == nil {
			filtered[id] = value
		}
	}
	return filtered
}

func valueShape(kind ComponentKind) string {
	switch kind {
	case KindInputText, KindTextarea, KindSelect, KindApproval:
		return "string"
	case KindNumber:
		return "number"
	case KindCheckbox, KindToggle:
		return "boolean"
	case KindMultiSelect, KindChecklist, KindGallery, KindRanking:
		return "string_array"
	default:
		return ""
	}
}

// IsValidationError allows HTTP callers to distinguish bad input from internal errors.
func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
