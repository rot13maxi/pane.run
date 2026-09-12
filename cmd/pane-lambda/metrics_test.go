package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestUsageMetricValues(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
		want   map[string]int
	}{
		{"native create", http.MethodPost, "/api/v1/surfaces", 201, map[string]int{"SurfaceCreateAttempts": 1, "SurfacesCreated": 1}},
		{"import create", http.MethodPost, "/api/v1/imports/a2ui", 201, map[string]int{"SurfaceCreateAttempts": 1, "SurfacesCreated": 1}},
		{"rejected create", http.MethodPost, "/api/v1/surfaces", 400, map[string]int{"SurfaceCreateAttempts": 1, "CreateErrors": 1}},
		{"failed create", http.MethodPost, "/api/v1/surfaces", 500, map[string]int{"SurfaceCreateAttempts": 1, "CreateErrors": 1}},
		{"surface load", http.MethodGet, "/api/v1/public/public-id/state", 200, map[string]int{"SurfaceLoads": 1}},
		{"state write", http.MethodPut, "/api/v1/public/public-id/state", 200, map[string]int{"StateWrites": 1}},
		{"submission", http.MethodPost, "/api/v1/public/public-id/submit", 200, map[string]int{"Submissions": 1}},
		{"failed load", http.MethodGet, "/api/v1/public/public-id/state", 410, map[string]int{}},
		{"management request", http.MethodGet, "/api/v1/surfaces/id", 200, map[string]int{}},
		{"extra public segment", http.MethodGet, "/api/v1/public/id/state/more", 200, map[string]int{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usageMetricValues(test.method, test.path, test.status); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("metrics = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestEmitUsageMetrics(t *testing.T) {
	var out bytes.Buffer
	now := time.Date(2026, 9, 12, 12, 0, 0, 123000000, time.UTC)
	if err := emitUsageMetrics(&out, now, http.MethodPost, "/api/v1/surfaces", 201); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(out.Bytes(), &event); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if event["Service"] != usageMetricService || event["SurfaceCreateAttempts"] != float64(1) || event["SurfacesCreated"] != float64(1) {
		t.Fatalf("event = %#v", event)
	}
	metadata := event["_aws"].(map[string]any)
	if metadata["Timestamp"] != float64(now.UnixMilli()) {
		t.Fatalf("timestamp = %#v", metadata["Timestamp"])
	}
	directive := metadata["CloudWatchMetrics"].([]any)[0].(map[string]any)
	if directive["Namespace"] != usageMetricNamespace {
		t.Fatalf("namespace = %#v", directive["Namespace"])
	}
}

func TestEmitUsageMetricsSkipsUntrackedRequest(t *testing.T) {
	var out bytes.Buffer
	if err := emitUsageMetrics(&out, time.Now(), http.MethodGet, "/healthz", 200); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected event: %s", out.String())
	}
}
