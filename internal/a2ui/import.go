// Package a2ui translates a deliberately small, non-executable subset of the
// A2UI v0.9 protocol into Pane's canonical V1 document.
package a2ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/agent-surface/agent-surface/internal/schema"
)

const (
	Version       = "v0.9"
	BasicCatalog  = "https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"
	MaxMessages   = 256
	MaxComponents = schema.MaxComponents
)

type Options struct {
	Title       string
	Description string
	TTLSeconds  int
}

type Result struct {
	Spec      schema.Spec
	Values    map[string]any
	Warnings  []string
	SurfaceID string
}

type envelope struct {
	Version          string            `json:"version"`
	CreateSurface    *createSurface    `json:"createSurface,omitempty"`
	UpdateComponents *updateComponents `json:"updateComponents,omitempty"`
	UpdateDataModel  *updateDataModel  `json:"updateDataModel,omitempty"`
	DeleteSurface    *surfaceRef       `json:"deleteSurface,omitempty"`
}

type createSurface struct {
	SurfaceID     string         `json:"surfaceId"`
	CatalogID     string         `json:"catalogId"`
	Theme         map[string]any `json:"theme,omitempty"`
	SendDataModel bool           `json:"sendDataModel,omitempty"`
}

type updateComponents struct {
	SurfaceID  string           `json:"surfaceId"`
	Components []map[string]any `json:"components"`
}

type updateDataModel struct {
	SurfaceID string          `json:"surfaceId"`
	Path      string          `json:"path,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

type surfaceRef struct {
	SurfaceID string `json:"surfaceId"`
}

// Import accepts either a JSON array of envelopes or a sequence of JSON values
// (including JSONL). It consumes the batch in order and imports exactly one live
// A2UI surface.
func Import(data []byte, opts Options) (Result, error) {
	messages, err := decode(data)
	if err != nil {
		return Result{}, err
	}
	var created *createSurface
	components := map[string]map[string]any{}
	order := []string{}
	model := any(map[string]any{})
	for i, message := range messages {
		if message.Version != Version && message.Version != "v0.9.1" {
			return Result{}, fmt.Errorf("message %d: version must be %q or %q", i+1, Version, "v0.9.1")
		}
		count := 0
		if message.CreateSurface != nil {
			count++
		}
		if message.UpdateComponents != nil {
			count++
		}
		if message.UpdateDataModel != nil {
			count++
		}
		if message.DeleteSurface != nil {
			count++
		}
		if count != 1 {
			return Result{}, fmt.Errorf("message %d: must contain exactly one A2UI operation", i+1)
		}
		switch {
		case message.CreateSurface != nil:
			if created != nil {
				return Result{}, fmt.Errorf("message %d: duplicate createSurface", i+1)
			}
			if message.CreateSurface.SurfaceID == "" {
				return Result{}, fmt.Errorf("message %d: surfaceId is required", i+1)
			}
			if message.CreateSurface.CatalogID != BasicCatalog {
				return Result{}, fmt.Errorf("message %d: unsupported catalogId %q", i+1, message.CreateSurface.CatalogID)
			}
			created = message.CreateSurface
		case message.UpdateComponents != nil:
			if err := sameSurface(i, created, message.UpdateComponents.SurfaceID); err != nil {
				return Result{}, err
			}
			for _, component := range message.UpdateComponents.Components {
				id, _ := component["id"].(string)
				if id == "" {
					return Result{}, fmt.Errorf("message %d: component id is required", i+1)
				}
				if _, ok := components[id]; !ok {
					order = append(order, id)
				}
				components[id] = component
				if len(components) > MaxComponents {
					return Result{}, fmt.Errorf("A2UI surface exceeds %d components", MaxComponents)
				}
			}
		case message.UpdateDataModel != nil:
			if err := sameSurface(i, created, message.UpdateDataModel.SurfaceID); err != nil {
				return Result{}, err
			}
			var value any
			present := len(message.UpdateDataModel.Value) != 0
			if present && json.Unmarshal(message.UpdateDataModel.Value, &value) != nil {
				return Result{}, fmt.Errorf("message %d: invalid data model value", i+1)
			}
			model, err = applyPointer(model, message.UpdateDataModel.Path, value, present)
			if err != nil {
				return Result{}, fmt.Errorf("message %d: %w", i+1, err)
			}
		case message.DeleteSurface != nil:
			if err := sameSurface(i, created, message.DeleteSurface.SurfaceID); err != nil {
				return Result{}, err
			}
			return Result{}, fmt.Errorf("message %d: cannot import a deleted surface", i+1)
		}
	}
	if created == nil {
		return Result{}, errors.New("createSurface is required")
	}
	if _, ok := components["root"]; !ok {
		return Result{}, errors.New("component with id root is required")
	}
	c := compiler{all: components, ids: map[string]string{}, used: map[string]bool{}, model: model}
	translated, err := c.walk("root", map[string]bool{})
	if err != nil {
		return Result{}, err
	}
	if len(translated) == 0 {
		return Result{}, errors.New("root contains no supported visible components")
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "Imported A2UI surface"
	}
	spec := schema.Spec{Version: schema.Version, Title: title, Description: opts.Description, TTLSeconds: opts.TTLSeconds, Components: translated, Actions: c.actions}
	if err := schema.ValidateSpec(&spec); err != nil {
		return Result{}, fmt.Errorf("translated surface: %w", err)
	}
	if err := schema.ValidateValues(spec, c.values); err != nil {
		return Result{}, fmt.Errorf("translated initial state: %w", err)
	}
	if len(created.Theme) != 0 {
		c.warn("A2UI theme was ignored; Pane owns presentation")
	}
	if created.SendDataModel {
		c.warn("sendDataModel was ignored; Surface state is read through its public or management API")
	}
	return Result{Spec: spec, Values: c.values, Warnings: c.warnings, SurfaceID: created.SurfaceID}, nil
}

func decode(data []byte) ([]envelope, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("A2UI batch is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid A2UI JSON: %w", err)
	}
	var messages []envelope
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &messages); err != nil {
			return nil, fmt.Errorf("invalid A2UI message array: %w", err)
		}
	} else {
		var first envelope
		if err := json.Unmarshal(raw, &first); err != nil {
			return nil, fmt.Errorf("invalid A2UI message: %w", err)
		}
		messages = append(messages, first)
		for {
			var next envelope
			err := dec.Decode(&next)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("invalid A2UI JSON sequence: %w", err)
			}
			messages = append(messages, next)
		}
	}
	if len(messages) == 0 {
		return nil, errors.New("A2UI batch contains no messages")
	}
	if len(messages) > MaxMessages {
		return nil, fmt.Errorf("A2UI batch exceeds %d messages", MaxMessages)
	}
	return messages, nil
}

func sameSurface(index int, created *createSurface, id string) error {
	if created == nil {
		return fmt.Errorf("message %d: createSurface must come first", index+1)
	}
	if id != created.SurfaceID {
		return fmt.Errorf("message %d: surfaceId %q does not match %q", index+1, id, created.SurfaceID)
	}
	return nil
}

type compiler struct {
	all      map[string]map[string]any
	ids      map[string]string
	used     map[string]bool
	model    any
	values   map[string]any
	actions  schema.Actions
	warnings []string
}

func (c *compiler) walk(id string, stack map[string]bool) ([]schema.Component, error) {
	if stack[id] {
		return nil, fmt.Errorf("component graph contains a cycle at %q", id)
	}
	raw, ok := c.all[id]
	if !ok {
		return nil, fmt.Errorf("component %q is referenced but undefined", id)
	}
	kind, _ := raw["component"].(string)
	if kind == "" {
		return nil, fmt.Errorf("component %q omits component type", id)
	}
	stack[id] = true
	defer delete(stack, id)
	switch kind {
	case "Column", "Row", "Card":
		children, err := childIDs(raw)
		if err != nil {
			return nil, fmt.Errorf("component %q: %w", id, err)
		}
		if kind == "Row" {
			c.warn(fmt.Sprintf("component %q: Row layout was flattened", id))
		}
		var out []schema.Component
		for _, child := range children {
			v, err := c.walk(child, stack)
			if err != nil {
				return nil, err
			}
			out = append(out, v...)
		}
		return out, nil
	case "Text":
		text, err := c.dynamicString(raw["text"])
		if err != nil {
			return nil, c.propErr(id, "text", err)
		}
		variant, _ := raw["variant"].(string)
		if variant == "h1" || variant == "h2" || variant == "h3" {
			level, _ := strconv.Atoi(strings.TrimPrefix(variant, "h"))
			return []schema.Component{{Kind: schema.KindHeading, Content: text, Level: level}}, nil
		}
		return []schema.Component{{Kind: schema.KindText, Content: text}}, nil
	case "Divider":
		return []schema.Component{{Kind: schema.KindDivider}}, nil
	case "Image":
		url, err := c.dynamicString(raw["url"])
		if err != nil {
			return nil, c.propErr(id, "url", err)
		}
		alt, _ := c.dynamicString(raw["altText"])
		return []schema.Component{{Kind: schema.KindImage, URL: url, Alt: alt}}, nil
	case "TextField":
		return c.textField(id, raw)
	case "CheckBox":
		label, err := c.dynamicString(raw["label"])
		if err != nil {
			return nil, c.propErr(id, "label", err)
		}
		sid := c.surfaceID(id)
		component := schema.Component{ID: sid, Kind: schema.KindCheckbox, Label: label}
		if value, ok, err := c.boundValue(raw["value"]); err != nil {
			return nil, c.propErr(id, "value", err)
		} else if ok {
			c.ensureValues()
			c.values[sid] = value
		}
		return []schema.Component{component}, nil
	case "ChoicePicker":
		return c.choicePicker(id, raw)
	case "Button":
		return nil, c.button(id, raw)
	default:
		return nil, fmt.Errorf("component %q: unsupported Basic Catalog component %q", id, kind)
	}
}

func (c *compiler) textField(id string, raw map[string]any) ([]schema.Component, error) {
	label, err := c.dynamicString(raw["label"])
	if err != nil {
		return nil, c.propErr(id, "label", err)
	}
	variant, _ := raw["variant"].(string)
	kind := schema.KindInputText
	if variant == "longText" {
		kind = schema.KindTextarea
	} else if variant == "number" {
		kind = schema.KindNumber
	} else if variant != "" && variant != "shortText" {
		return nil, fmt.Errorf("component %q: unsupported TextField variant %q", id, variant)
	}
	sid := c.surfaceID(id)
	component := schema.Component{ID: sid, Kind: kind, Label: label}
	if placeholder, e := c.dynamicString(raw["placeholder"]); e == nil {
		component.Placeholder = placeholder
	} else if raw["placeholder"] != nil {
		return nil, c.propErr(id, "placeholder", e)
	}
	if value, ok, e := c.boundValue(raw["value"]); e != nil {
		return nil, c.propErr(id, "value", e)
	} else if ok {
		c.ensureValues()
		c.values[sid] = value
	}
	if checks, ok := raw["checks"].([]any); ok && len(checks) > 0 {
		c.warn(fmt.Sprintf("component %q: A2UI checks were ignored", id))
	}
	return []schema.Component{component}, nil
}

func (c *compiler) choicePicker(id string, raw map[string]any) ([]schema.Component, error) {
	label, _ := c.dynamicString(raw["label"])
	if label == "" {
		label = "Choose"
	}
	items, ok := raw["options"].([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("component %q: options must be a non-empty array", id)
	}
	options := make([]schema.Option, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("component %q: option %d must be an object", id, i)
		}
		l, _ := c.dynamicString(m["label"])
		v, _ := c.dynamicString(m["value"])
		if l == "" || v == "" {
			return nil, fmt.Errorf("component %q: option %d needs literal label and value", id, i)
		}
		options = append(options, schema.Option{Label: l, Value: v})
	}
	variant, _ := raw["variant"].(string)
	kind := schema.KindMultiSelect
	if variant == "mutuallyExclusive" {
		kind = schema.KindSelect
	} else if variant != "multipleSelection" && variant != "" {
		return nil, fmt.Errorf("component %q: unsupported ChoicePicker variant %q", id, variant)
	}
	sid := c.surfaceID(id)
	component := schema.Component{ID: sid, Kind: kind, Label: label, Options: options}
	if value, found, err := c.boundValue(raw["value"]); err != nil {
		return nil, c.propErr(id, "value", err)
	} else if found {
		if kind == schema.KindSelect {
			if list, ok := value.([]any); ok && len(list) == 1 {
				value = list[0]
			}
		}
		c.ensureValues()
		c.values[sid] = value
	}
	return []schema.Component{component}, nil
}

func (c *compiler) button(id string, raw map[string]any) error {
	action, ok := raw["action"].(map[string]any)
	if !ok {
		return fmt.Errorf("component %q: Button requires an event action", id)
	}
	event, ok := action["event"].(map[string]any)
	if !ok {
		return fmt.Errorf("component %q: Button functions are not supported", id)
	}
	name, _ := event["name"].(string)
	if name != "submit" && name != "reset" {
		return fmt.Errorf("component %q: unsupported Button event %q", id, name)
	}
	label := strings.Title(name)
	if child, ok := raw["child"].(string); ok {
		if target := c.all[child]; target != nil {
			if text, e := c.dynamicString(target["text"]); e == nil && text != "" {
				label = text
			}
		}
	}
	if name == "submit" {
		c.actions.Submit = &schema.Action{Label: label}
	} else {
		c.actions.Reset = &schema.Action{Label: label}
	}
	return nil
}

func childIDs(raw map[string]any) ([]string, error) {
	if child, ok := raw["child"].(string); ok {
		return []string{child}, nil
	}
	items, ok := raw["children"].([]any)
	if !ok {
		return nil, errors.New("static child or children is required")
	}
	out := make([]string, len(items))
	for i, item := range items {
		var ok bool
		out[i], ok = item.(string)
		if !ok {
			return nil, errors.New("dynamic child templates are not supported")
		}
	}
	return out, nil
}

func (c *compiler) dynamicString(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	if literal, ok := value.(string); ok {
		return literal, nil
	}
	resolved, found, err := c.boundValue(value)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("binding path is not present in the data model")
	}
	literal, ok := resolved.(string)
	if !ok {
		return "", errors.New("must resolve to a string")
	}
	return literal, nil
}

func (c *compiler) boundValue(value any) (any, bool, error) {
	m, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil, false, nil
		}
		return value, true, nil
	}
	if _, ok := m["call"]; ok {
		return nil, false, errors.New("function calls are not supported")
	}
	path, ok := m["path"].(string)
	if !ok {
		return nil, false, errors.New("dynamic value must contain path")
	}
	v, found, err := lookupPointer(c.model, path)
	return v, found, err
}

var invalidID = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (c *compiler) surfaceID(id string) string {
	if existing := c.ids[id]; existing != "" {
		return existing
	}
	v := invalidID.ReplaceAllString(id, "_")
	if v == "" || !((v[0] >= 'A' && v[0] <= 'Z') || (v[0] >= 'a' && v[0] <= 'z')) {
		v = "a2ui_" + v
	}
	if len(v) > 64 {
		v = v[:64]
	}
	base := v
	for n := 2; c.used[v]; n++ {
		suffix := "_" + strconv.Itoa(n)
		v = base
		if len(v)+len(suffix) > 64 {
			v = v[:64-len(suffix)]
		}
		v += suffix
	}
	c.ids[id] = v
	c.used[v] = true
	if v != id {
		c.warn(fmt.Sprintf("component id %q was normalized to %q", id, v))
	}
	return v
}
func (c *compiler) ensureValues() {
	if c.values == nil {
		c.values = map[string]any{}
	}
}
func (c *compiler) warn(v string) {
	for _, old := range c.warnings {
		if old == v {
			return
		}
	}
	c.warnings = append(c.warnings, v)
}
func (c *compiler) propErr(id, prop string, err error) error {
	return fmt.Errorf("component %q property %s: %w", id, prop, err)
}

func lookupPointer(root any, pointer string) (any, bool, error) {
	if pointer == "" || pointer == "/" {
		return root, true, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false, errors.New("binding path must be a JSON Pointer")
	}
	cur := root
	for _, raw := range strings.Split(pointer[1:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch node := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = node[token]
			if !ok {
				return nil, false, nil
			}
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false, nil
			}
			cur = node[i]
		default:
			return nil, false, nil
		}
	}
	return cur, true, nil
}

func applyPointer(root any, pointer string, value any, present bool) (any, error) {
	if pointer == "" || pointer == "/" {
		if !present {
			return map[string]any{}, nil
		}
		return value, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return root, errors.New("data model path must be a JSON Pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	node, ok := root.(map[string]any)
	if !ok {
		return root, errors.New("partial data model update requires an object root")
	}
	cur := node
	for _, raw := range parts[:len(parts)-1] {
		key := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		next, ok := cur[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[key] = next
		}
		cur = next
	}
	key := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "~1", "/"), "~0", "~")
	if present {
		cur[key] = value
	} else {
		delete(cur, key)
	}
	return root, nil
}
