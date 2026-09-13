package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
)

type recipeOptions struct {
	title, description, id, help, notes, tone, density, theme, submit string
	ttl                                                               time.Duration
	required                                                          bool
}

func recipe(name string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	var o recipeOptions
	fs.StringVar(&o.title, "title", defaultRecipeTitle(name), "page title")
	fs.StringVar(&o.description, "description", "", "short text below the title")
	fs.StringVar(&o.id, "id", recipeID(name), "result key")
	fs.StringVar(&o.help, "hint", "", "help text for the interaction")
	fs.StringVar(&o.notes, "notes", "", "also add a notes field with this label")
	fs.StringVar(&o.tone, "tone", "", "neutral, warm, playful, or professional")
	fs.StringVar(&o.density, "density", "", "comfortable or compact")
	fs.StringVar(&o.theme, "theme", "", "light, dark, or system")
	fs.StringVar(&o.submit, "submit", "Done", "submit button label; empty disables submit")
	fs.DurationVar(&o.ttl, "ttl", 0, "lifetime, for example 30m or 48h (max 168h)")
	server := fs.String("server", env("PANE_SERVER", defaultServer), "service URL")
	var multi bool
	if name == "gallery" || name == "pick" {
		fs.BoolVar(&multi, "multi", false, "allow multiple selections")
	}
	o.required = true
	fs.BoolFunc("optional", "allow submit without completing the primary interaction", func(string) error { o.required = false; return nil })
	var body, bodyFile string
	if name == "approve" {
		fs.StringVar(&body, "body", "", "approval text")
		fs.StringVar(&bodyFile, "body-file", "", "read approval text from a file")
	}
	fs.Usage = func() { recipeUsage(fs, name) }
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	values := fs.Args()
	spec := schema.Spec{Version: schema.Version, Title: o.title, Description: o.description,
		Presentation: schema.Presentation{Tone: o.tone, Density: o.density, ColorScheme: o.theme}}
	if o.ttl != 0 {
		if o.ttl%time.Second != 0 {
			return errors.New("--ttl must be a whole number of seconds")
		}
		spec.TTLSeconds = int(o.ttl / time.Second)
	}
	if o.submit != "" {
		spec.Actions.Submit = &schema.Action{Label: o.submit}
	}
	var assets []string
	switch name {
	case "gallery":
		if len(values) == 0 {
			return errors.New("gallery requires at least one image path")
		}
		items := make([]schema.Item, len(values))
		labels := make([]string, len(values))
		paths := make([]string, len(values))
		for i, arg := range values {
			labels[i], paths[i] = galleryArg(arg)
		}
		itemValues := uniqueValues(labels)
		for i := range values {
			assetName := fmt.Sprintf("image_%d", i+1)
			items[i] = schema.Item{Value: itemValues[i], Label: labels[i], Image: "asset:" + assetName}
			assets = append(assets, assetName+"="+paths[i])
		}
		max := intPtr(1)
		if multi {
			max = nil
		}
		spec.Components = append(spec.Components, schema.Component{Kind: schema.KindGallery, ID: o.id, Label: o.title, Help: o.help, Required: o.required, MaxSelections: max, Items: items})
	case "pick":
		if len(values) == 0 {
			return errors.New("pick requires at least one choice")
		}
		options := optionsFrom(values)
		kind := schema.KindSelect
		if multi {
			kind = schema.KindMultiSelect
		}
		spec.Components = append(spec.Components, schema.Component{Kind: kind, ID: o.id, Label: o.title, Help: o.help, Required: o.required, Options: options})
	case "rank":
		if len(values) < 2 {
			return errors.New("rank requires at least two items")
		}
		spec.Components = append(spec.Components, schema.Component{Kind: schema.KindRanking, ID: o.id, Label: o.title, Help: o.help, Required: o.required, Items: itemsFrom(values)})
	case "checklist":
		if len(values) == 0 {
			return errors.New("checklist requires at least one item")
		}
		spec.Components = append(spec.Components, schema.Component{Kind: schema.KindChecklist, ID: o.id, Label: o.title, Help: o.help, Required: o.required, Items: itemsFrom(values)})
	case "approve":
		if bodyFile != "" {
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				return err
			}
			body = string(data)
		}
		if body == "" && len(values) > 0 {
			body = strings.Join(values, " ")
		}
		if body != "" {
			spec.Components = append(spec.Components, schema.Component{Kind: schema.KindText, Content: body})
		}
		spec.Components = append(spec.Components, schema.Component{Kind: schema.KindApproval, ID: o.id, Label: o.title, Help: o.help, Required: o.required})
	}
	if o.notes != "" {
		spec.Components = append(spec.Components, schema.Component{Kind: schema.KindTextarea, ID: availableID(spec.Components, "notes"), Label: o.notes})
	}
	if err := schema.ValidateSpec(&spec); err != nil {
		return fmt.Errorf("invalid recipe: %w", err)
	}
	encoded, _ := json.Marshal(spec)
	return createDocument(newClient(*server), encoded, assets, out)
}

func recipeUsage(fs *flag.FlagSet, name string) {
	examples := map[string]string{
		"gallery":   `pane gallery --title "Pick a direction" --multi ./mockups/*.png`,
		"pick":      `pane pick --title "Choose a color" "Ocean blue" "Forest green"`,
		"rank":      `pane rank --title "Prioritize features" Search Export Sharing`,
		"checklist": `pane checklist --title "Packing list" Passport Charger Jacket`,
		"approve":   `pane approve --title "Release review" --body-file release.md --notes "Comments"`,
	}
	fmt.Fprintf(fs.Output(), "Usage: %s\n\nOptions:\n", examples[name])
	fs.PrintDefaults()
}

func defaultRecipeTitle(name string) string {
	return map[string]string{"gallery": "Choose images", "pick": "Choose an option", "rank": "Rank these items", "checklist": "Checklist", "approve": "Review and approve"}[name]
}
func recipeID(name string) string {
	if name == "gallery" || name == "pick" {
		return "selection"
	}
	if name == "rank" {
		return "ranking"
	}
	if name == "approve" {
		return "decision"
	}
	return "checklist"
}
func intPtr(v int) *int { return &v }

func galleryArg(arg string) (string, string) {
	if label, path, ok := strings.Cut(arg, "="); ok && label != "" && path != "" {
		return label, path
	}
	base := filepath.Base(arg)
	return strings.TrimSuffix(base, filepath.Ext(base)), arg
}

var nonID = regexp.MustCompile(`[^A-Za-z0-9]+`)

func valueFor(s string, i int) string {
	v := strings.Trim(nonID.ReplaceAllString(strings.ToLower(s), "_"), "_")
	if v == "" || (v[0] >= '0' && v[0] <= '9') {
		v = "item_" + strconv.Itoa(i+1)
	}
	return v
}
func optionsFrom(values []string) []schema.Option {
	r := make([]schema.Option, len(values))
	for i, value := range uniqueValues(values) {
		r[i] = schema.Option{Value: value, Label: values[i]}
	}
	return r
}
func itemsFrom(values []string) []schema.Item {
	r := make([]schema.Item, len(values))
	for i, value := range uniqueValues(values) {
		r[i] = schema.Item{Value: value, Label: values[i]}
	}
	return r
}
func uniqueValues(labels []string) []string {
	result := make([]string, len(labels))
	used := map[string]bool{}
	for i, label := range labels {
		base := valueFor(label, i)
		value := base
		for n := 2; used[value]; n++ {
			value = base + "_" + strconv.Itoa(n)
		}
		used[value] = true
		result[i] = value
	}
	return result
}
func availableID(components []schema.Component, base string) string {
	used := map[string]bool{}
	for _, c := range components {
		used[c.ID] = true
	}
	id := base
	for n := 2; used[id]; n++ {
		id = base + "_" + strconv.Itoa(n)
	}
	return id
}

type addEnvelope struct {
	ID       string        `json:"id"`
	URL      string        `json:"url"`
	Revision uint64        `json:"revision"`
	Status   schema.Status `json:"status"`
}

func add(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(out)
	server := fs.String("server", env("PANE_SERVER", ""), "service URL")
	token := fs.String("token", env("PANE_TOKEN", ""), "management token")
	theme := fs.String("theme", "", "set surface theme: light, dark, or system")
	id := fs.String("id", "", "result key (inferred from label if omitted)")
	label := fs.String("label", "", "component label")
	help := fs.String("hint", "", "help text")
	required := fs.Bool("required", false, "require a value before submit")
	placeholder := fs.String("placeholder", "", "input placeholder")
	level := fs.Int("level", 2, "heading level, 1 through 3")
	min := fs.Float64("min", 0, "minimum number")
	max := fs.Float64("max", 0, "maximum number")
	hasMin, hasMax := flagPresent(args, "min"), flagPresent(args, "max")
	fs.Usage = func() { addUsage(out) }
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	positionals := fs.Args()
	if len(positionals) < 2 {
		return errors.New("add requires a surface id and component kind; run pane add --help")
	}
	surfaceID, err := addSurfaceID(positionals[0], in)
	if err != nil {
		return err
	}
	kind, values := positionals[1], positionals[2:]
	r, err := resolveReceipt(surfaceID, *server, *token)
	if err != nil {
		return err
	}
	c := newClient(r.Server)
	data, err := c.request(http.MethodGet, "/api/v1/surfaces/"+url.PathEscape(r.ID), r.ManagementToken, "application/json", nil)
	if err != nil {
		return err
	}
	var current schema.SurfaceResponse
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("invalid surface response: %w", err)
	}
	if *theme != "" {
		current.Spec.Presentation.ColorScheme = *theme
		if err := schema.ValidateSpec(&current.Spec); err != nil {
			return fmt.Errorf("invalid --theme: %w", err)
		}
	}
	component, assetPath, err := componentFromAdd(kind, values, *id, *label, *help, *placeholder, *required, *level, hasMin, *min, hasMax, *max, current.Spec.Components)
	if err != nil {
		return err
	}
	if assetPath != "" {
		uploaded, err := c.upload(r.ID, r.ManagementToken, assetPath)
		if err != nil {
			return err
		}
		component.URL = uploaded
	}
	appendComponent(&current.Spec, component)
	if err := schema.ValidateSpec(&current.Spec); err != nil {
		return fmt.Errorf("invalid component: %w", err)
	}
	body, _ := json.Marshal(current.Spec)
	response, err := c.request(http.MethodPut, "/api/v1/surfaces/"+url.PathEscape(r.ID), r.ManagementToken, "application/json", body)
	if err != nil {
		return err
	}
	var updated struct {
		ID     string        `json:"id"`
		URL    string        `json:"url"`
		Result schema.Result `json:"result"`
	}
	if err := json.Unmarshal(response, &updated); err != nil {
		return fmt.Errorf("invalid update response: %w", err)
	}
	if updated.ID == "" || updated.URL == "" {
		return errors.New("update response omitted id or url")
	}
	return writeAddEnvelope(out, addEnvelope{ID: updated.ID, URL: updated.URL, Revision: updated.Result.Revision, Status: updated.Result.Status})
}

func addSurfaceID(arg string, in io.Reader) (string, error) {
	if arg != "-" {
		return arg, nil
	}
	if in == nil {
		return "", errors.New("add - requires JSON from the previous pane command on stdin")
	}
	data, err := io.ReadAll(io.LimitReader(in, 1<<20+1))
	if err != nil {
		return "", fmt.Errorf("read piped surface JSON: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("add - received empty stdin; pipe JSON from pane create or pane add")
	}
	if len(data) > 1<<20 {
		return "", errors.New("piped surface JSON exceeds 1 MiB")
	}
	var prior struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &prior); err != nil {
		return "", fmt.Errorf("add - expected surface JSON on stdin: %w", err)
	}
	if prior.ID == "" {
		return "", errors.New("piped surface JSON omitted id")
	}
	return prior.ID, nil
}

func writeAddEnvelope(out io.Writer, envelope addEnvelope) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func componentFromAdd(kind string, values []string, id, label, help, placeholder string, required bool, level int, hasMin bool, min float64, hasMax bool, max float64, existing []schema.Component) (schema.Component, string, error) {
	kind = strings.ReplaceAll(kind, "-", "_")
	c := schema.Component{Label: label, Help: help, Required: required, Placeholder: placeholder}
	needText := func() (string, error) {
		if len(values) == 0 {
			return "", errors.New(kind + " requires text")
		}
		return strings.Join(values, " "), nil
	}
	interactive := func(defaultLabel string) {
		if c.Label == "" {
			c.Label = defaultLabel
		}
		if id == "" {
			id = valueFor(c.Label, 0)
		}
		c.ID = availableID(existing, id)
	}
	switch kind {
	case "heading":
		v, e := needText()
		if e != nil {
			return c, "", e
		}
		c.Kind = schema.KindHeading
		c.Content = v
		c.Level = level
	case "text":
		v, e := needText()
		if e != nil {
			return c, "", e
		}
		c.Kind = schema.KindText
		c.Content = v
	case "divider":
		c.Kind = schema.KindDivider
	case "image":
		if len(values) != 1 {
			return c, "", errors.New("image requires one local path")
		}
		c.Kind = schema.KindImage
		c.Alt = label
		return c, values[0], nil
	case "input", "input_text", "textarea", "number", "checkbox", "toggle", "approve", "approval":
		labels := map[string]string{"input": "Response", "input_text": "Response", "textarea": "Notes", "number": "Value", "checkbox": "Check this", "toggle": "Enable", "approve": "Decision", "approval": "Decision"}
		interactive(labels[kind])
		kinds := map[string]schema.ComponentKind{"input": schema.KindInputText, "input_text": schema.KindInputText, "textarea": schema.KindTextarea, "number": schema.KindNumber, "checkbox": schema.KindCheckbox, "toggle": schema.KindToggle, "approve": schema.KindApproval, "approval": schema.KindApproval}
		c.Kind = kinds[kind]
		if kind == "number" {
			if hasMin {
				c.Min = &min
			}
			if hasMax {
				c.Max = &max
			}
		}
	case "select", "pick", "multi_select", "checklist", "rank", "ranking", "sort":
		if len(values) == 0 {
			return c, "", errors.New(kind + " requires one or more items")
		}
		interactive("Choose")
		if kind == "select" || kind == "pick" || kind == "multi_select" {
			if kind == "select" || kind == "pick" {
				c.Kind = schema.KindSelect
			} else {
				c.Kind = schema.KindMultiSelect
			}
			c.Options = optionsFrom(values)
		} else {
			if kind == "checklist" {
				c.Kind = schema.KindChecklist
			} else {
				c.Kind = schema.KindRanking
			}
			c.Items = itemsFrom(values)
		}
	default:
		return c, "", fmt.Errorf("unsupported add kind %q", kind)
	}
	return c, "", nil
}
func appendComponent(spec *schema.Spec, component schema.Component) {
	// Empty `pane create` uses a divider to satisfy V1 schema validation. It is
	// only a bootstrap marker and should not leak into the composed document.
	if len(spec.Components) == 1 && spec.Components[0].Kind == schema.KindDivider && spec.Components[0].ID == "" {
		spec.Components = nil
	}
	hadInteractive := false
	for _, existing := range spec.Components {
		if schema.IsInteractive(existing.Kind) {
			hadInteractive = true
			break
		}
	}
	spec.Components = append(spec.Components, component)
	if !hadInteractive && schema.IsInteractive(component.Kind) && spec.Actions.Submit == nil {
		spec.Actions.Submit = &schema.Action{Label: "Done"}
	}
}

func flagPresent(args []string, name string) bool {
	prefix := "--" + name
	for _, a := range args {
		if a == prefix || strings.HasPrefix(a, prefix+"=") {
			return true
		}
	}
	return false
}
func addUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  pane add <id|-> heading|text <text>
  pane add <id|-> image <path> [--label ALT]
  pane add <id|-> input|textarea|number|checkbox|toggle|approve|approval --label LABEL [--id KEY]
  pane add <id|-> pick|select|multi-select|checklist|rank|sort [--label LABEL] <item>...
  pane add <id|-> divider

Use - as the surface id to read the previous create/add JSON from stdin.
Common options: --id, --label, --hint, --required, --theme, --server, --token
Input options: --placeholder; number options: --min, --max; heading: --level`)
}
