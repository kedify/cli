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
