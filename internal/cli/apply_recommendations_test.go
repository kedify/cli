package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRecommendationsDiffDryRunDoesNotMutateValuesFile(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)
	originalValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/keda-operator",
		"--namespace", "keda",
		"--container", "keda-operator",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--resources", "cpu-requests,memory-limits",
		"--min-confidence", "20",
		"--format", "diff",
		"--dry-run",
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code != 0 {
		t.Fatalf("Run() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "-        cpu: 100m") || !strings.Contains(stdout.String(), "+        cpu: 20m") {
		t.Fatalf("stdout = %q, want unified diff with cpu change", stdout.String())
	}
	currentValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(currentValues) != string(originalValues) {
		t.Fatalf("values file changed during dry-run")
	}
}

func TestApplyRecommendationsDiffPatchesValuesFile(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/keda-operator",
		"--namespace", "keda",
		"--container", "keda-operator",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--resources", "cpu-requests,memory-limits",
		"--min-confidence", "20",
		"--format", "diff",
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code != 0 {
		t.Fatalf("Run() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "+        memory: 138Mi") {
		t.Fatalf("stdout = %q, want unified diff with memory change", stdout.String())
	}
	valuesData, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	valuesText := string(valuesData)
	for _, expected := range []string{"cpu: 20m", "memory: 138Mi"} {
		if !strings.Contains(valuesText, expected) {
			t.Fatalf("values file = %q, want %q", valuesText, expected)
		}
	}
}

func TestApplyRecommendationsOverrideWritesOutputFile(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)
	outputFile := filepath.Join(t.TempDir(), "override.yaml")
	originalValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/kedify-agent",
		"--namespace", "keda",
		"--container", "manager",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--min-confidence", "20",
		"--format", "override",
		"--output-file", outputFile,
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code != 0 {
		t.Fatalf("Run() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	overrideData, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	overrideText := string(overrideData)
	for _, expected := range []string{"kedifyAgent:", "memory: 50Mi", "memory: 150Mi"} {
		if !strings.Contains(overrideText, expected) {
			t.Fatalf("override file = %q, want %q", overrideText, expected)
		}
	}
	if stdout.String() != overrideText {
		t.Fatalf("stdout = %q, want generated override yaml", stdout.String())
	}
	currentValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(currentValues) != string(originalValues) {
		t.Fatalf("values file changed during override output mode")
	}
}

func TestApplyRecommendationsFailsBelowConfidenceThreshold(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/keda-operator",
		"--namespace", "keda",
		"--container", "keda-operator",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--resources", "cpu-requests",
		"--format", "json",
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code == 0 {
		t.Fatalf("Run() exit code = %d, want non-zero", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if result.Result != "failed" {
		t.Fatalf("result = %#v", result)
	}
	if !containsReasonCode(result.Reasons, "below_confidence_threshold") {
		t.Fatalf("reasons = %#v, want below_confidence_threshold", result.Reasons)
	}
}

func TestApplyRecommendationsFailsWhenResourceIsMissing(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/kedify-agent",
		"--namespace", "keda",
		"--container", "manager",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--resources", "cpu-limits",
		"--min-confidence", "20",
		"--format", "json",
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code == 0 {
		t.Fatalf("Run() exit code = %d, want non-zero", code)
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if !containsReasonCode(result.Reasons, "not_found") {
		t.Fatalf("reasons = %#v, want not_found", result.Reasons)
	}
}

func TestApplyRecommendationsFailsWhenValuesMappingIsAmbiguous(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	valuesData, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	valuesData = append(valuesData, []byte(`
duplicates:
  another:
    name: keda-operator
    containerName: keda-operator
    resources:
      requests:
        cpu: 100m
`)...)
	if err := os.WriteFile(valuesFile, valuesData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{
		"apply", "recommendations", "deployment/keda-operator",
		"--namespace", "keda",
		"--container", "keda-operator",
		"--chart-path", chartPath,
		"--values-file", valuesFile,
		"--recommendations-file", recommendationsFile,
		"--resources", "cpu-requests",
		"--min-confidence", "20",
		"--format", "json",
	}, bytes.NewBuffer(nil), stdout, stderr)

	if code == 0 {
		t.Fatalf("Run() exit code = %d, want non-zero", code)
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if !containsReasonCode(result.Reasons, "ambiguous") {
		t.Fatalf("reasons = %#v, want ambiguous", result.Reasons)
	}
}

func copyTestChart(t *testing.T) (string, string) {
	t.Helper()

	sourceRoot, err := filepath.Abs("../../test/chart")
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	targetRoot := filepath.Join(t.TempDir(), "chart")
	copyDir(t, sourceRoot, targetRoot)

	return targetRoot, filepath.Join(targetRoot, "values.yaml")
}

func copyDir(t *testing.T, source, target string) {
	t.Helper()

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		targetPath := filepath.Join(target, entry.Name())

		if entry.IsDir() {
			copyDir(t, sourcePath, targetPath)
			continue
		}

		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if err := os.WriteFile(targetPath, data, info.Mode()); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
}

func testRecommendationsFile(t *testing.T) string {
	t.Helper()

	path, err := filepath.Abs("../../test/recommendations.json")
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	return path
}

func containsReasonCode(reasons []applyRecommendationReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
