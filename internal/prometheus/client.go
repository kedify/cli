package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout        = 30 * time.Second
	allMetrics            = `{__name__=~".+"}`
	mimirScopeOrgIDHeader = "X-Scope-OrgID"
	mimirScopeOrgID       = "kedify-agent"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	scopeOrgID string
}

type Series struct {
	Labels map[string]string
	Points []Point
}

type Point struct {
	Timestamp float64
	Value     float64
}

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
}

type queryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type matrixSeries struct {
	Metric map[string]string   `json:"metric"`
	Values [][]json.RawMessage `json:"values"`
}

func NewClient(rawURL string) (*Client, error) {
	return NewClientWithHTTP(rawURL, &http.Client{Timeout: defaultTimeout})
}

func NewMimirClient(rawURL string) (*Client, error) {
	return NewMimirClientWithHTTP(rawURL, &http.Client{Timeout: defaultTimeout})
}

func NewClientWithHTTP(rawURL string, httpClient *http.Client) (*Client, error) {
	return newClientWithHTTP(rawURL, httpClient, "")
}

func NewMimirClientWithHTTP(rawURL string, httpClient *http.Client) (*Client, error) {
	return newClientWithHTTP(rawURL, httpClient, mimirScopeOrgID)
}

func newClientWithHTTP(rawURL string, httpClient *http.Client, scopeOrgID string) (*Client, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	baseURL, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse Prometheus URL: %w", err)
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		scopeOrgID: scopeOrgID,
	}, nil
}

func NormalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("Prometheus server URL is required")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse Prometheus URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("Prometheus URL scheme must be http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("Prometheus server URL must include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Prometheus server URL must not include a query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return parsed.String(), nil
}

func (c *Client) MetricNames(ctx context.Context) ([]string, error) {
	var names []string
	if err := c.get(
		ctx,
		"/api/v1/label/__name__/values",
		url.Values{"match[]": []string{allMetrics}},
		&names,
	); err != nil {
		return nil, fmt.Errorf("list Prometheus metric names: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) LabelNames(ctx context.Context, selector string) ([]string, error) {
	var labels []string
	if err := c.get(ctx, "/api/v1/labels", url.Values{"match[]": []string{selector}}, &labels); err != nil {
		return nil, fmt.Errorf("list labels for %q: %w", selector, err)
	}

	filtered := labels[:0]
	for _, label := range labels {
		if label != "__name__" {
			filtered = append(filtered, label)
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func (c *Client) LabelValues(ctx context.Context, label, selector string) ([]string, error) {
	var values []string
	path := "/api/v1/label/" + url.PathEscape(label) + "/values"
	if err := c.get(ctx, path, url.Values{"match[]": []string{selector}}, &values); err != nil {
		return nil, fmt.Errorf("list values for label %q: %w", label, err)
	}

	sort.Strings(values)
	return values, nil
}

func (c *Client) ValidateQuery(ctx context.Context, query string) error {
	var data queryData
	if err := c.get(ctx, "/api/v1/query", url.Values{"query": []string{query}}, &data); err != nil {
		return fmt.Errorf("validate PromQL query: %w", err)
	}
	return nil
}

func (c *Client) RangeQuery(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	if step <= 0 {
		return nil, errors.New("Prometheus range query step must be greater than zero")
	}

	params := url.Values{
		"query": []string{query},
		"start": []string{start.UTC().Format(time.RFC3339)},
		"end":   []string{end.UTC().Format(time.RFC3339)},
		"step":  []string{strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}

	var data queryData
	if err := c.get(ctx, "/api/v1/query_range", params, &data); err != nil {
		return nil, fmt.Errorf("query Prometheus range: %w", err)
	}
	if data.ResultType != "matrix" {
		return nil, fmt.Errorf("range query returned %q, want matrix", data.ResultType)
	}

	var rawSeries []matrixSeries
	if err := json.Unmarshal(data.Result, &rawSeries); err != nil {
		return nil, fmt.Errorf("decode Prometheus matrix: %w", err)
	}

	series := make([]Series, 0, len(rawSeries))
	for _, raw := range rawSeries {
		points := make([]Point, 0, len(raw.Values))
		for _, value := range raw.Values {
			if len(value) != 2 {
				continue
			}

			var timestamp float64
			if err := json.Unmarshal(value[0], &timestamp); err != nil {
				return nil, fmt.Errorf("decode Prometheus sample timestamp: %w", err)
			}

			var text string
			if err := json.Unmarshal(value[1], &text); err != nil {
				return nil, fmt.Errorf("decode Prometheus sample value: %w", err)
			}
			number, err := strconv.ParseFloat(text, 64)
			if err != nil {
				continue
			}
			points = append(points, Point{Timestamp: timestamp, Value: number})
		}
		if len(points) > 0 {
			series = append(series, Series{Labels: raw.Metric, Points: points})
		}
	}
	return series, nil
}

func BuildSelector(metric string, labels map[string]string) string {
	if len(labels) == 0 {
		return metric
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	matchers := make([]string, 0, len(keys))
	for _, key := range keys {
		matchers = append(matchers, key+"="+strconv.Quote(labels[key]))
	}
	return metric + "{" + strings.Join(matchers, ",") + "}"
}

func (c *Client) get(ctx context.Context, path string, params url.Values, target any) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create Prometheus request: %w", err)
	}
	if c.scopeOrgID != "" {
		request.Header.Set(mimirScopeOrgIDHeader, c.scopeOrgID)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read Prometheus response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Prometheus returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode Prometheus response: %w", err)
	}
	if envelope.Status != "success" {
		detail := strings.TrimSpace(envelope.Error)
		if detail == "" {
			detail = "unknown API error"
		}
		if envelope.ErrorType != "" {
			detail = envelope.ErrorType + ": " + detail
		}
		return errors.New(detail)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("decode Prometheus data: %w", err)
	}
	return nil
}
