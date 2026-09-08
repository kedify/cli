package analyze

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kedify/recommender/analysis"

	clictx "github.com/kedify/cli/internal/cli/context"
)

const validSnapshotRequest = `{"input":{"schemaVersion":"resource-analysis-input/v1","observedIntervalHours":24,"containers":[]},"policy":{}}`

func TestRecommendationsAnalyzesSnapshotFromStdin(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := (&RecommendationsCmd{Snapshot: "-"}).Run(&clictx.Context{
		Stdin:  strings.NewReader(validSnapshotRequest),
		Stdout: stdout,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var result analysis.Output
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.SchemaVersion != analysis.OutputSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", result.SchemaVersion, analysis.OutputSchemaVersion)
	}
	if result.DetectorVersion != analysis.ResourceRightSizeDetectorVersion {
		t.Fatalf("detectorVersion = %q, want %q", result.DetectorVersion, analysis.ResourceRightSizeDetectorVersion)
	}
	if result.PolicyVersion == "" {
		t.Fatal("policyVersion is empty")
	}
	if result.Results == nil || len(result.Results) != 0 {
		t.Fatalf("results = %#v, want an empty array", result.Results)
	}
}

func TestRecommendationsReadsSnapshotFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte(validSnapshotRequest), 0o600); err != nil {
		t.Fatal(err)
	}

	err := (&RecommendationsCmd{Snapshot: path}).Run(&clictx.Context{Stdout: io.Discard})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRecommendationsRejectsInvalidSnapshot(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "malformed JSON", data: `{`, want: "invalid snapshot request"},
		{name: "unknown field", data: `{"unknown":true}`, want: "unknown field"},
		{name: "multiple objects", data: validSnapshotRequest + `{}`, want: "expected one JSON object"},
		{name: "unsupported schema", data: `{"input":{"schemaVersion":"resource-analysis-input/v2","observedIntervalHours":24},"policy":{}}`, want: "unsupported input schema version"},
		{name: "invalid interval", data: `{"input":{"schemaVersion":"resource-analysis-input/v1"},"policy":{}}`, want: "observedIntervalHours must be greater than 0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (&RecommendationsCmd{Snapshot: "-"}).Run(&clictx.Context{
				Stdin:  strings.NewReader(test.data),
				Stdout: io.Discard,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRecommendationsRejectsOversizedSnapshot(t *testing.T) {
	err := (&RecommendationsCmd{Snapshot: "-"}).Run(&clictx.Context{
		Stdin:  bytes.NewReader(bytes.Repeat([]byte(" "), maxSnapshotBytes+1)),
		Stdout: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "request exceeds 16777216-byte limit") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRecommendationsReportsOutputFailure(t *testing.T) {
	want := errors.New("write failed")
	err := (&RecommendationsCmd{Snapshot: "-"}).Run(&clictx.Context{
		Stdin:  strings.NewReader(validSnapshotRequest),
		Stdout: errorWriter{err: want},
	})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
