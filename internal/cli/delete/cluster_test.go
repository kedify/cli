package delete

import (
	"bytes"
	"io"
	"strings"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
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
	deletedIDs      []string
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

func (f *fakeClusterService) DeleteCluster(apiURL, token, clusterID string) error {
	f.lastURL = apiURL
	f.lastToken = token
	f.lastID = clusterID
	f.deletedIDs = append(f.deletedIDs, clusterID)
	if f.err != nil {
		return f.err
	}
	return nil
}

func TestDeleteClusterCmdRunDeletesNamedCluster(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{
			{"id": "1", "name": "alpha"},
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
	}

	if err := (&DeleteClusterCmd{Name: "beta"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(clusterService.deletedIDs) != 1 || clusterService.deletedIDs[0] != "2" {
		t.Fatalf("deleted IDs = %#v", clusterService.deletedIDs)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `Deleted cluster "beta" (2).`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDeleteClusterCmdRunDeletesUUIDDirectly(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		cluster: map[string]any{
			"id":   "fc6af0dc-685b-4055-805d-0d3e0ead1596",
			"name": "alpha",
		},
	}

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
	}

	id := "fc6af0dc-685b-4055-805d-0d3e0ead1596"
	if err := (&DeleteClusterCmd{Name: id}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(clusterService.deletedIDs) != 1 || clusterService.deletedIDs[0] != id {
		t.Fatalf("deleted IDs = %#v", clusterService.deletedIDs)
	}
}

func TestDeleteClusterCmdRunUsesSelectorWhenNameMissing(t *testing.T) {
	store := &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	clusterService := &fakeClusterService{
		clusters: []map[string]any{{"id": "2", "name": "beta"}},
	}

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
			return clusters[0], nil
		},
	}

	if err := (&DeleteClusterCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(clusterService.deletedIDs) != 1 || clusterService.deletedIDs[0] != "2" {
		t.Fatalf("deleted IDs = %#v", clusterService.deletedIDs)
	}
}
