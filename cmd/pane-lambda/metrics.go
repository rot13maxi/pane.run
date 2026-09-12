package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	usageMetricNamespace = "Pane/Usage"
	usageMetricService   = "Pane"
)

type metricDefinition struct {
	Name string `json:"Name"`
	Unit string `json:"Unit"`
}

type cloudWatchMetricDirective struct {
	Namespace  string             `json:"Namespace"`
	Dimensions [][]string         `json:"Dimensions"`
	Metrics    []metricDefinition `json:"Metrics"`
}

type embeddedMetricMetadata struct {
	Timestamp         int64                       `json:"Timestamp"`
	CloudWatchMetrics []cloudWatchMetricDirective `json:"CloudWatchMetrics"`
}

type usageMetricEvent struct {
	AWS     embeddedMetricMetadata `json:"_aws"`
	Service string                 `json:"Service"`
	Values  map[string]int         `json:"-"`
}

func (e usageMetricEvent) MarshalJSON() ([]byte, error) {
	type eventMetadata struct {
		AWS     embeddedMetricMetadata `json:"_aws"`
		Service string                 `json:"Service"`
	}
	base, err := json.Marshal(eventMetadata{AWS: e.AWS, Service: e.Service})
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	for name, value := range e.Values {
		fields[name] = value
	}
	return json.Marshal(fields)
}

// emitUsageMetrics writes one CloudWatch Embedded Metric Format event for a
// request. Values contain only global counters: no surface identifiers,
// capabilities, specification content, or client information are recorded.
func emitUsageMetrics(w io.Writer, now time.Time, method, requestPath string, status int) error {
	values := usageMetricValues(method, requestPath, status)
	if len(values) == 0 {
		return nil
	}
	names := []string{
		"SurfaceCreateAttempts",
		"SurfacesCreated",
		"SurfaceLoads",
		"StateWrites",
		"Submissions",
		"CreateErrors",
	}
	definitions := make([]metricDefinition, 0, len(values))
	for _, name := range names {
		if _, ok := values[name]; ok {
			definitions = append(definitions, metricDefinition{Name: name, Unit: "Count"})
		}
	}
	event := usageMetricEvent{
		AWS: embeddedMetricMetadata{
			Timestamp: now.UnixMilli(),
			CloudWatchMetrics: []cloudWatchMetricDirective{{
				Namespace:  usageMetricNamespace,
				Dimensions: [][]string{{"Service"}},
				Metrics:    definitions,
			}},
		},
		Service: usageMetricService,
		Values:  values,
	}
	return json.NewEncoder(w).Encode(event)
}

func usageMetricValues(method, requestPath string, status int) map[string]int {
	values := make(map[string]int)
	isCreate := method == http.MethodPost && (requestPath == "/api/v1/surfaces" || requestPath == "/api/v1/imports/a2ui")
	if isCreate {
		values["SurfaceCreateAttempts"] = 1
		if status == http.StatusCreated {
			values["SurfacesCreated"] = 1
		} else if status >= http.StatusBadRequest {
			values["CreateErrors"] = 1
		}
	}
	action, ok := publicAction(requestPath)
	if !ok || status != http.StatusOK {
		return values
	}
	switch {
	case method == http.MethodGet && action == "state":
		values["SurfaceLoads"] = 1
	case method == http.MethodPut && action == "state":
		values["StateWrites"] = 1
	case method == http.MethodPost && action == "submit":
		values["Submissions"] = 1
	}
	return values
}

func publicAction(requestPath string) (string, bool) {
	const prefix = "/api/v1/public/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(requestPath, prefix), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
