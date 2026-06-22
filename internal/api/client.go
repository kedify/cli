package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

type Client struct {
	httpClient *http.Client
}

type paginatedResponse struct {
	Items    []any    `json:"items"`
	PageInfo pageInfo `json:"pageInfo"`
}

type pageInfo struct {
	HasNext bool `json:"hasNext"`
	Page    int  `json:"page"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) ListClusters(apiURL, token string) ([]map[string]any, error) {
	items, err := c.listPaginatedItems(apiURL, token, "/clusters")
	if err != nil {
		return nil, err
	}

	clusters := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cluster, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parse response items: unexpected item type %T", item)
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func (c *Client) GetCluster(apiURL, token, clusterID string) (map[string]any, error) {
	var payload map[string]any
	if err := c.getJSON(apiURL, token, "/clusters/"+url.PathEscape(clusterID), &payload); err != nil {
		return nil, fmt.Errorf("request cluster %s: %w", clusterID, err)
	}

	return payload, nil
}

func (c *Client) GetRecommendations(apiURL, token, clusterID string) (any, error) {
	path := "/clusters/" + url.PathEscape(clusterID) + "/recommendations"

	items, err := c.listPaginatedItems(apiURL, token, path)
	if err == nil {
		return items, nil
	}
	if !strings.Contains(err.Error(), "response is not paginated") {
		return nil, fmt.Errorf("request recommendations for cluster %s: %w", clusterID, err)
	}

	var payload any
	if err := c.getJSON(apiURL, token, path, &payload); err != nil {
		return nil, fmt.Errorf("request recommendations for cluster %s: %w", clusterID, err)
	}

	return payload, nil
}

func (c *Client) listPaginatedItems(apiURL, token, path string) ([]any, error) {
	var allItems []any
	page := 1

	for {
		response, err := c.listPage(apiURL, token, path, page)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, response.Items...)
		if !response.PageInfo.HasNext {
			break
		}

		nextPage := response.PageInfo.Page + 1
		if nextPage <= page {
			nextPage = page + 1
		}
		page = nextPage
	}

	return allItems, nil
}

func (c *Client) listPage(apiURL, token, path string, page int) (paginatedResponse, error) {
	requestURL, err := url.Parse(strings.TrimRight(apiURL, "/") + path)
	if err != nil {
		return paginatedResponse{}, fmt.Errorf("build request url: %w", err)
	}

	if page > 1 {
		query := requestURL.Query()
		query.Set("page", fmt.Sprintf("%d", page))
		requestURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return paginatedResponse{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return paginatedResponse{}, fmt.Errorf("request page %d for %s: %w", page, path, err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return paginatedResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return paginatedResponse{}, fmt.Errorf("request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload paginatedResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return paginatedResponse{}, fmt.Errorf("parse response as json: %w", err)
	}

	if payload.Items == nil || payload.PageInfo.Page == 0 {
		return paginatedResponse{}, fmt.Errorf("response is not paginated")
	}

	return payload, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}

	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response body: %w", err)
	}

	return body, nil
}

func (c *Client) getJSON(apiURL, token, path string, target any) error {
	requestURL := strings.TrimRight(apiURL, "/") + path

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("parse response as json: %w", err)
	}

	return nil
}
