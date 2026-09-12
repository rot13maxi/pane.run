package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
)

const defaultServer = "http://localhost:8080"

type receipt struct {
	ID              string `json:"id"`
	Server          string `json:"server"`
	ManagementToken string `json:"management_token"`
}

type client struct {
	server string
	http   *http.Client
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "surface:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "create":
		return create(args[1:], stdout)
	case "gallery", "pick", "rank", "checklist", "approve":
		return recipe(args[0], args[1:], stdout)
	case "add":
		return add(args[1:], os.Stdin, stdout)
	case "read":
		return managed(http.MethodGet, "", args[1:], stdout)
	case "update":
		return update(args[1:], stdout)
	case "close":
		return managed(http.MethodPost, "/close", args[1:], stdout)
	case "delete":
		return managed(http.MethodDelete, "", args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, "surface dev")
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  surface gallery [options] <image>...
  surface pick [options] <choice>...
  surface rank [options] <item>...
  surface checklist [options] <item>...
  surface approve [options]
  surface add <id> <kind> [options] [values...]

  surface create [spec.json] [--title TITLE] [--server URL] [--asset name=path]
  surface read <id> [--server URL] [--token TOKEN]
  surface update <id> <spec.json> [--server URL] [--token TOKEN]
  surface close <id> [--server URL] [--token TOKEN]
  surface delete <id> [--server URL] [--token TOKEN]

Run "surface <command> --help" for recipe and component options.
Environment: SURFACE_SERVER, SURFACE_TOKEN, SURFACE_CONFIG_DIR`)
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func create(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(out)
	server := fs.String("server", env("SURFACE_SERVER", defaultServer), "service URL")
	title := fs.String("title", "Untitled surface", "page title when no specification file is given")
	description := fs.String("description", "", "page description when no specification file is given")
	ttl := fs.Duration("ttl", 0, "lifetime when no specification file is given, for example 30m or 48h")
	var assets stringList
	fs.Var(&assets, "asset", "name=path (repeatable)")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("create accepts at most one specification file")
	}
	var spec []byte
	var err error
	if fs.NArg() == 1 {
		spec, err = os.ReadFile(fs.Arg(0))
	} else {
		document := schema.Spec{Version: schema.Version, Title: *title, Description: *description,
			Components: []schema.Component{{Kind: schema.KindDivider}}}
		if *ttl != 0 {
			if *ttl%time.Second != 0 {
				return errors.New("--ttl must be a whole number of seconds")
			}
			document.TTLSeconds = int(*ttl / time.Second)
		}
		spec, err = json.Marshal(document)
	}
	if err != nil {
		return err
	}
	return createDocument(newClient(*server), spec, assets, out)
}

func createDocument(c client, spec []byte, assets []string, out io.Writer) error {
	created, err := c.request(http.MethodPost, "/api/v1/surfaces", "", "application/json", spec)
	if err != nil {
		return err
	}
	var meta map[string]any
	if err := json.Unmarshal(created, &meta); err != nil {
		return fmt.Errorf("invalid create response: %w", err)
	}
	id, _ := meta["id"].(string)
	token, _ := meta["management_token"].(string)
	if id == "" || token == "" {
		return errors.New("create response omitted id or management_token")
	}
	if err := saveReceipt(receipt{ID: id, Server: c.server, ManagementToken: token}); err != nil {
		return fmt.Errorf("surface created but receipt could not be saved: %w", err)
	}
	uploaded := map[string]string{}
	for _, item := range assets {
		name, path, ok := strings.Cut(item, "=")
		if !ok || name == "" || path == "" {
			return fmt.Errorf("invalid --asset %q, expected name=path", item)
		}
		assetURL, err := c.upload(id, token, path)
		if err != nil {
			return fmt.Errorf("upload %s: %w", name, err)
		}
		uploaded[name] = assetURL
	}
	if len(uploaded) > 0 {
		var document any
		if err := json.Unmarshal(spec, &document); err != nil {
			return err
		}
		document = replaceAssets(document, uploaded)
		updated, _ := json.Marshal(document)
		if _, err := c.request(http.MethodPut, "/api/v1/surfaces/"+url.PathEscape(id), token, "application/json", updated); err != nil {
			return fmt.Errorf("apply uploaded assets: %w", err)
		}
	}
	return pretty(out, created)
}

func update(args []string, out io.Writer) error {
	fs, server, token := managedFlags("update")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("update requires an id and specification file")
	}
	body, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	r, err := resolveReceipt(fs.Arg(0), *server, *token)
	if err != nil {
		return err
	}
	response, err := newClient(r.Server).request(http.MethodPut, "/api/v1/surfaces/"+url.PathEscape(r.ID), r.ManagementToken, "application/json", body)
	if err != nil {
		return err
	}
	return pretty(out, response)
}

func managed(method, suffix string, args []string, out io.Writer) error {
	fs, server, token := managedFlags(strings.ToLower(method))
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("command requires a surface id")
	}
	r, err := resolveReceipt(fs.Arg(0), *server, *token)
	if err != nil {
		return err
	}
	body, err := newClient(r.Server).request(method, "/api/v1/surfaces/"+url.PathEscape(r.ID)+suffix, r.ManagementToken, "application/json", nil)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		_, err = fmt.Fprintln(out, `{"ok":true}`)
		return err
	}
	return pretty(out, body)
}

func managedFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	server := fs.String("server", env("SURFACE_SERVER", ""), "service URL")
	token := fs.String("token", env("SURFACE_TOKEN", ""), "management token")
	return fs, server, token
}

func reorderFlags(args []string) []string {
	// The flag package stops at the first positional argument. Agent-generated commands
	// commonly put flags last, so move known flag/value pairs before positionals.
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") && i+1 < len(args) {
			flags = append(flags, args[i], args[i+1])
			i++
		} else {
			rest = append(rest, args[i])
		}
	}
	return append(flags, rest...)
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return fs.Parse(append(flags, rest...))
}

func newClient(server string) client {
	return client{server: strings.TrimRight(server, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c client) request(method, path, token, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, c.server+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("service returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (c client) upload(id, token, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, io.LimitReader(f, 10<<20+1)); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	data, err := c.request(http.MethodPost, "/api/v1/surfaces/"+url.PathEscape(id)+"/assets", token, w.FormDataContentType(), body.Bytes())
	if err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", errors.New("upload response omitted url")
	}
	return result.URL, nil
}

func replaceAssets(v any, assets map[string]string) any {
	switch value := v.(type) {
	case string:
		if name, ok := strings.CutPrefix(value, "asset:"); ok {
			if replacement, exists := assets[name]; exists {
				return replacement
			}
		}
	case []any:
		for i := range value {
			value[i] = replaceAssets(value[i], assets)
		}
	case map[string]any:
		for key := range value {
			value[key] = replaceAssets(value[key], assets)
		}
	}
	return v
}

func configDir() (string, error) {
	if v := os.Getenv("SURFACE_CONFIG_DIR"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "surface", "receipts"), nil
}

func saveReceipt(r receipt) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	return os.WriteFile(filepath.Join(dir, r.ID+".json"), data, 0600)
}

func resolveReceipt(id, server, token string) (receipt, error) {
	if server != "" && token != "" {
		return receipt{ID: id, Server: strings.TrimRight(server, "/"), ManagementToken: token}, nil
	}
	dir, err := configDir()
	if err == nil {
		data, readErr := os.ReadFile(filepath.Join(dir, id+".json"))
		if readErr == nil {
			var r receipt
			if json.Unmarshal(data, &r) == nil {
				if server != "" {
					r.Server = strings.TrimRight(server, "/")
				}
				if token != "" {
					r.ManagementToken = token
				}
				if r.Server != "" && r.ManagementToken != "" {
					return r, nil
				}
			}
		}
	}
	if server == "" {
		server = env("SURFACE_SERVER", defaultServer)
	}
	if token == "" {
		return receipt{}, errors.New("management token not found; pass --token or use the machine that created the surface")
	}
	return receipt{ID: id, Server: server, ManagementToken: token}, nil
}

func pretty(w io.Writer, data []byte) error {
	var v any
	if json.Unmarshal(data, &v) != nil {
		_, err := w.Write(data)
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
