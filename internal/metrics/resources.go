package metrics

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultScalerAddress = "keda-otel-scaler.keda.svc:4318"
	DefaultTargetValue   = "1"
)

type ResourceOptions struct {
	Query               string
	PrometheusURL       string
	Namespace           string
	ScaledObjectName    string
	ScaleTargetName     string
	MetricPredictorName string
	PrometheusStart     string
	PrometheusEnd       string
	PrometheusStep      string
}

type objectMeta struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type scaledObject struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   objectMeta       `yaml:"metadata"`
	Spec       scaledObjectSpec `yaml:"spec"`
}

type scaledObjectSpec struct {
	ScaleTargetRef scaleTargetRef `yaml:"scaleTargetRef"`
	Triggers       []trigger      `yaml:"triggers"`
}

type scaleTargetRef struct {
	Name string `yaml:"name"`
}

type trigger struct {
	Type     string            `yaml:"type"`
	Metadata map[string]string `yaml:"metadata"`
}

type metricPredictor struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   objectMeta          `yaml:"metadata"`
	Spec       metricPredictorSpec `yaml:"spec"`
}

type metricPredictorSpec struct {
	Source metricPredictorSource `yaml:"source"`
}

type metricPredictorSource struct {
	OneShotPrometheus prometheusSource `yaml:"oneShotPrometheus"`
}

type prometheusSource struct {
	URL     string `yaml:"url"`
	Query   string `yaml:"query"`
	Start   string `yaml:"start,omitempty"`
	End     string `yaml:"end,omitempty"`
	Step    string `yaml:"step,omitempty"`
	Timeout string `yaml:"timeout,omitempty"`
}

func BuildResources(options ResourceOptions) ([]byte, error) {
	if strings.TrimSpace(options.Query) == "" {
		return nil, fmt.Errorf("PromQL query is required")
	}
	if options.Namespace == "" {
		options.Namespace = "default"
	}

	resources := make([]any, 0, 2)
	if options.ScaledObjectName != "" {
		if options.ScaleTargetName == "" {
			return nil, fmt.Errorf("scale target name is required for ScaledObject %q", options.ScaledObjectName)
		}
		resources = append(resources, scaledObject{
			APIVersion: "keda.sh/v1alpha1",
			Kind:       "ScaledObject",
			Metadata: objectMeta{
				Name:      options.ScaledObjectName,
				Namespace: options.Namespace,
			},
			Spec: scaledObjectSpec{
				ScaleTargetRef: scaleTargetRef{Name: options.ScaleTargetName},
				Triggers: []trigger{{
					Type: "kedify-otel",
					Metadata: map[string]string{
						"metricQuery":   options.Query,
						"scalerAddress": DefaultScalerAddress,
						"targetValue":   DefaultTargetValue,
					},
				}},
			},
		})
	}

	if options.MetricPredictorName != "" {
		if strings.TrimSpace(options.PrometheusURL) == "" {
			return nil, fmt.Errorf("prometheus URL is required for MetricPredictor %q", options.MetricPredictorName)
		}
		rangeFields := 0
		for _, field := range []string{options.PrometheusStart, options.PrometheusEnd, options.PrometheusStep} {
			if field != "" {
				rangeFields++
			}
		}
		if rangeFields != 0 && rangeFields != 3 {
			return nil, fmt.Errorf("prometheus start, end, and step must be specified together")
		}
		resources = append(resources, metricPredictor{
			APIVersion: "keda.kedify.io/v1alpha1",
			Kind:       "MetricPredictor",
			Metadata: objectMeta{
				Name:      options.MetricPredictorName,
				Namespace: options.Namespace,
			},
			Spec: metricPredictorSpec{
				Source: metricPredictorSource{
					OneShotPrometheus: prometheusSource{
						URL:     options.PrometheusURL,
						Query:   options.Query,
						Start:   options.PrometheusStart,
						End:     options.PrometheusEnd,
						Step:    options.PrometheusStep,
						Timeout: "30s",
					},
				},
			},
		})
	}

	if len(resources) == 0 {
		return nil, nil
	}

	var output bytes.Buffer
	for index, resource := range resources {
		data, err := yaml.Marshal(resource)
		if err != nil {
			return nil, fmt.Errorf("encode Kubernetes resource: %w", err)
		}
		if index > 0 {
			output.WriteString("---\n")
		}
		if _, err := output.Write(data); err != nil {
			return nil, fmt.Errorf("write Kubernetes resource: %w", err)
		}
	}
	return output.Bytes(), nil
}
