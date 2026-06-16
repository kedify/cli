package cli

import (
	"bytes"
	"io"
	"testing"
)

type fakeCredentialsStore struct {
	creds    credentials
	readErr  error
	writeErr error
	wrote    credentials
}

func (f *fakeCredentialsStore) ReadCredentials() (credentials, error) {
	if f.readErr != nil {
		return credentials{}, f.readErr
	}
	return f.creds, nil
}

func (f *fakeCredentialsStore) WriteCredentials(creds credentials) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.wrote = creds
	return nil
}

type fakeClusterService struct {
	clusters  []map[string]any
	cluster   map[string]any
	err       error
	lastURL   string
	lastToken string
	lastID    string
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

func TestLoginCmdRunStoresCredentials(t *testing.T) {
	store := &fakeCredentialsStore{}
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		credentials: store,
		readSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			return "secret-token", nil
		},
	}

	if err := (&LoginCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "secret-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "secret-token")
	}
}

func TestLoginCmdRunUsesExplicitToken(t *testing.T) {
	store := &fakeCredentialsStore{}
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		credentials: store,
		readSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			t.Fatal("readSecret should not be called when token is provided explicitly")
			return "", nil
		},
	}

	if err := (&LoginCmd{Token: "arg-token"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "arg-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "arg-token")
	}
}

func TestListClustersCmdRunWritesClusters(t *testing.T) {
	store := &fakeCredentialsStore{creds: credentials{Token: "stored-token"}}
	service := &fakeClusterService{
		clusters: []map[string]any{{"name": "alpha"}, {"name": "beta"}},
	}

	var gotValue any
	var gotFormat string
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		apiURL:      "https://api.dev.kedify.io/v1",
		token:       "override-token",
		client:      service,
		credentials: store,
		writeOutput: func(_ io.Writer, value any, format string) error {
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
	if service.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", service.lastToken, "override-token")
	}
}

func TestGetClusterCmdRunFindsNamedCluster(t *testing.T) {
	store := &fakeCredentialsStore{creds: credentials{Token: "stored-token"}}
	service := &fakeClusterService{
		clusters: []map[string]any{
			{"id": "1", "name": "alpha"},
			{"id": "2", "name": "beta"},
		},
	}

	var gotValue any
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		apiURL:      "https://api.dev.kedify.io/v1",
		token:       "override-token",
		client:      service,
		credentials: store,
		selectCluster: func(_ io.Reader, _ io.Writer, _ []map[string]any) (map[string]any, error) {
			t.Fatal("selector should not be called when name is provided")
			return nil, nil
		},
		writeOutput: func(_ io.Writer, value any, _ string) error {
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
	if service.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", service.lastToken, "override-token")
	}
}

func TestGetClusterCmdRunUsesDedicatedEndpointForUUID(t *testing.T) {
	store := &fakeCredentialsStore{creds: credentials{Token: "stored-token"}}
	service := &fakeClusterService{
		cluster: map[string]any{
			"id":   "fc6af0dc-685b-4055-805d-0d3e0ead1596",
			"name": "alpha",
		},
	}

	var gotValue any
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		apiURL:      "https://api.dev.kedify.io/v1",
		token:       "override-token",
		client:      service,
		credentials: store,
		selectCluster: func(_ io.Reader, _ io.Writer, _ []map[string]any) (map[string]any, error) {
			t.Fatal("selector should not be called when uuid is provided")
			return nil, nil
		},
		writeOutput: func(_ io.Writer, value any, _ string) error {
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
	if service.lastID != id {
		t.Fatalf("service lastID = %q, want %q", service.lastID, id)
	}
}

func TestGetClusterCmdRunUsesSelectorWhenNameMissing(t *testing.T) {
	store := &fakeCredentialsStore{creds: credentials{Token: "stored-token"}}
	service := &fakeClusterService{
		clusters: []map[string]any{{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha"}},
		cluster:  map[string]any{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha", "agentStatus": "connected"},
	}
	selected := map[string]any{"id": "fc6af0dc-685b-4055-805d-0d3e0ead1596", "name": "alpha"}

	var gotValue any
	ctx := &context{
		stdin:       bytes.NewBuffer(nil),
		stdout:      &bytes.Buffer{},
		stderr:      &bytes.Buffer{},
		apiURL:      "https://api.dev.kedify.io/v1",
		token:       "override-token",
		client:      service,
		credentials: store,
		selectCluster: func(_ io.Reader, _ io.Writer, clusters []map[string]any) (map[string]any, error) {
			if len(clusters) != 1 {
				t.Fatalf("selector clusters len = %d, want 1", len(clusters))
			}
			return selected, nil
		},
		writeOutput: func(_ io.Writer, value any, _ string) error {
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
	if service.lastToken != "override-token" {
		t.Fatalf("service token = %q, want %q", service.lastToken, "override-token")
	}
	if service.lastID != "fc6af0dc-685b-4055-805d-0d3e0ead1596" {
		t.Fatalf("service lastID = %q", service.lastID)
	}
}

func TestFindClusterReturnsErrorWhenMissing(t *testing.T) {
	_, err := findCluster([]map[string]any{{"name": "alpha"}}, "beta")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveTokenFallsBackToCredentials(t *testing.T) {
	ctx := &context{
		credentials: &fakeCredentialsStore{creds: credentials{Token: "stored-token"}},
	}

	token, err := resolveToken(ctx)
	if err != nil {
		t.Fatalf("resolveToken() error = %v", err)
	}
	if token != "stored-token" {
		t.Fatalf("token = %q, want %q", token, "stored-token")
	}
}

func TestResolveTokenPrefersContextToken(t *testing.T) {
	ctx := &context{
		token:       "override-token",
		credentials: &fakeCredentialsStore{creds: credentials{Token: "stored-token"}},
	}

	token, err := resolveToken(ctx)
	if err != nil {
		t.Fatalf("resolveToken() error = %v", err)
	}
	if token != "override-token" {
		t.Fatalf("token = %q, want %q", token, "override-token")
	}
}
