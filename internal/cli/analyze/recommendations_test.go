package analyze

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	clierrors "github.com/kedify/cli/internal/errors"
)

const (
	helperModeEnv        = "KEDIFY_TEST_ANALYZER_MODE"
	helperExpectedEnv    = "KEDIFY_TEST_ANALYZER_REQUEST"
	helperExitInternal   = 1
	helperExitInvalid    = 2
	validSnapshotRequest = `{"protocolVersion":"kedify-analyzer/v1","input":{"schemaVersion":"resource-analysis-input/v1","observedIntervalHours":1,"containers":[]},"policy":{}}`
	validAnalyzerOutput  = `{"protocolVersion":"kedify-analyzer/v1","analyzerVersion":"test","engineVersion":"1","inputSchemaVersion":"resource-analysis-input/v1","outputSchemaVersion":"resource-analysis-output/v1","output":{"schemaVersion":"resource-analysis-output/v1","detectorVersion":"1"}}` + "\n"
)

func TestMain(m *testing.M) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		os.Exit(m.Run())
	}

	request, err := io.ReadAll(os.Stdin)
	if err != nil || string(request) != os.Getenv(helperExpectedEnv) {
		_, _ = fmt.Fprintln(os.Stderr, "fake analyzer received an unexpected request")
		os.Exit(helperExitInvalid)
	}

	switch mode {
	case "success":
		if _, err := io.WriteString(os.Stdout, validAnalyzerOutput); err != nil {
			os.Exit(helperExitInternal)
		}
		os.Exit(0)
	case "exit-1", "exit-2":
		exitCode, _ := strconv.Atoi(strings.TrimPrefix(mode, "exit-"))
		_, _ = fmt.Fprintf(os.Stderr, "fake analyzer failed with code %d\n", exitCode)
		os.Exit(exitCode)
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unknown fake analyzer mode")
		os.Exit(helperExitInternal)
	}
}

func TestRecommendationsRunsAnalyzerWithStdinSnapshot(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := runWithFakeAnalyzer(t, "success", bytes.NewBufferString(validSnapshotRequest), stdout, stderr)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != validAnalyzerOutput {
		t.Fatalf("stdout = %q, want %q", stdout.String(), validAnalyzerOutput)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRecommendationsPreservesAnalyzerExitCodesAndStderr(t *testing.T) {
	for _, exitCode := range []int{helperExitInternal, helperExitInvalid} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			err := runWithFakeAnalyzer(t, fmt.Sprintf("exit-%d", exitCode), bytes.NewBufferString(validSnapshotRequest), stdout, stderr)
			var resultError *clierrors.CommandResultError
			if !errors.As(err, &resultError) || resultError.ExitCode != exitCode {
				t.Fatalf("Run() error = %#v, want command exit code %d", err, exitCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), fmt.Sprintf("failed with code %d", exitCode)) {
				t.Fatalf("stderr = %q, want analyzer diagnostic", stderr.String())
			}
		})
	}
}

func TestRecommendationsReportsShortStdoutWrite(t *testing.T) {
	err := runWithFakeAnalyzer(t, "success", bytes.NewBufferString(validSnapshotRequest), shortWriter{}, &bytes.Buffer{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Run() error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestRequestValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "malformed", data: `{`, want: "invalid snapshot request JSON"},
		{name: "protocol", data: `{"protocolVersion":"kedify-analyzer/v2","input":{"schemaVersion":"resource-analysis-input/v1"}}`, want: `requires "kedify-analyzer/v1"`},
		{name: "input schema", data: `{"protocolVersion":"kedify-analyzer/v1","input":{"schemaVersion":"resource-analysis-input/v2"}}`, want: `requires "resource-analysis-input/v1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRequest([]byte(test.data)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateRequest() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestResponseValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*responseMetadata)
		want   string
	}{
		{name: "protocol", mutate: func(response *responseMetadata) { response.ProtocolVersion = "kedify-analyzer/v2" }, want: `expected "kedify-analyzer/v1"`},
		{name: "analyzer version", mutate: func(response *responseMetadata) { response.AnalyzerVersion = "" }, want: "missing analyzerVersion"},
		{name: "engine", mutate: func(response *responseMetadata) { response.EngineVersion = "2" }, want: `expected "1"`},
		{name: "input schema", mutate: func(response *responseMetadata) { response.InputSchemaVersion = "resource-analysis-input/v2" }, want: `expected "resource-analysis-input/v1"`},
		{name: "output schema", mutate: func(response *responseMetadata) { response.OutputSchemaVersion = "resource-analysis-output/v2" }, want: `expected "resource-analysis-output/v1"`},
		{name: "missing output", mutate: func(response *responseMetadata) { response.Output = nil }, want: "missing output"},
		{name: "nested output schema", mutate: func(response *responseMetadata) { response.Output.SchemaVersion = "resource-analysis-output/v2" }, want: `expected "resource-analysis-output/v1"`},
		{name: "detector", mutate: func(response *responseMetadata) { response.Output.DetectorVersion = "2" }, want: `expected "1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := validResponseMetadata()
			test.mutate(&response)
			data, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateResponse(data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateResponse() error = %v, want substring %q", err, test.want)
			}
		})
	}

	if err := validateResponse([]byte(`{`)); err == nil || !strings.Contains(err.Error(), "invalid JSON response") {
		t.Fatalf("validateResponse() malformed JSON error = %v", err)
	}
}

func TestReadSnapshotSupportsFilesAndRejectsOversizedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte(validSnapshotRequest), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSnapshot(path, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("readSnapshot() error = %v", err)
	}
	if string(got) != validSnapshotRequest {
		t.Fatalf("readSnapshot() = %q, want request", got)
	}

	_, err = readSnapshot("-", bytes.NewReader(bytes.Repeat([]byte(" "), maxSnapshotBytes+1)))
	if err == nil || !strings.Contains(err.Error(), "request exceeds 16777216-byte limit") {
		t.Fatalf("readSnapshot() oversized error = %v", err)
	}
}

func TestDiscoverAnalyzerOrder(t *testing.T) {
	t.Run("explicit relative file", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		name := "custom-analyzer"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		writeExecutable(t, name)
		got, err := discoverAnalyzer(name, "", func(string) (string, error) {
			t.Fatal("PATH discovery must not run for an explicit analyzer")
			return "", nil
		})
		if err != nil {
			t.Fatalf("discoverAnalyzer() error = %v", err)
		}
		want, _ := filepath.Abs(name)
		if got != want {
			t.Fatalf("discoverAnalyzer() = %q, want absolute path %q", got, want)
		}
	})

	t.Run("sibling before PATH", func(t *testing.T) {
		dir := t.TempDir()
		sibling := filepath.Join(dir, analyzerName())
		writeExecutable(t, sibling)
		got, err := discoverAnalyzer("", filepath.Join(dir, "kedify"), func(string) (string, error) {
			t.Fatal("PATH discovery must not run when a sibling analyzer exists")
			return "", nil
		})
		if err != nil || got != sibling {
			t.Fatalf("discoverAnalyzer() = %q, %v; want %q", got, err, sibling)
		}
	})

	t.Run("PATH fallback", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), analyzerName())
		got, err := discoverAnalyzer("", "", func(name string) (string, error) {
			if name != analyzerName() {
				t.Fatalf("LookPath(%q), want %q", name, analyzerName())
			}
			return path, nil
		})
		if err != nil || got != path {
			t.Fatalf("discoverAnalyzer() = %q, %v; want %q", got, err, path)
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, err := discoverAnalyzer("", "", func(string) (string, error) {
			return "", errors.New("not found")
		})
		if err == nil || !strings.Contains(err.Error(), "install a matching analyzer") || !strings.Contains(err.Error(), "--analyzer") {
			t.Fatalf("discoverAnalyzer() error = %v", err)
		}
	})
}

func runWithFakeAnalyzer(t *testing.T, mode string, stdin io.Reader, stdout, stderr io.Writer) error {
	t.Helper()
	t.Setenv(helperModeEnv, mode)
	t.Setenv(helperExpectedEnv, validSnapshotRequest)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return (&RecommendationsCmd{Snapshot: "-", Analyzer: executable}).Run(&clictx.Context{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
}

func validResponseMetadata() responseMetadata {
	return responseMetadata{
		ProtocolVersion:     analyzerProtocolVersion,
		AnalyzerVersion:     "test",
		EngineVersion:       engineVersion,
		InputSchemaVersion:  inputSchemaVersion,
		OutputSchemaVersion: outputSchemaVersion,
		Output: &outputMetadata{
			SchemaVersion:   outputSchemaVersion,
			DetectorVersion: engineVersion,
		},
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	return len(data) / 2, nil
}
