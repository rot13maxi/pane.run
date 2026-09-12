package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
)

var (
	ErrNotFound  = errors.New("surface not found")
	ErrExpired   = errors.New("surface expired")
	ErrForbidden = errors.New("invalid management capability")
	ErrConflict  = errors.New("revision conflict")
	ErrClosed    = errors.New("surface is closed")
)

type Asset struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type Surface struct {
	ID             string        `json:"id"`
	PublicID       string        `json:"public_id"`
	ManagementHash string        `json:"management_hash"`
	Spec           schema.Spec   `json:"spec"`
	Result         schema.Result `json:"result"`
	CreatedAt      time.Time     `json:"created_at"`
	ExpiresAt      time.Time     `json:"expires_at"`
	ClosedAt       *time.Time    `json:"closed_at,omitempty"`
	Assets         []Asset       `json:"assets,omitempty"`
}

type database struct {
	Surfaces map[string]*Surface `json:"surfaces"`
}

type Store struct {
	mu       sync.RWMutex
	path     string
	assetDir string
	now      func() time.Time
	db       database
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store path is required")
	}
	s := &Store{path: path, assetDir: filepath.Join(filepath.Dir(path), "assets"), now: time.Now, db: database{Surfaces: make(map[string]*Surface)}}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.assetDir, 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) != 0 {
		if err := json.Unmarshal(b, &s.db); err != nil {
			return nil, fmt.Errorf("decode store: %w", err)
		}
		if s.db.Surfaces == nil {
			s.db.Surfaces = make(map[string]*Surface)
		}
	}
	return s, nil
}

func token(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *Store) Create(spec schema.Spec, ttl time.Duration) (Surface, string, error) {
	return s.CreateWithValues(spec, ttl, nil)
}

// CreateWithValues creates a surface with validated initial autosave state.
func (s *Store) CreateWithValues(spec schema.Spec, ttl time.Duration, values map[string]any) (Surface, string, error) {
	if values == nil {
		values = map[string]any{}
	}
	if err := schema.ValidateValues(spec, values); err != nil {
		return Surface{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := token(18)
	if err != nil {
		return Surface{}, "", err
	}
	pub, err := token(18)
	if err != nil {
		return Surface{}, "", err
	}
	management, err := token(32)
	if err != nil {
		return Surface{}, "", err
	}
	now := s.now().UTC()
	surface := &Surface{ID: id, PublicID: pub, ManagementHash: hashToken(management), Spec: spec, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	surface.Result.Status = schema.StatusActive
	surface.Result.Revision = 0
	surface.Result.Values = values
	surface.Result.CreatedAt = now
	surface.Result.UpdatedAt = now
	surface.Result.ExpiresAt = surface.ExpiresAt
	s.db.Surfaces[id] = surface
	if err := s.saveLocked(); err != nil {
		delete(s.db.Surfaces, id)
		return Surface{}, "", err
	}
	return clone(surface), management, nil
}

func (s *Store) Get(id, management string) (Surface, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, err := s.managementLocked(id, management)
	if err != nil {
		return Surface{}, err
	}
	return clone(v), nil
}

func (s *Store) Public(publicID string) (Surface, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.byPublicLocked(publicID)
	if v == nil {
		return Surface{}, ErrNotFound
	}
	if !s.now().Before(v.ExpiresAt) {
		return Surface{}, ErrExpired
	}
	return clone(v), nil
}

func (s *Store) Update(id, management string, spec schema.Spec) (Surface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.managementLocked(id, management)
	if err != nil {
		return Surface{}, err
	}
	v.Result.Values = schema.FilterCompatibleValues(v.Spec, spec, v.Result.Values)
	v.Spec = spec
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if err := s.saveLocked(); err != nil {
		return Surface{}, err
	}
	return clone(v), nil
}

func (s *Store) WriteState(publicID string, revision uint64, values map[string]any) (Surface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byPublicLocked(publicID)
	if v == nil {
		return Surface{}, ErrNotFound
	}
	if !s.now().Before(v.ExpiresAt) {
		return Surface{}, ErrExpired
	}
	if v.ClosedAt != nil {
		return clone(v), ErrClosed
	}
	if revision != v.Result.Revision {
		return clone(v), ErrConflict
	}
	if err := schema.ValidateValues(v.Spec, values); err != nil {
		return Surface{}, err
	}
	v.Result.Values = values
	v.Result.Revision++
	v.Result.UpdatedAt = s.now().UTC()
	if err := s.saveLocked(); err != nil {
		return Surface{}, err
	}
	return clone(v), nil
}

func (s *Store) Submit(publicID string) (Surface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byPublicLocked(publicID)
	if v == nil {
		return Surface{}, ErrNotFound
	}
	if !s.now().Before(v.ExpiresAt) {
		return Surface{}, ErrExpired
	}
	if v.ClosedAt != nil {
		return clone(v), ErrClosed
	}
	if err := schema.ValidateSubmission(v.Spec, v.Result.Values); err != nil {
		return Surface{}, err
	}
	now := s.now().UTC()
	v.Result.Status = schema.StatusSubmitted
	v.Result.SubmittedAt = &now
	v.Result.Revision++
	v.Result.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return Surface{}, err
	}
	return clone(v), nil
}
func (s *Store) Reset(publicID string) (Surface, error) {
	return s.setStatus(publicID, schema.StatusActive, true)
}

func (s *Store) setStatus(publicID string, status schema.Status, reset bool) (Surface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byPublicLocked(publicID)
	if v == nil {
		return Surface{}, ErrNotFound
	}
	if !s.now().Before(v.ExpiresAt) {
		return Surface{}, ErrExpired
	}
	if v.ClosedAt != nil {
		return clone(v), ErrClosed
	}
	now := s.now().UTC()
	v.Result.Status = status
	if reset {
		v.Result.Values = map[string]any{}
		v.Result.SubmittedAt = nil
	}
	if status == schema.StatusSubmitted {
		v.Result.SubmittedAt = &now
	}
	v.Result.Revision++
	v.Result.UpdatedAt = now
	if err := s.saveLocked(); err != nil {
		return Surface{}, err
	}
	return clone(v), nil
}

func (s *Store) Close(id, management string) (Surface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.managementLocked(id, management)
	if err != nil {
		return Surface{}, err
	}
	if v.ClosedAt == nil {
		now := s.now().UTC()
		v.ClosedAt = &now
		v.Result.Status = schema.StatusClosed
		v.Result.Revision++
		v.Result.UpdatedAt = now
		if err := s.saveLocked(); err != nil {
			return Surface{}, err
		}
	}
	return clone(v), nil
}

func (s *Store) Delete(id, management string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.managementLocked(id, management)
	if err != nil {
		return err
	}
	delete(s.db.Surfaces, id)
	if err := s.saveLocked(); err != nil {
		s.db.Surfaces[id] = v
		return err
	}
	return os.RemoveAll(filepath.Join(s.assetDir, v.PublicID))
}

func (s *Store) AddAsset(id, management, filename, contentType string, data []byte) (Surface, Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.managementLocked(id, management)
	if err != nil {
		return Surface{}, Asset{}, err
	}
	assetID, err := token(18)
	if err != nil {
		return Surface{}, Asset{}, err
	}
	dir := filepath.Join(s.assetDir, v.PublicID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Surface{}, Asset{}, err
	}
	path := filepath.Join(dir, assetID)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return Surface{}, Asset{}, err
	}
	a := Asset{ID: assetID, Filename: filepath.Base(filename), ContentType: contentType, Size: int64(len(data))}
	v.Assets = append(v.Assets, a)
	if err := s.saveLocked(); err != nil {
		_ = os.Remove(path)
		v.Assets = v.Assets[:len(v.Assets)-1]
		return Surface{}, Asset{}, err
	}
	return clone(v), a, nil
}

func (s *Store) Asset(publicID, assetID string) (Asset, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.byPublicLocked(publicID)
	if v == nil {
		return Asset{}, "", ErrNotFound
	}
	if !s.now().Before(v.ExpiresAt) {
		return Asset{}, "", ErrExpired
	}
	for _, a := range v.Assets {
		if a.ID == assetID {
			return a, filepath.Join(s.assetDir, v.PublicID, a.ID), nil
		}
	}
	return Asset{}, "", ErrNotFound
}

func (s *Store) managementLocked(id, capability string) (*Surface, error) {
	v := s.db.Surfaces[id]
	if v == nil {
		return nil, ErrNotFound
	}
	want, got := []byte(v.ManagementHash), []byte(hashToken(capability))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		return nil, ErrForbidden
	}
	if !s.now().Before(v.ExpiresAt) {
		return nil, ErrExpired
	}
	return v, nil
}

func (s *Store) byPublicLocked(id string) *Surface {
	for _, v := range s.db.Surfaces {
		if v.PublicID == id {
			return v
		}
	}
	return nil
}

func clone(v *Surface) Surface {
	b, _ := json.Marshal(v)
	var out Surface
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".surfaces-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
