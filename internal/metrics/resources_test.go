package metrics

import (
	"bytes"
	"io"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildResourcesCreatesScaledObjectAndMetricPredictor(t *testing.T) {
	data, err := BuildResources(ResourceOptions{
		Query:               `sum(http_requests_total{method="GET"})`,
		PrometheusURL:       "http://prometheus.monitoring.svc.cluster.local:9090",
		Namespace:           "apps",
		ScaledObjectName:    "web-scaler",
		ScaleTargetName:     "web",
		MetricPredictorName: "web-predictor",
		PrometheusStart:     "2026-07-20T12:00:00Z",
		PrometheusEnd:       "2026-07-27T12:00:00Z",
		PrometheusStep:      "30s",
	})
	if err != nil {
		t.Fatalf("BuildResources() error = %v", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var resources []map[string]any
	for {
		var resource map[string]any
		err := decoder.Decode(&resource)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode resource: %v\n%s", err, data)
		}
		if len(resource) > 0 {
			resources = append(resources, resource)
		}
	}
	if len(resources) != 2 {
		t.Fatalf("decoded %d resources, want 2\n%s", len(resources), data)
	}

	scaledObject := resources[0]
	if scaledObject["apiVersion"] != "keda.sh/v1alpha1" || scaledObject["kind"] != "ScaledObject" {
		t.Fatalf("ScaledObject header = %#v", scaledObject)
	}
	scaledSpec := scaledObject["spec"].(map[string]any)
	triggers := scaledSpec["triggers"].([]any)
	trigger := triggers[0].(map[string]any)
	if trigger["type"] != "kedify-otel" {
		t.Fatalf("trigger type = %#v", trigger["type"])
	}
	metadata := trigger["metadata"].(map[string]any)
	if metadata["metricQuery"] != `sum(http_requests_total{method="GET"})` {
		t.Fatalf("metricQuery = %#v", metadata["metricQuery"])
	}

	predictor := resources[1]
	if predictor["apiVersion"] != "keda.kedify.io/v1alpha1" || predictor["kind"] != "MetricPredictor" {
		t.Fatalf("MetricPredictor header = %#v", predictor)
	}
	predictorSpec := predictor["spec"].(map[string]any)
	source := predictorSpec["source"].(map[string]any)
	prometheusSource := source["oneShotPrometheus"].(map[string]any)
	if prometheusSource["url"] != "http://prometheus.monitoring.svc.cluster.local:9090" {
		t.Fatalf("MetricPredictor URL = %#v", prometheusSource["url"])
	}
	if prometheusSource["query"] != `sum(http_requests_total{method="GET"})` {
		t.Fatalf("MetricPredictor query = %#v", prometheusSource["query"])
	}
	if prometheusSource["start"] != "2026-07-20T12:00:00Z" ||
		prometheusSource["end"] != "2026-07-27T12:00:00Z" ||
		prometheusSource["step"] != "30s" {
		t.Fatalf("MetricPredictor range = %#v", prometheusSource)
	}
}

func TestBuildResourcesValidatesRequiredFields(t *testing.T) {
	if _, err := BuildResources(ResourceOptions{}); err == nil {
		t.Fatal("empty options returned nil error")
	}
	if _, err := BuildResources(ResourceOptions{
		Query:            "up",
		ScaledObjectName: "demo",
	}); err == nil {
		t.Fatal("missing scale target returned nil error")
	}
	if _, err := BuildResources(ResourceOptions{
		Query:               "up",
		MetricPredictorName: "demo",
	}); err == nil {
		t.Fatal("missing Prometheus URL returned nil error")
	}
}
