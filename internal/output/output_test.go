package output

import (
	"bytes"
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
