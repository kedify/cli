package analyze

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kedify/recommender/analysis"

	clictx "github.com/kedify/cli/internal/cli/context"
)

const maxSnapshotBytes = 16 << 20

type RecommendationsCmd struct {
	Snapshot string `arg:"" name:"snapshot-request" help:"Path to a normalized snapshot request, or - for stdin."`
}

type snapshotRequest struct {
	Input  analysis.Input  `json:"input"`
	Policy analysis.Policy `json:"policy"`
}

func (c *RecommendationsCmd) Run(ctx *clictx.Context) error {
	data, err := readSnapshot(c.Snapshot, ctx.Stdin)
	if err != nil {
		return err
	}

	request, err := decodeSnapshot(data)
	if err != nil {
		return err
	}

	result, err := analysis.Analyze(request.Input, request.Policy)
	if err != nil {
		return fmt.Errorf("analyze snapshot: %w", err)
	}

	if err := json.NewEncoder(ctx.Stdout).Encode(result); err != nil {
		return fmt.Errorf("write analysis result: %w", err)
	}
	return nil
}

func readSnapshot(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		data, err := readSnapshotFrom(stdin)
		if err != nil {
			return nil, fmt.Errorf("read snapshot request from stdin: %w", err)
		}
		return data, nil
	}

	file, err := os.Open(filepath.Clean(path)) // #nosec G304 -- the snapshot path is explicit user input.
	if err != nil {
		return nil, fmt.Errorf("read snapshot request %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	data, err := readSnapshotFrom(file)
	if err != nil {
		return nil, fmt.Errorf("read snapshot request %q: %w", path, err)
	}
	return data, nil
}

func readSnapshotFrom(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSnapshotBytes {
		return nil, fmt.Errorf("request exceeds %d-byte limit", maxSnapshotBytes)
	}
	return data, nil
}

func decodeSnapshot(data []byte) (snapshotRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var request snapshotRequest
	if err := decoder.Decode(&request); err != nil {
		return snapshotRequest{}, fmt.Errorf("invalid snapshot request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return snapshotRequest{}, fmt.Errorf("invalid snapshot request: expected one JSON object")
	}
	return request, nil
}
