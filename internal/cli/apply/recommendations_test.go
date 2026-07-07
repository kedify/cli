package apply

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	clierrors "github.com/kedify/cli/internal/errors"
	"github.com/kedify/cli/internal/output"
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
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		Container:           "keda-operator",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "diff",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "-            cpu: 100m") || !strings.Contains(stdout.String(), "+            cpu: 20m") {
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
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		Container:           "keda-operator",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "diff",
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "+            memory: 138Mi") {
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
	for _, expected := range []string{"name: audit-sidecar", "cpu: 5m", "memory: 64Mi"} {
		if !strings.Contains(valuesText, expected) {
			t.Fatalf("values file = %q, want unchanged sidecar value %q", valuesText, expected)
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
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/kedify-agent",
		Namespace:           "keda",
		Container:           "manager",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		MinConfidence:       20,
		Format:              "override",
		OutputFile:          outputFile,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	overrideData, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	overrideText := string(overrideData)
	for _, expected := range []string{"kedifyAgent:", "containers:", "manager:", "memory: 50Mi", "memory: 150Mi"} {
		if !strings.Contains(overrideText, expected) {
			t.Fatalf("override file = %q, want %q", overrideText, expected)
		}
	}
	if strings.Contains(overrideText, "proxy:") {
		t.Fatalf("override file = %q, did not expect unrelated sidecar override", overrideText)
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

func TestApplyRecommendationsJSONReportsContainerScopedPath(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)
	originalValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		Container:           "keda-operator",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "json",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if result.Result != "planned" {
		t.Fatalf("result = %#v, want planned", result)
	}
	if len(result.Patched) != 2 {
		t.Fatalf("patched = %#v, want two patched resources", result.Patched)
	}

	gotPaths := make([]string, 0, len(result.Patched))
	for _, patched := range result.Patched {
		gotPaths = append(gotPaths, patched.Path)
	}
	for _, expected := range []string{
		"deployments.kedaOperator.containers.operator.resources.requests.cpu",
		"deployments.kedaOperator.containers.operator.resources.limits.memory",
	} {
		if !slices.Contains(gotPaths, expected) {
			t.Fatalf("patched paths = %#v, want %q", gotPaths, expected)
		}
	}

	currentValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(currentValues) != string(originalValues) {
		t.Fatalf("values file changed during dry-run json mode")
	}
}

func TestApplyRecommendationsWithoutContainerAutoResolvesSingleMatchingContainer(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator-metrics-apiserver",
		Namespace:           "keda",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "json",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if result.Container != "keda-operator-metrics-apiserver" {
		t.Fatalf("result container = %q, want keda-operator-metrics-apiserver", result.Container)
	}
	if !slices.Equal(result.Containers, []string{"keda-operator-metrics-apiserver"}) {
		t.Fatalf("result containers = %#v, want single matched container", result.Containers)
	}
	if result.Result != "planned" {
		t.Fatalf("result = %#v, want planned", result)
	}
}

func TestApplyRecommendationsWithoutContainerPatchesAllMatchedContainers(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)
	originalValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "json",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if result.Result != "planned" {
		t.Fatalf("result = %#v, want planned", result)
	}
	if result.Container != "" {
		t.Fatalf("result container = %q, want empty for multi-container match", result.Container)
	}
	if !slices.Equal(result.Containers, []string{"audit-sidecar", "keda-operator"}) {
		t.Fatalf("result containers = %#v, want both matched containers", result.Containers)
	}
	if len(result.Patched) != 4 {
		t.Fatalf("patched = %#v, want four patched resources", result.Patched)
	}

	gotPaths := make([]string, 0, len(result.Patched))
	gotContainers := make([]string, 0, len(result.Patched))
	for _, patched := range result.Patched {
		gotPaths = append(gotPaths, patched.Path)
		gotContainers = append(gotContainers, patched.Container)
	}
	for _, expected := range []string{
		"deployments.kedaOperator.containers.operator.resources.requests.cpu",
		"deployments.kedaOperator.containers.operator.resources.limits.memory",
		"deployments.kedaOperator.containers.auditSidecar.resources.requests.cpu",
		"deployments.kedaOperator.containers.auditSidecar.resources.limits.memory",
	} {
		if !slices.Contains(gotPaths, expected) {
			t.Fatalf("patched paths = %#v, want %q", gotPaths, expected)
		}
	}
	for _, expected := range []string{"audit-sidecar", "keda-operator"} {
		if !slices.Contains(gotContainers, expected) {
			t.Fatalf("patched containers = %#v, want %q", gotContainers, expected)
		}
	}

	currentValues, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(currentValues) != string(originalValues) {
		t.Fatalf("values file changed during dry-run")
	}
}

func TestApplyRecommendationsWithoutContainerFailsWhenOneMatchedContainerIsMissingRequestedResource(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := writeRecommendationsFile(t, []map[string]any{
		{
			"kind":        "Deployment",
			"name":        "keda-operator",
			"namespace":   "keda",
			"resourceUID": "keda/deployment/keda-operator/keda-operator/cpu-requests",
			"status":      "waiting",
			"labels": map[string]any{
				"workloadContainer": "keda-operator",
				"originalValue":     "100m",
				"suggestedValue":    "20m",
				"confidence":        80,
			},
		},
		{
			"kind":        "Deployment",
			"name":        "keda-operator",
			"namespace":   "keda",
			"resourceUID": "keda/deployment/keda-operator/keda-operator/memory-limits",
			"status":      "waiting",
			"labels": map[string]any{
				"workloadContainer": "keda-operator",
				"originalValue":     "1000Mi",
				"suggestedValue":    "138Mi",
				"confidence":        80,
			},
		},
		{
			"kind":        "Deployment",
			"name":        "keda-operator",
			"namespace":   "keda",
			"resourceUID": "keda/deployment/keda-operator/audit-sidecar/cpu-requests",
			"status":      "waiting",
			"labels": map[string]any{
				"workloadContainer": "audit-sidecar",
				"originalValue":     "5m",
				"suggestedValue":    "10m",
				"confidence":        80,
			},
		},
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests,memory-limits",
		MinConfidence:       20,
		Format:              "json",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code == 0 {
		t.Fatalf("runApply() exit code = %d, want non-zero", code)
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if !containsReasonCode(result.Reasons, "not_found") {
		t.Fatalf("reasons = %#v, want not_found", result.Reasons)
	}
	if !strings.Contains(stdout.String(), "audit-sidecar") || !strings.Contains(stdout.String(), "memory-limits") {
		t.Fatalf("stdout = %q, want failing container and resource details", stdout.String())
	}
}

func TestApplyRecommendationsFailsBelowConfidenceThreshold(t *testing.T) {
	chartPath, valuesFile := copyTestChart(t)
	recommendationsFile := testRecommendationsFile(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		Container:           "keda-operator",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests",
		MinConfidence:       90,
		Format:              "json",
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code == 0 {
		t.Fatalf("runApply() exit code = %d, want non-zero", code)
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
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/kedify-agent",
		Namespace:           "keda",
		Container:           "manager",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-limits",
		MinConfidence:       20,
		Format:              "json",
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code == 0 {
		t.Fatalf("runApply() exit code = %d, want non-zero", code)
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
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/keda-operator",
		Namespace:           "keda",
		Container:           "keda-operator",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests",
		MinConfidence:       20,
		Format:              "json",
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code == 0 {
		t.Fatalf("runApply() exit code = %d, want non-zero", code)
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if !containsReasonCode(result.Reasons, "ambiguous") {
		t.Fatalf("reasons = %#v, want ambiguous", result.Reasons)
	}
}

func TestApplyRecommendationsPassesNamespaceToHelmTemplate(t *testing.T) {
	chartPath := filepath.Join(t.TempDir(), "chart")
	valuesFile := filepath.Join(chartPath, "values.yaml")
	recommendationsFile := writeRecommendationsFile(t, []map[string]any{
		{
			"kind":        "Deployment",
			"name":        "demo",
			"namespace":   "custom-ns",
			"resourceUID": "custom-ns/deployment/demo/demo/cpu-requests",
			"status":      "waiting",
			"labels": map[string]any{
				"workloadContainer": "demo",
				"originalValue":     "100m",
				"suggestedValue":    "200m",
				"confidence":        80,
			},
		},
	})

	writeTestChartFiles(t, chartPath, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: namespace-test\nversion: 0.1.0\n",
		"values.yaml": `deployment:
  name: demo
  containerName: demo
  resources:
    requests:
      cpu: 100m
`,
		"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Values.deployment.name }}
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    matchLabels:
      app: {{ .Values.deployment.name }}
  template:
    metadata:
      labels:
        app: {{ .Values.deployment.name }}
    spec:
      containers:
        - name: {{ .Values.deployment.containerName }}
          image: registry.k8s.io/pause:3.10
          resources:
{{ toYaml .Values.deployment.resources | indent 12 }}
`,
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runApply(t, &ApplyRecommendationsCmd{
		Target:              "deployment/demo",
		Namespace:           "custom-ns",
		ChartPath:           chartPath,
		ValuesFile:          valuesFile,
		RecommendationsFile: recommendationsFile,
		Resources:           "cpu-requests",
		MinConfidence:       20,
		Format:              "json",
		DryRun:              true,
	}, stdout, stderr)

	if err != nil {
		t.Fatalf("runApply() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("runApply() exit code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var result applyRecommendationsResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v, stdout = %q", err, stdout.String())
	}
	if result.Result != "planned" {
		t.Fatalf("result = %#v, want planned", result)
	}
	if len(result.Reasons) != 0 {
		t.Fatalf("reasons = %#v, want empty", result.Reasons)
	}
}

func runApply(t *testing.T, cmd *ApplyRecommendationsCmd, stdout, stderr *bytes.Buffer) (int, error) {
	t.Helper()

	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      stdout,
		Stderr:      stderr,
		WriteOutput: output.WriteOutput,
	}

	if err := cmd.Run(ctx); err != nil {
		var cmdErr *clierrors.CommandResultError
		if errors.As(err, &cmdErr) {
			if cmdErr.Payload != nil {
				if writeErr := output.WriteOutput(stdout, cmdErr.Payload, "json"); writeErr != nil {
					return 1, writeErr
				}
			}
			return cmdErr.ExitCode, nil
		}
		return 1, err
	}

	return 0, nil
}

func copyTestChart(t *testing.T) (string, string) {
	t.Helper()

	sourceRoot, err := filepath.Abs("../../../test/chart")
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

	path, err := filepath.Abs("../../../test/recommendations.json")
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	return path
}

func writeRecommendationsFile(t *testing.T, items []map[string]any) string {
	t.Helper()

	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "recommendations.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTestChartFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for relativePath, content := range files {
		path := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
}

func containsReasonCode(reasons []applyRecommendationReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
