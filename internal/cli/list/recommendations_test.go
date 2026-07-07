package list

import (
	"bytes"
	"io"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/output"
	"github.com/kedify/cli/internal/service"
)

type fakeRecommendationCredentialsStore struct {
	creds service.Credentials
}

func (f *fakeRecommendationCredentialsStore) ReadCredentials() (service.Credentials, error) {
	return f.creds, nil
}

func (f *fakeRecommendationCredentialsStore) WriteCredentials(service.Credentials) error {
	return nil
}

type fakeRecommendationClusterService struct {
	recommendations any
	err             error
	lastURL         string
	lastToken       string
	lastID          string
}

func (f *fakeRecommendationClusterService) ListClusters(string, string) ([]map[string]any, error) {
	return nil, nil
}

func (f *fakeRecommendationClusterService) GetCluster(string, string, string) (map[string]any, error) {
	return nil, nil
}

func (f *fakeRecommendationClusterService) GetRecommendations(apiURL, token, clusterID string) (any, error) {
	f.lastURL = apiURL
	f.lastToken = token
	f.lastID = clusterID
	if f.err != nil {
		return nil, f.err
	}
	return f.recommendations, nil
}

func TestListRecommendationsCmdRunWritesRecommendations(t *testing.T) {
	store := &fakeRecommendationCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeRecommendationClusterService{
		recommendations: map[string]any{
			"items": []map[string]any{{"kind": "cpu"}},
		},
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

	id := "fc6af0dc-685b-4055-805d-0d3e0ead1596"
	if err := (&ListRecommendationsCmd{ClusterID: id, Output: "yaml"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	payload, ok := gotValue.(map[string]any)
	if !ok {
		t.Fatalf("got output value = %#v", gotValue)
	}
	if gotFormat != "yaml" {
		t.Fatalf("output format = %q, want %q", gotFormat, "yaml")
	}
	if payload["items"] == nil {
		t.Fatalf("payload = %#v", payload)
	}
	if clusterService.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", clusterService.lastToken, "override-token")
	}
	if clusterService.lastID != id {
		t.Fatalf("service lastID = %q, want %q", clusterService.lastID, id)
	}
}

func TestListRecommendationsCmdRunWritesResultsOnlyToStdout(t *testing.T) {
	store := &fakeRecommendationCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeRecommendationClusterService{
		recommendations: []map[string]any{{"kind": "cpu"}},
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

	if err := (&ListRecommendationsCmd{
		ClusterID: "fc6af0dc-685b-4055-805d-0d3e0ead1596",
		Output:    "json",
	}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.Len() == 0 {
		t.Fatal("stdout is empty, want rendered output")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
