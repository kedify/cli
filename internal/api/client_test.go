package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestListClustersReadsAllPages(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		page := r.URL.Query().Get("page")
		var body string
		switch page {
		case "":
			body = `{"items":[{"name":"alpha"}],"pageInfo":{"hasNext":true,"page":1}}`
		case "2":
			body = `{"items":[{"name":"beta"}],"pageInfo":{"hasNext":false,"page":2}}`
		default:
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader("unexpected page")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}}

	clusters, err := client.ListClusters("https://api.dev.kedify.io/v1", "token")
	if err != nil {
		t.Fatalf("ListClusters() error = %v", err)
	}

	if len(clusters) != 2 {
		t.Fatalf("len(clusters) = %d, want 2", len(clusters))
	}
}

func TestListClustersReturnsHTTPError(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     fmt.Sprintf("%d %s", http.StatusBadRequest, http.StatusText(http.StatusBadRequest)),
			Body:       io.NopCloser(strings.NewReader("bad request")),
			Header:     make(http.Header),
		}, nil
	})}}

	_, err := client.ListClusters("https://api.dev.kedify.io/v1", "token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetClusterCallsDedicatedEndpoint(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/clusters/fc6af0dc-685b-4055-805d-0d3e0ead1596" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"id":"fc6af0dc-685b-4055-805d-0d3e0ead1596","name":"alpha"}`)),
			Header:     make(http.Header),
		}, nil
	})}}

	cluster, err := client.GetCluster("https://api.dev.kedify.io/v1", "token", "fc6af0dc-685b-4055-805d-0d3e0ead1596")
	if err != nil {
		t.Fatalf("GetCluster() error = %v", err)
	}

	if cluster["name"] != "alpha" {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestGetRecommendationsCallsDedicatedEndpoint(t *testing.T) {
	requests := 0
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/v1/clusters/fc6af0dc-685b-4055-805d-0d3e0ead1596/recommendations" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`[{"kind":"cpu"}]`)),
			Header:     make(http.Header),
		}, nil
	})}}

	recommendations, err := client.GetRecommendations("https://api.dev.kedify.io/v1", "token", "fc6af0dc-685b-4055-805d-0d3e0ead1596")
	if err != nil {
		t.Fatalf("GetRecommendations() error = %v", err)
	}

	items, ok := recommendations.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("recommendations = %#v", recommendations)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestGetRecommendationsReadsAllPages(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/clusters/fc6af0dc-685b-4055-805d-0d3e0ead1596/recommendations" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		page := r.URL.Query().Get("page")
		var body string
		switch page {
		case "":
			body = `{"items":[{"kind":"cpu"}],"pageInfo":{"hasNext":true,"page":1}}`
		case "2":
			body = `{"items":[{"kind":"memory"}],"pageInfo":{"hasNext":false,"page":2}}`
		default:
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader("unexpected page")),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}}

	recommendations, err := client.GetRecommendations("https://api.dev.kedify.io/v1", "token", "fc6af0dc-685b-4055-805d-0d3e0ead1596")
	if err != nil {
		t.Fatalf("GetRecommendations() error = %v", err)
	}

	items, ok := recommendations.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("recommendations = %#v", recommendations)
	}
}

func TestGetRecommendationsFallsBackToNonPaginatedPayload(t *testing.T) {
	requests := 0
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/v1/clusters/fc6af0dc-685b-4055-805d-0d3e0ead1596/recommendations" {
			t.Fatalf("path = %q", r.URL.Path)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"summary":{"count":1}}`)),
			Header:     make(http.Header),
		}, nil
	})}}

	recommendations, err := client.GetRecommendations("https://api.dev.kedify.io/v1", "token", "fc6af0dc-685b-4055-805d-0d3e0ead1596")
	if err != nil {
		t.Fatalf("GetRecommendations() error = %v", err)
	}

	payload, ok := recommendations.(map[string]any)
	if !ok || payload["summary"] == nil {
		t.Fatalf("recommendations = %#v", recommendations)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}
