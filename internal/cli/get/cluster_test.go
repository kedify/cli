package get

import (
	"bytes"
	"io"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/output"
	"github.com/kedify/cli/internal/service"
)

type fakeCredentialsStore struct {
	creds service.Credentials
}

func (f *fakeCredentialsStore) ReadCredentials() (service.Credentials, error) {
	return f.creds, nil
}

func (f *fakeCredentialsStore) WriteCredentials(service.Credentials) error {
	return nil
}

type fakeClusterService struct {
	clusters        []map[string]any
	cluster         map[string]any
	recommendations any
	err             error
	lastURL         string
	lastToken       string
	lastID          string
}

func (f *fakeClusterService) ListClusters(apiURL, token string) ([]map[string]any, error) {
	f.lastURL = apiURL
	f.lastToken = token
	if f.err != nil {
		return nil, f.err
	}
	return f.clusters, nil
}

func (f *fakeClusterService) GetCluster(apiURL, token, clusterID string) (map[string]any, error) {
	f.lastURL = apiURL
	f.lastToken = token
	f.lastID = clusterID
	if f.err != nil {
		return nil, f.err
	}
	return f.cluster, nil
}

func (f *fakeClusterService) GetRecommendations(apiURL, token, clusterID string) (any, error) {
	f.lastURL = apiURL
	f.lastToken = token
	f.lastID = clusterID
	if f.err != nil {
		return nil, f.err
	}
	return f.recommendations, nil
}

func TestGetClusterCmdRunFindsNamedCluster(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{
			{"id": "1", "name": "alpha"},
			{"id": "2", "name": "beta"},
		},
	}

	var gotValue any
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		APIURL:      "https://api.dev.kedify.io/v1",
		Token:       "override-token",
		Client:      clusterService,
		Credentials: store,
		SelectCluster: func(_ io.Reader, _ io.Writer, _ io.Writer, _ []map[string]any) (map[string]any, error) {
			t.Fatal("SelectCluster should not be called when name is provided")
			return nil, nil
		},
		WriteOutput: func(_ io.Writer, value any, _ string) error {
			gotValue = value
			return nil
		},
	}

	cmd := &GetClusterCmd{Name: "beta", Output: "json"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cluster, ok := gotValue.(map[string]any)
	if !ok {
		t.Fatalf("got output value = %#v", gotValue)
	}
	if cluster["id"] != "2" {
		t.Fatalf("selected cluster = %#v", cluster)
	}
	if clusterService.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", clusterService.lastToken, "override-token")
	}
}

func TestGetClusterCmdRunWritesResultsOnlyToStdout(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{
			{"id": "2", "name": "beta"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      stdout,
		Stderr:      stderr,
		APIURL:      "https://api.dev.kedify.io/v1",
		Token:       "override-token",
		Client:      clusterService,
		Credentials: store,
		SelectCluster: func(_ io.Reader, _ io.Writer, _ io.Writer, _ []map[string]any) (map[string]any, error) {
			t.Fatal("SelectCluster should not be called when name is provided")
			return nil, nil
		},
		WriteOutput: output.WriteOutput,
	}

	if err := (&GetClusterCmd{Name: "beta", Output: "json"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.Len() == 0 {
		t.Fatal("stdout is empty, want rendered output")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestGetClusterCmdRunUsesDedicatedEndpointForUUID(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		cluster: map[string]any{
			"id":   "fc6af0dc-685b-4055-805d-0d3e0ead1596",
			"name": "alpha",
		},
	}

	var gotValue any
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		APIURL:      "https://api.dev.kedify.io/v1",
		Token:       "override-token",
		Client:      clusterService,
		Credentials: store,
		SelectCluster: func(_ io.Reader, _ io.Writer, _ io.Writer, _ []map[string]any) (map[string]any, error) {
			t.Fatal("SelectCluster should not be called when UUID is provided")
			return nil, nil
		},
		WriteOutput: func(_ io.Writer, value any, _ string) error {
			gotValue = value
			return nil
		},
	}

	id := "fc6af0dc-685b-4055-805d-0d3e0ead1596"
	cmd := &GetClusterCmd{Name: id, Output: "json"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cluster, ok := gotValue.(map[string]any)
	if !ok || cluster["id"] != id {
		t.Fatalf("got output value = %#v", gotValue)
	}
	if clusterService.lastID != id {
		t.Fatalf("service lastID = %q, want %q", clusterService.lastID, id)
	}
}

func TestGetClusterCmdRunUsesSelectorWhenNameMissing(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha"}},
		cluster:  map[string]any{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha", "agentStatus": "connected"},
	}
	selected := map[string]any{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha"}

	var gotValue any
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		APIURL:      "https://api.dev.kedify.io/v1",
		Token:       "override-token",
		Client:      clusterService,
		Credentials: store,
		SelectCluster: func(_ io.Reader, _ io.Writer, _ io.Writer, clusters []map[string]any) (map[string]any, error) {
			if len(clusters) != 1 {
				t.Fatalf("selector clusters len = %d, want 1", len(clusters))
			}
			return selected, nil
		},
		WriteOutput: func(_ io.Writer, value any, _ string) error {
			gotValue = value
			return nil
		},
	}

	if err := (&GetClusterCmd{Output: "json"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cluster, ok := gotValue.(map[string]any)
	if !ok {
		t.Fatalf("got output value = %#v", gotValue)
	}
	if cluster["id"] != selected["id"] {
		t.Fatalf("got output value = %#v, want selected cluster", gotValue)
	}
	if clusterService.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", clusterService.lastToken, "override-token")
	}
	if clusterService.lastID != "fc6af0dc-685b-4055-805d-0d3e0ead1596" {
		t.Fatalf("service lastID = %q", clusterService.lastID)
	}
}

func TestFindClusterReturnsErrorWhenMissing(t *testing.T) {
	_, err := findCluster([]map[string]any{{"name": "alpha"}}, "beta")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
