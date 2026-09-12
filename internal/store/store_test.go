package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-surface/agent-surface/internal/schema"
)

func testSpec() schema.Spec {
	return schema.Spec{Version: schema.Version, Title: "Test", Components: []schema.Component{{ID: "name", Kind: schema.KindInputText, Label: "Name"}}}
}

func TestLifecycleAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surfaces.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rec, capability, err := s.Create(testSpec(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if capability == "" || rec.PublicID == "" || rec.ID == rec.PublicID {
		t.Fatal("missing or reused capabilities")
	}
	if _, err := s.Get(rec.ID, "wrong"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong token: %v", err)
	}
	updated, err := s.WriteState(rec.PublicID, 0, map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Result.Revision != 1 {
		t.Fatalf("revision=%d", updated.Result.Revision)
	}
	if _, err := s.WriteState(rec.PublicID, 0, map[string]any{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write: %v", err)
	}
	if _, err := s.Submit(rec.PublicID); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(rec.ID, capability)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Values["name"] != "Ada" || got.Result.Status != schema.StatusSubmitted {
		t.Fatalf("result=%+v", got.Result)
	}
}

func TestUpdateFiltersIncompatibleState(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "db.json"))
	rec, token, _ := s.Create(testSpec(), time.Hour)
	_, _ = s.WriteState(rec.PublicID, 0, map[string]any{"name": "Ada"})
	next := schema.Spec{Version: schema.Version, Title: "Next", Components: []schema.Component{{ID: "name", Kind: schema.KindNumber, Label: "Count"}}}
	got, err := s.Update(rec.ID, token, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Result.Values["name"]; ok {
		t.Fatal("incompatible value was preserved")
	}
}
