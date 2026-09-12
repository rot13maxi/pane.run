package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/agent-surface/agent-surface/internal/a2ui"
	"github.com/agent-surface/agent-surface/internal/render"

	"github.com/agent-surface/agent-surface/internal/schema"
	"github.com/agent-surface/agent-surface/internal/store"
)

const (
	maxJSON    = 1 << 20
	maxAsset   = 10 << 20
	defaultTTL = 24 * time.Hour
	minTTL     = time.Minute
	maxTTL     = 7 * 24 * time.Hour
)

type Handler struct {
	store   SurfaceStore
	baseURL string
	log     *log.Logger
	pages   PageStore
}

type SurfaceStore interface {
	Create(schema.Spec, time.Duration) (store.Surface, string, error)
	CreateWithValues(schema.Spec, time.Duration, map[string]any) (store.Surface, string, error)
	Get(string, string) (store.Surface, error)
	Public(string) (store.Surface, error)
	Update(string, string, schema.Spec) (store.Surface, error)
	WriteState(string, uint64, map[string]any) (store.Surface, error)
	Submit(string) (store.Surface, error)
	Reset(string) (store.Surface, error)
	Close(string, string) (store.Surface, error)
	Delete(string, string) error
	AddAsset(string, string, string, string, []byte) (store.Surface, store.Asset, error)
	Asset(string, string) (store.Asset, string, error)
}

type PageStore interface {
	PutPage(store.Surface, []byte) error
	DeletePage(string) error
}

func New(st SurfaceStore, baseURL string, logger *log.Logger) http.Handler {
	return NewHosted(st, baseURL, logger, nil)
}

func NewHosted(st SurfaceStore, baseURL string, logger *log.Logger, pages PageStore) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{store: st, baseURL: strings.TrimRight(baseURL, "/"), log: logger, pages: pages}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch {
	case r.URL.Path == "/healthz" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]string{"status": "ok"})
	case r.URL.Path == "/api/v1/surfaces" && r.Method == http.MethodPost:
		h.create(w, r)
	case r.URL.Path == "/api/v1/imports/a2ui" && r.Method == http.MethodPost:
		h.importA2UI(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/surfaces/"):
		h.management(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/public/"):
		h.publicAPI(w, r)
	case strings.HasPrefix(r.URL.Path, "/s/") && r.Method == http.MethodGet:
		h.page(w, r)
	case strings.HasPrefix(r.URL.Path, "/a/") && r.Method == http.MethodGet:
		h.asset(w, r)
	default:
		writeError(w, 404, "not_found", "route not found")
	}
}

func (h *Handler) importA2UI(w http.ResponseWriter, r *http.Request) {
	if p := r.URL.Query().Get("protocol"); p != "" && p != "v0.9.1" {
		writeError(w, 400, "unsupported_protocol", "protocol must be v0.9.1")
		return
	}
	ttlSeconds := 0
	if raw := r.URL.Query().Get("ttl_seconds"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, 400, "invalid_ttl", "ttl_seconds must be an integer")
			return
		}
		ttlSeconds = value
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSON)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, 400, "invalid_a2ui", "could not read A2UI batch")
		return
	}
	result, err := a2ui.Import(body, a2ui.Options{Title: r.URL.Query().Get("title"), Description: r.URL.Query().Get("description"), TTLSeconds: ttlSeconds})
	if err != nil {
		writeError(w, 400, "invalid_a2ui", err.Error())
		return
	}
	ttl := defaultTTL
	if ttlSeconds != 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}
	s, token, err := h.store.CreateWithValues(result.Spec, ttl, result.Values)
	if err != nil {
		h.internal(w, err)
		return
	}
	if err := h.publish(s); err != nil {
		_ = h.store.Delete(s.ID, token)
		h.internal(w, fmt.Errorf("publish imported page: %w", err))
		return
	}
	writeJSON(w, 201, map[string]any{"id": s.ID, "public_id": s.PublicID, "url": h.baseURL + "/s/" + s.PublicID, "management_token": token, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt, "import": map[string]any{"format": "a2ui", "protocol": "v0.9.1", "surface_id": result.SurfaceID, "warnings": result.Warnings}})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var spec schema.Spec
	if err := decodeJSON(w, r, &spec); err != nil {
		return
	}
	if err := schema.ValidateSpec(&spec); err != nil {
		writeError(w, 400, "invalid_spec", err.Error())
		return
	}
	ttl := defaultTTL
	if spec.TTLSeconds != 0 {
		ttl = time.Duration(spec.TTLSeconds) * time.Second
	}
	if ttl < minTTL || ttl > maxTTL {
		writeError(w, 400, "invalid_ttl", "ttl_seconds must be between 60 and 604800")
		return
	}
	s, token, err := h.store.Create(spec, ttl)
	if err != nil {
		h.internal(w, err)
		return
	}
	if err := h.publish(s); err != nil {
		_ = h.store.Delete(s.ID, token)
		h.internal(w, fmt.Errorf("publish page: %w", err))
		return
	}
	writeJSON(w, 201, map[string]any{"id": s.ID, "public_id": s.PublicID, "url": h.baseURL + "/s/" + s.PublicID, "management_token": token, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt})
}

func (h *Handler) management(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/surfaces/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, 404, "not_found", "surface not found")
		return
	}
	id, token := parts[0], bearer(r)
	if token == "" {
		writeError(w, 401, "unauthorized", "management token required")
		return
	}
	var s store.Surface
	var err error
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s, err = h.store.Get(id, token)
		case http.MethodPut:
			var spec schema.Spec
			if decodeJSON(w, r, &spec) != nil {
				return
			}
			if e := schema.ValidateSpec(&spec); e != nil {
				writeError(w, 400, "invalid_spec", e.Error())
				return
			}
			s, err = h.store.Update(id, token, spec)
			if err == nil {
				err = h.publish(s)
			}
		case http.MethodDelete:
			current, getErr := h.store.Get(id, token)
			if getErr != nil {
				err = getErr
				break
			}
			// Hosted content must be removed before its authorization record. If
			// object deletion fails, retaining the record keeps this operation
			// authenticated and safely retryable with the same capability.
			if h.pages != nil {
				err = h.pages.DeletePage(current.PublicID)
			}
			if err == nil {
				err = h.store.Delete(id, token)
			}
			if err == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		default:
			writeError(w, 405, "method_not_allowed", "method not allowed")
			return
		}
	} else if len(parts) == 2 && parts[1] == "close" && r.Method == http.MethodPost {
		s, err = h.store.Close(id, token)
		if err == nil {
			err = h.publish(s)
		}
	} else if len(parts) == 2 && parts[1] == "results" && r.Method == http.MethodGet {
		s, err = h.store.Get(id, token)
		if err == nil {
			writeJSON(w, 200, s.Result)
			return
		}
	} else if len(parts) == 2 && parts[1] == "assets" && r.Method == http.MethodPost {
		h.upload(w, r, id, token)
		return
	} else {
		writeError(w, 404, "not_found", "route not found")
		return
	}
	if err != nil {
		h.storeError(w, err, nil)
		return
	}
	writeJSON(w, 200, managementView(s, h.baseURL))
}

func (h *Handler) publish(s store.Surface) error {
	if h.pages == nil {
		return nil
	}
	var page strings.Builder
	root := "/api/v1/public/" + s.PublicID
	if err := render.Render(&page, render.Page{Spec: s.Spec, Result: s.Result, PublicID: s.PublicID, StateURL: root + "/state", SubmitURL: root + "/submit", ResetURL: root + "/reset", ReadOnly: s.ClosedAt != nil}); err != nil {
		return err
	}
	return h.pages.PutPage(s, []byte(page.String()))
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request, id, token string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAsset+(64<<10))
	filename := strings.TrimSpace(r.Header.Get("X-Filename"))
	contentType := r.Header.Get("Content-Type")
	reader := io.Reader(r.Body)
	if strings.HasPrefix(contentType, "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			writeError(w, 400, "invalid_multipart", "invalid multipart upload")
			return
		}
		part, err := mr.NextPart()
		if err != nil {
			writeError(w, 400, "empty_asset", "asset body is empty")
			return
		}
		defer part.Close()
		reader = part
		filename = part.FileName()
		contentType = part.Header.Get("Content-Type")
	}
	b, err := io.ReadAll(io.LimitReader(reader, maxAsset+1))
	if err != nil || len(b) > maxAsset {
		writeError(w, 413, "asset_too_large", "asset exceeds 10 MiB")
		return
	}
	if len(b) == 0 {
		writeError(w, 400, "empty_asset", "asset body is empty")
		return
	}
	filename = path.Base(filename)
	if filename == "." || filename == "/" || filename == "" {
		filename = "asset"
	}
	if len(filename) > 255 {
		writeError(w, 400, "invalid_filename", "filename is too long")
		return
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(b)
	}
	if mt, _, e := mime.ParseMediaType(contentType); e == nil {
		contentType = mt
	} else {
		contentType = "application/octet-stream"
	}
	s, a, err := h.store.AddAsset(id, token, filename, contentType, b)
	if err != nil {
		h.storeError(w, err, nil)
		return
	}
	writeJSON(w, 201, map[string]any{"id": a.ID, "filename": a.Filename, "content_type": a.ContentType, "size": a.Size, "url": h.baseURL + "/a/" + s.PublicID + "/" + a.ID})
}

func (h *Handler) publicAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/public/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		writeError(w, 404, "not_found", "route not found")
		return
	}
	publicID, action := parts[0], parts[1]
	if action == "state" && r.Method == http.MethodGet {
		s, e := h.store.Public(publicID)
		if e != nil {
			h.storeError(w, e, nil)
			return
		}
		writeJSON(w, 200, publicView(s))
		return
	}
	if action == "state" && r.Method == http.MethodPut {
		var input schema.StateUpdate
		if decodeJSON(w, r, &input) != nil {
			return
		}
		if input.Values == nil {
			input.Values = map[string]any{}
		}
		s, e := h.store.WriteState(publicID, input.Revision, input.Values)
		if e != nil {
			h.storeError(w, e, &s)
			return
		}
		writeJSON(w, 200, publicView(s))
		return
	}
	if action == "submit" && r.Method == http.MethodPost {
		s, e := h.store.Submit(publicID)
		if e != nil {
			h.storeError(w, e, &s)
			return
		}
		writeJSON(w, 200, publicView(s))
		return
	}
	if action == "reset" && r.Method == http.MethodPost {
		s, e := h.store.Reset(publicID)
		if e != nil {
			h.storeError(w, e, &s)
			return
		}
		writeJSON(w, 200, publicView(s))
		return
	}
	writeError(w, 405, "method_not_allowed", "method not allowed")
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/s/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, 404, "not_found", "surface not found")
		return
	}
	s, e := h.store.Public(id)
	if e != nil {
		h.storeError(w, e, nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	root := "/api/v1/public/" + s.PublicID
	if e := render.Render(w, render.Page{Spec: s.Spec, Result: s.Result, PublicID: s.PublicID, StateURL: root + "/state", SubmitURL: root + "/submit", ResetURL: root + "/reset", ReadOnly: s.ClosedAt != nil}); e != nil {
		h.log.Printf("render: %v", e)
	}
}

func (h *Handler) asset(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/a/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		writeError(w, 404, "not_found", "asset not found")
		return
	}
	a, file, e := h.store.Asset(parts[0], parts[1])
	if e != nil {
		h.storeError(w, e, nil)
		return
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", strings.ReplaceAll(a.Filename, "\"", "")))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeFile(w, r, file)
}

func managementView(s store.Surface, base string) map[string]any {
	return map[string]any{"id": s.ID, "public_id": s.PublicID, "url": base + "/s/" + s.PublicID, "spec": s.Spec, "result": s.Result, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt, "closed_at": s.ClosedAt, "assets": s.Assets}
}
func publicView(s store.Surface) map[string]any {
	return map[string]any{"result": s.Result, "closed": s.ClosedAt != nil, "expires_at": s.ExpiresAt}
}

func bearer(r *http.Request) string {
	const p = "Bearer "
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, p) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, p))
}
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSON)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "invalid JSON: "+err.Error())
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, 400, "invalid_json", "request must contain one JSON value")
		return errors.New("trailing JSON")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (h *Handler) internal(w http.ResponseWriter, err error) {
	h.log.Printf("server: %v", err)
	writeError(w, 500, "internal_error", "internal server error")
}
func (h *Handler) storeError(w http.ResponseWriter, err error, current *store.Surface) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, 404, "not_found", "surface not found")
	case errors.Is(err, store.ErrForbidden):
		writeError(w, 403, "forbidden", "invalid management token")
	case errors.Is(err, store.ErrExpired):
		writeError(w, 410, "expired", "surface has expired")
	case errors.Is(err, store.ErrClosed):
		writeError(w, 409, "closed", "surface is closed")
	case errors.Is(err, store.ErrConflict):
		if current != nil {
			writeJSON(w, 409, map[string]any{"error": map[string]string{"code": "revision_conflict", "message": "state revision is stale"}, "result": current.Result})
		} else {
			writeError(w, 409, "revision_conflict", "state revision is stale")
		}
	default:
		if schema.IsValidationError(err) {
			writeError(w, 400, "invalid_values", err.Error())
		} else {
			h.internal(w, err)
		}
	}
}
