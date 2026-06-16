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

type clustersResponse struct {
	Items    []map[string]any `json:"items"`
	PageInfo pageInfo         `json:"pageInfo"`
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
	var allItems []map[string]any
	page := 1

	for {
		response, err := c.listClustersPage(apiURL, token, page)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, response.Items...)
		if !response.PageInfo.HasNext {
			break
		}

		page = response.PageInfo.Page + 1
		if page <= 1 {
			page++
		}
	}

	return allItems, nil
}

func (c *Client) GetCluster(apiURL, token, clusterID string) (map[string]any, error) {
	requestURL := strings.TrimRight(apiURL, "/") + "/clusters/" + url.PathEscape(clusterID)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request cluster %s: %w", clusterID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse response as json: %w", err)
	}

	return payload, nil
}

func (c *Client) listClustersPage(apiURL, token string, page int) (clustersResponse, error) {
	requestURL, err := url.Parse(strings.TrimRight(apiURL, "/") + "/clusters")
	if err != nil {
		return clustersResponse{}, fmt.Errorf("build request url: %w", err)
	}

	if page > 1 {
		query := requestURL.Query()
		query.Set("page", fmt.Sprintf("%d", page))
		requestURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return clustersResponse{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return clustersResponse{}, fmt.Errorf("request clusters page %d: %w", page, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return clustersResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return clustersResponse{}, fmt.Errorf("request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload clustersResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return clustersResponse{}, fmt.Errorf("parse response as json: %w", err)
	}

	return payload, nil
}
