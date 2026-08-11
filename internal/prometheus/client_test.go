package prometheus

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMetricNamesUsesLabelValuesWithSeriesSelector(t *testing.T) {
	client := testClient(t, "http://prometheus.test/prometheus/", func(r *http.Request) string {
		if r.URL.Path != "/prometheus/api/v1/label/__name__/values" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("match[]"); got != allMetrics {
			t.Fatalf("match[] = %q, want %q", got, allMetrics)
		}
		return `{"status":"success","data":["up","requests_total"]}`
	})

	names, err := client.MetricNames(context.Background())
	if err != nil {
		t.Fatalf("MetricNames() error = %v", err)
	}
	want := []string{"requests_total", "up"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("MetricNames() = %#v, want %#v", names, want)
	}
}

func TestLabelNamesAndValuesUsePrometheusMetadataEndpoints(t *testing.T) {
	var calls int
	client := testClient(t, "http://prometheus.test", func(r *http.Request) string {
		calls++
		switch r.URL.Path {
		case "/api/v1/labels":
			if got := r.URL.Query().Get("match[]"); got != "http_requests_total" {
				t.Fatalf("labels match[] = %q", got)
			}
			return `{"status":"success","data":["zone","__name__","method"]}`
		case "/api/v1/label/method/values":
			if got := r.URL.Query().Get("match[]"); got != `http_requests_total{zone="eu"}` {
				t.Fatalf("values match[] = %q", got)
			}
			return `{"status":"success","data":["POST","GET"]}`
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
			return ""
		}
	})
	labels, err := client.LabelNames(context.Background(), "http_requests_total")
	if err != nil {
		t.Fatalf("LabelNames() error = %v", err)
	}
	if want := []string{"method", "zone"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("LabelNames() = %#v, want %#v", labels, want)
	}

	values, err := client.LabelValues(context.Background(), "method", `http_requests_total{zone="eu"}`)
	if err != nil {
		t.Fatalf("LabelValues() error = %v", err)
	}
	if want := []string{"GET", "POST"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("LabelValues() = %#v, want %#v", values, want)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestValidateQueryReturnsPrometheusAPIError(t *testing.T) {
	client := testClient(t, "http://prometheus.test", func(r *http.Request) string {
		if got := r.Header.Get(mimirScopeOrgIDHeader); got != "" {
			t.Fatalf("%s = %q for regular Prometheus", mimirScopeOrgIDHeader, got)
		}
		return `{"status":"error","errorType":"bad_data","error":"parse error"}`
	})
	err := client.ValidateQuery(context.Background(), "not valid(")
	if err == nil || err.Error() != "validate PromQL query: bad_data: parse error" {
		t.Fatalf("ValidateQuery() error = %v", err)
	}
}

func TestValidateQueryUsesMimirPrefixAndScopeOrgID(t *testing.T) {
	client := testMimirClient(t, "http://mimir-gateway.test/prometheus", func(r *http.Request) string {
		if r.URL.Path != "/prometheus/api/v1/query" {
			t.Fatalf("path = %q, want /prometheus/api/v1/query", r.URL.Path)
		}
		if got := r.Header.Get(mimirScopeOrgIDHeader); got != mimirScopeOrgID {
			t.Fatalf("%s = %q, want %q", mimirScopeOrgIDHeader, got, mimirScopeOrgID)
		}
		if got := r.URL.Query().Get("query"); got != "sum(up)" {
			t.Fatalf("query = %q, want sum(up)", got)
		}
		return `{"status":"success","data":{"resultType":"vector","result":[]}}`
	})

	if err := client.ValidateQuery(context.Background(), "sum(up)"); err != nil {
		t.Fatalf("ValidateQuery() error = %v", err)
	}
}

func TestRangeQueryDecodesMatrix(t *testing.T) {
	client := testClient(t, "http://prometheus.test", func(r *http.Request) string {
		if got := r.URL.Query().Get("step"); got != "30" {
			t.Fatalf("step = %q, want 30", got)
		}
		return `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up","job":"prometheus"},"values":[[10,"1"],[40,"2.5"],[70,"stale"]]}]}}`
	})
	series, err := client.RangeQuery(
		context.Background(),
		"up",
		time.Unix(10, 0),
		time.Unix(70, 0),
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("RangeQuery() error = %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 2 {
		t.Fatalf("RangeQuery() = %#v", series)
	}
	if series[0].Points[1].Value != 2.5 {
		t.Fatalf("second value = %v, want 2.5", series[0].Points[1].Value)
	}
}

func TestBuildSelectorSortsAndQuotesLabels(t *testing.T) {
	got := BuildSelector("requests_total", map[string]string{
		"zone":   "eu-west",
		"method": `G"ET`,
	})
	want := `requests_total{method="G\"ET",zone="eu-west"}`
	if got != want {
		t.Fatalf("BuildSelector() = %q, want %q", got, want)
	}
}

func TestNormalizeURL(t *testing.T) {
	got, err := NormalizeURL("prometheus.monitoring:9090/")
	if err != nil {
		t.Fatalf("NormalizeURL() error = %v", err)
	}
	if got != "http://prometheus.monitoring:9090" {
		t.Fatalf("NormalizeURL() = %q", got)
	}

	for _, value := range []string{"", "ftp://prometheus", "http:///missing-host", "http://prometheus?x=1"} {
		if _, err := NormalizeURL(value); err == nil {
			t.Fatalf("NormalizeURL(%q) returned nil error", value)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testClient(t *testing.T, baseURL string, response func(*http.Request) string) *Client {
	t.Helper()
	httpClient := testHTTPClient(response)
	client, err := NewClientWithHTTP(baseURL, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP() error = %v", err)
	}
	return client
}

func testMimirClient(t *testing.T, baseURL string, response func(*http.Request) string) *Client {
	t.Helper()
	httpClient := testHTTPClient(response)
	client, err := NewMimirClientWithHTTP(baseURL, httpClient)
	if err != nil {
		t.Fatalf("NewMimirClientWithHTTP() error = %v", err)
	}
	return client
}

func testHTTPClient(response func(*http.Request) string) *http.Client {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := response(request)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		}),
	}
	return httpClient
}
