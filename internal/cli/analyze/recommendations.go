package analyze

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	clictx "github.com/kedify/cli/internal/cli/context"
	clierrors "github.com/kedify/cli/internal/errors"
)

const (
	analyzerProtocolVersion  = "kedify-analyzer/v1"
	inputSchemaVersion       = "resource-analysis-input/v1"
	outputSchemaVersion      = "resource-analysis-output/v1"
	engineVersion            = "1"
	maxSnapshotBytes         = 16 << 20
	maxAnalyzerResponseBytes = 64 << 20
)

type RecommendationsCmd struct {
	Snapshot string `arg:"" name:"snapshot-request" help:"Path to a versioned normalized snapshot request, or - for stdin."`
	Analyzer string `name:"analyzer" help:"Path to the kedify-analyzer executable. Overrides sibling and PATH discovery." placeholder:"PATH"`
}

type requestMetadata struct {
	ProtocolVersion string `json:"protocolVersion"`
	Input           struct {
		SchemaVersion string `json:"schemaVersion"`
	} `json:"input"`
}

type responseMetadata struct {
	ProtocolVersion     string          `json:"protocolVersion"`
	AnalyzerVersion     string          `json:"analyzerVersion"`
	EngineVersion       string          `json:"engineVersion"`
	InputSchemaVersion  string          `json:"inputSchemaVersion"`
	OutputSchemaVersion string          `json:"outputSchemaVersion"`
	Output              *outputMetadata `json:"output"`
}

type outputMetadata struct {
	SchemaVersion   string `json:"schemaVersion"`
	DetectorVersion string `json:"detectorVersion"`
}

func (c *RecommendationsCmd) Run(ctx *clictx.Context) error {
	request, err := readSnapshot(c.Snapshot, ctx.Stdin)
	if err != nil {
		return err
	}
	if err := validateRequest(request); err != nil {
		return err
	}

	cliExecutable, _ := os.Executable()
	analyzer, err := discoverAnalyzer(c.Analyzer, cliExecutable, exec.LookPath)
	if err != nil {
		return err
	}

	command := exec.Command(analyzer) // #nosec G204 -- the executable is selected explicitly or from trusted local discovery; no shell is used.
	command.Stdin = bytes.NewReader(request)
	command.Stderr = ctx.Stderr
	response := cappedBuffer{limit: maxAnalyzerResponseBytes}
	command.Stdout = &response
	runErr := command.Run()
	if response.exceeded {
		return fmt.Errorf("analyzer response exceeds %d-byte limit", maxAnalyzerResponseBytes)
	}
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			return &clierrors.CommandResultError{ExitCode: exitError.ExitCode()}
		}
		return fmt.Errorf("run analyzer %q: %w", analyzer, runErr)
	}

	if err := validateResponse(response.Bytes()); err != nil {
		return fmt.Errorf("incompatible analyzer %q: %w", analyzer, err)
	}
	if _, err := io.Copy(ctx.Stdout, bytes.NewReader(response.Bytes())); err != nil {
		return fmt.Errorf("write analyzer result: %w", err)
	}
	return nil
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining > len(data) {
		remaining = len(data)
	}
	if remaining > 0 {
		_, _ = b.buffer.Write(data[:remaining])
	}
	if remaining < len(data) {
		b.exceeded = true
	}
	return len(data), nil
}

func (b *cappedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

func readSnapshot(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		request, err := readSnapshotFrom(stdin)
		if err != nil {
			return nil, fmt.Errorf("read snapshot request from stdin: %w", err)
		}
		return request, nil
	}

	file, err := os.Open(filepath.Clean(path)) // #nosec G304 -- the snapshot path is explicit user input.
	if err != nil {
		return nil, fmt.Errorf("read snapshot request %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	request, err := readSnapshotFrom(file)
	if err != nil {
		return nil, fmt.Errorf("read snapshot request %q: %w", path, err)
	}
	return request, nil
}

func readSnapshotFrom(reader io.Reader) ([]byte, error) {
	request, err := io.ReadAll(io.LimitReader(reader, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(request) > maxSnapshotBytes {
		return nil, fmt.Errorf("request exceeds %d-byte limit", maxSnapshotBytes)
	}
	return request, nil
}

func validateRequest(data []byte) error {
	var metadata requestMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("invalid snapshot request JSON: %w", err)
	}
	if metadata.ProtocolVersion != analyzerProtocolVersion {
		return fmt.Errorf("snapshot request uses protocolVersion %q; this CLI requires %q", metadata.ProtocolVersion, analyzerProtocolVersion)
	}
	if metadata.Input.SchemaVersion != inputSchemaVersion {
		return fmt.Errorf("snapshot request uses input schema %q; this CLI requires %q", metadata.Input.SchemaVersion, inputSchemaVersion)
	}
	return nil
}

func validateResponse(data []byte) error {
	var metadata responseMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	if metadata.ProtocolVersion != analyzerProtocolVersion {
		return fmt.Errorf("unsupported protocolVersion %q; expected %q", metadata.ProtocolVersion, analyzerProtocolVersion)
	}
	if strings.TrimSpace(metadata.AnalyzerVersion) == "" {
		return errors.New("response is missing analyzerVersion")
	}
	if metadata.EngineVersion != engineVersion {
		return fmt.Errorf("unsupported engineVersion %q; expected %q", metadata.EngineVersion, engineVersion)
	}
	if metadata.InputSchemaVersion != inputSchemaVersion {
		return fmt.Errorf("unsupported inputSchemaVersion %q; expected %q", metadata.InputSchemaVersion, inputSchemaVersion)
	}
	if metadata.OutputSchemaVersion != outputSchemaVersion {
		return fmt.Errorf("unsupported outputSchemaVersion %q; expected %q", metadata.OutputSchemaVersion, outputSchemaVersion)
	}
	if metadata.Output == nil {
		return errors.New("response is missing output")
	}
	if metadata.Output.SchemaVersion != outputSchemaVersion {
		return fmt.Errorf("output uses schemaVersion %q; expected %q", metadata.Output.SchemaVersion, outputSchemaVersion)
	}
	if metadata.Output.DetectorVersion != engineVersion {
		return fmt.Errorf("output uses detectorVersion %q; expected %q", metadata.Output.DetectorVersion, engineVersion)
	}
	return nil
}

func discoverAnalyzer(override, cliExecutable string, lookPath func(string) (string, error)) (string, error) {
	if strings.TrimSpace(override) != "" {
		path, err := filepath.Abs(filepath.Clean(override))
		if err != nil {
			return "", fmt.Errorf("resolve --analyzer %q: %w", override, err)
		}
		if err := validateExecutable(path); err != nil {
			return "", fmt.Errorf("--analyzer %q is not executable: %w", override, err)
		}
		return path, nil
	}

	name := analyzerName()
	if cliExecutable != "" {
		sibling := filepath.Join(filepath.Dir(cliExecutable), name)
		if err := validateExecutable(sibling); err == nil {
			return sibling, nil
		}
	}

	path, err := lookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found; install a matching analyzer beside kedify, add it to PATH, or pass --analyzer", name)
	}
	return path, nil
}

func analyzerName() string {
	if runtime.GOOS == "windows" {
		return "kedify-analyzer.exe"
	}
	return "kedify-analyzer"
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return errors.New("execute permission is not set")
	}
	return nil
}
