package output

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestWriteTextClusterList(t *testing.T) {
	var out bytes.Buffer
	value := []map[string]any{
		{
			"name":        "alpha",
			"id":          "1",
			"agentStatus": "connected",
			"kedaStatus":  "ready",
			"createdAt":   "2026-06-15",
			"agent": map[string]any{
				"version": "v0.6.1",
				"kedaConfigs": []any{
					map[string]any{"kedaVersion": "v2.18.0"},
				},
			},
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	for _, expected := range []string{
		"NAME",
		"ID",
		"AGENT VERSION",
		"KEDA VERSION",
		"AGENT STATUS",
		"KEDA STATUS",
		"AGE",
		"alpha",
		"1",
		"v0.6.1",
		"v2.18.0",
		"connected",
		"ready",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
	if strings.Contains(got, "---") {
		t.Fatalf("unexpected divider line in output: %q", got)
	}
	if !strings.Contains(got, "NAME   ID  AGENT VERSION  KEDA VERSION  AGENT STATUS  KEDA STATUS  AGE") {
		t.Fatalf("unexpected text output: %q", got)
	}
}

func TestWriteTextSingleCluster(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{
		"name":        "alpha",
		"id":          "1",
		"agentStatus": "connected",
		"kedaStatus":  "ready",
		"createdAt":   "2026-06-16",
		"agent": map[string]any{
			"version": "v0.6.1",
			"kedaConfigs": []any{
				map[string]any{"kedaVersion": "v2.18.0"},
			},
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	for _, expected := range []string{
		"NAME",
		"ID",
		"AGENT VERSION",
		"KEDA VERSION",
		"AGENT STATUS",
		"KEDA STATUS",
		"AGE",
		"alpha",
		"1",
		"v0.6.1",
		"v2.18.0",
		"connected",
		"ready",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
	if strings.Contains(got, "---") {
		t.Fatalf("unexpected divider line in output: %q", got)
	}
	if !strings.Contains(got, "NAME   ID  AGENT VERSION  KEDA VERSION  AGENT STATUS  KEDA STATUS  AGE") {
		t.Fatalf("unexpected padded output: %q", got)
	}
}

func TestWriteTextNonClusterMapFallsBackToYAML(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{
		"items": []any{
			map[string]any{
				"workloadName": "demo",
				"cpuRequest":   "100m",
			},
		},
		"pageInfo": map[string]any{
			"hasNext": false,
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "AGENT VERSION") {
		t.Fatalf("unexpected cluster table output: %q", got)
	}
	for _, expected := range []string{"items:", "workloadName: demo", "cpuRequest: 100m", "pageInfo:"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
}

func TestWriteTextNonClusterListFallsBackToYAML(t *testing.T) {
	var out bytes.Buffer
	value := []map[string]any{
		{
			"kind":         "cpu",
			"workloadName": "demo",
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "AGENT VERSION") {
		t.Fatalf("unexpected cluster table output: %q", got)
	}
	for _, expected := range []string{"- kind: cpu", "workloadName: demo"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
}

func TestWriteTextEmptyMapListFallsBackToYAML(t *testing.T) {
	var out bytes.Buffer
	value := []map[string]any{}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	if got != "[]\n" {
		t.Fatalf("output = %q, want YAML empty list", got)
	}
	if strings.Contains(got, "No clusters found.") {
		t.Fatalf("unexpected cluster-specific output: %q", got)
	}
}

func TestWriteTextRecommendationsListUsesTable(t *testing.T) {
	var out bytes.Buffer
	value := []any{
		map[string]any{
			"kind":        "Deployment",
			"name":        "demo",
			"namespace":   "default",
			"resourceUID": "default/deployment/demo/demo/cpu-requests",
			"labels": map[string]any{
				"originalValue":  "100m",
				"suggestedValue": "200m",
			},
		},
		map[string]any{
			"kind":        "Deployment",
			"name":        "demo",
			"namespace":   "default",
			"resourceUID": "default/deployment/demo/demo/memory-limits",
			"labels": map[string]any{
				"originalValue":  "512Mi",
				"suggestedValue": "256Mi",
			},
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	for _, expected := range []string{
		"KIND",
		"NAME",
		"NAMESPACE",
		"CPU REQUESTS",
		"CPU LIMITS",
		"MEMORY REQUESTS",
		"MEMORY LIMITS",
		"Deployment",
		"demo",
		"default",
		"100m -> 200m",
		"512Mi -> 256Mi",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
	if !strings.Contains(got, "KIND") || !strings.Contains(got, "MEMORY LIMITS") {
		t.Fatalf("unexpected header order in output %q", got)
	}
	if strings.Contains(got, "AGENT VERSION") {
		t.Fatalf("unexpected cluster table output: %q", got)
	}
	if strings.Contains(got, "- labels:") {
		t.Fatalf("unexpected yaml output: %q", got)
	}
}

func TestWriteTextRecommendationsFromSampleFileUsesTable(t *testing.T) {
	var items []any

	data, err := os.ReadFile("../../test/recommendations.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if err := json.Unmarshal(data, &items); err != nil {
		var payload struct {
			Items []any `json:"items"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		items = payload.Items
	}

	var out bytes.Buffer
	if err := Write(&out, items, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	for _, expected := range []string{
		"KIND",
		"NAME",
		"NAMESPACE",
		"CPU REQUESTS",
		"CPU LIMITS",
		"MEMORY REQUESTS",
		"MEMORY LIMITS",
		"Deployment",
		"keda-add-ons-http-interceptor",
		"keda",
		"250m -> 20m",
		"500m -> 100m",
		"20Mi -> 24Mi",
		"512Mi -> 73Mi",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
	if strings.Contains(got, "waiting") || strings.Contains(got, "20\n") {
		t.Fatalf("unexpected legacy recommendation columns in output %q", got)
	}
	if !strings.Contains(got, "keda-operator") || !strings.Contains(got, "100Mi -> 46Mi") {
		t.Fatalf("expected merged workload rows in output %q", got)
	}
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "MEMORY LIMITS") {
		t.Fatalf("missing recommendation headers in output %q", got)
	}
}

func TestWriteTextFlattenedGenericListFallsBackToYAML(t *testing.T) {
	var out bytes.Buffer
	value := []any{
		map[string]any{
			"kind":         "cpu",
			"workloadName": "demo",
		},
	}

	if err := Write(&out, value, "text"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "AGENT VERSION") {
		t.Fatalf("unexpected cluster table output: %q", got)
	}
	for _, expected := range []string{"- kind: cpu", "workloadName: demo"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in output %q", expected, got)
		}
	}
}
