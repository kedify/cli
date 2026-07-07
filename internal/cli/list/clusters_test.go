package list

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

func TestListClustersCmdRunWritesClusters(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{{"name": "alpha"}, {"name": "beta"}},
	}

	var gotValue any
	var gotFormat string
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		APIURL:      "https://api.dev.kedify.io/v1",
		Token:       "override-token",
		Client:      clusterService,
		Credentials: store,
		WriteOutput: func(_ io.Writer, value any, format string) error {
			gotValue = value
			gotFormat = format
			return nil
		},
	}

	cmd := &ListClustersCmd{Output: "yaml"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	clusters, ok := gotValue.([]map[string]any)
	if !ok || len(clusters) != 2 {
		t.Fatalf("got output value = %#v", gotValue)
	}
	if gotFormat != "yaml" {
		t.Fatalf("output format = %q, want %q", gotFormat, "yaml")
	}
	if clusterService.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", clusterService.lastToken, "override-token")
	}
}

func TestListClustersCmdRunWritesResultsOnlyToStdout(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{{"name": "alpha"}},
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
		WriteOutput: output.WriteOutput,
	}

	if err := (&ListClustersCmd{Output: "json"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.Len() == 0 {
		t.Fatal("stdout is empty, want rendered output")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
