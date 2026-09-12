package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReorderFlagsAllowsAgentFriendlyTrailingFlags(t *testing.T) {
	got := reorderFlags([]string{"abc", "spec.json", "--server", "https://example.test", "--token", "secret"})
	want := []string{"--server", "https://example.test", "--token", "secret", "abc", "spec.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reorderFlags() = %#v, want %#v", got, want)
	}
}

func TestReplaceAssetsRecursesWithoutChangingUnknownPlaceholders(t *testing.T) {
	var input any
	if err := json.Unmarshal([]byte(`{"hero":"asset:cover","items":[{"src":"asset:thumb"}],"other":"asset:missing"}`), &input); err != nil {
		t.Fatal(err)
	}
	got := replaceAssets(input, map[string]string{"cover": "/a/1", "thumb": "/a/2"})
	encoded, _ := json.Marshal(got)
	want := `{"hero":"/a/1","items":[{"src":"/a/2"}],"other":"asset:missing"}`
	if string(encoded) != want {
		t.Fatalf("replaceAssets() = %s, want %s", encoded, want)
	}
}

func TestResolveReceiptUsesPrivateFile(t *testing.T) {
	t.Setenv("SURFACE_CONFIG_DIR", t.TempDir())
	want := receipt{ID: "surface-1", Server: "https://surface.test", ManagementToken: "secret"}
	if err := saveReceipt(want); err != nil {
		t.Fatal(err)
	}
	got, err := resolveReceipt(want.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveReceipt() = %#v, want %#v", got, want)
	}
}
