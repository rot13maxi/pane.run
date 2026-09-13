package store

import (
	"errors"
	"path/filepath"
	"reflect"
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

func TestSubmissionIsFinal(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "db.json"))
	rec, token, _ := s.Create(testSpec(), time.Hour)
	_, _ = s.WriteState(rec.PublicID, 0, map[string]any{"name": "Ada"})
	submitted, err := s.Submit(rec.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteState(rec.PublicID, submitted.Result.Revision, map[string]any{"name": "Grace"}); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("write after submit: %v", err)
	}
	if _, err := s.Reset(rec.PublicID); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("reset after submit: %v", err)
	}
	if _, err := s.Submit(rec.PublicID); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("resubmit: %v", err)
	}
	if _, err := s.Update(rec.ID, token, testSpec()); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("update after submit: %v", err)
	}
	closed, err := s.Close(rec.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Result.Status != schema.StatusSubmitted || closed.Result.Values["name"] != "Ada" {
		t.Fatalf("close changed submitted result: %#v", closed.Result)
	}
}

func TestRankingStartsWithAuthoredOrder(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "db.json"))
	spec := schema.Spec{Version: schema.Version, Title: "Rank", Components: []schema.Component{{ID: "rank", Kind: schema.KindRanking, Label: "Rank", Required: true, Items: []schema.Item{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}}}}
	rec, _, err := s.Create(spec, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.Result.Values["rank"], []any{"a", "b"}) {
		t.Fatalf("initial ranking=%#v", rec.Result.Values["rank"])
	}
	if _, err := s.Submit(rec.PublicID); err != nil {
		t.Fatalf("untouched ranking did not submit: %v", err)
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
