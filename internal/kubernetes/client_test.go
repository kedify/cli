package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMatchingSelectorRecognizesConfiguredPrometheusLabels(t *testing.T) {
	tests := []struct {
		labels map[string]string
		want   string
	}{
		{
			labels: map[string]string{"app": "kube-prometheus-stack-prometheus"},
			want:   "app=kube-prometheus-stack-prometheus",
		},
		{
			labels: map[string]string{
				"app.kubernetes.io/component": "server",
				"app.kubernetes.io/name":      "prometheus",
				"extra":                       "kept",
			},
			want: "app.kubernetes.io/component=server,app.kubernetes.io/name=prometheus",
		},
		{
			labels: map[string]string{
				"app.kubernetes.io/component": "gateway",
				"app.kubernetes.io/name":      "mimir",
				"app.kubernetes.io/instance":  "mimir",
			},
			want: "app.kubernetes.io/component=gateway,app.kubernetes.io/name=mimir",
		},
		{
			labels: map[string]string{
				"app.kubernetes.io/component": "querier",
				"app.kubernetes.io/name":      "mimir",
			},
			want: "",
		},
		{
			labels: map[string]string{"app": "not-prometheus"},
			want:   "",
		},
	}

	for _, test := range tests {
		if got := matchingSelector(test.labels); got != test.want {
			t.Fatalf("matchingSelector(%v) = %q, want %q", test.labels, got, test.want)
		}
	}
}

func TestBestServicePortPrefersPrometheusAndHTTPPorts(t *testing.T) {
	port, ok := bestServicePort([]servicePort{
		{Name: "grpc", Port: 4317},
		{Name: "http", Port: 8080},
		{Name: "web", Port: 9090},
	})
	if !ok {
		t.Fatal("bestServicePort() ok = false")
	}
	if port.Port != 9090 {
		t.Fatalf("bestServicePort() = %#v, want port 9090", port)
	}
}

func TestDiscoverPrometheusServicesFiltersSortsAndBuildsURLs(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	script := `#!/bin/sh
printf '%s' '{"items":[
  {"metadata":{"name":"z-prom","namespace":"z","labels":{"app":"prometheus-server"}},"spec":{"ports":[{"name":"web","port":9090}]}},
  {"metadata":{"name":"ignored","namespace":"a","labels":{"app":"other"}},"spec":{"ports":[{"port":9090}]}},
  {"metadata":{"name":"a-prom","namespace":"a","labels":{"app":"rancher-monitoring-prometheus"}},"spec":{"ports":[{"name":"https","port":443}]}},
  {"metadata":{"name":"mimir-gateway","namespace":"mimir","labels":{"app.kubernetes.io/name":"mimir","app.kubernetes.io/component":"gateway"}},"spec":{"ports":[{"name":"http-metrics","port":8080},{"name":"gateway","port":80}]}}
]}'
`
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	services, err := NewClientWithBinary(kubectl).DiscoverPrometheusServices(context.Background())
	if err != nil {
		t.Fatalf("DiscoverPrometheusServices() error = %v", err)
	}
	if len(services) != 3 {
		t.Fatalf("services = %#v", services)
	}
	if services[0].Name != "a-prom" || services[1].Name != "mimir-gateway" || services[2].Name != "z-prom" {
		t.Fatalf("services are not sorted: %#v", services)
	}
	if got := services[0].ClusterURL(); got != "https://a-prom.a.svc.cluster.local:443" {
		t.Fatalf("ClusterURL() = %q", got)
	}
	if got := services[1].ClusterURL(); got != "http://mimir-gateway.mimir.svc.cluster.local:80/prometheus" {
		t.Fatalf("Mimir ClusterURL() = %q", got)
	}
	if services[1].Port != 80 {
		t.Fatalf("Mimir port = %d, want 80", services[1].Port)
	}
	if !services[1].Mimir {
		t.Fatal("Mimir gateway was not marked as Mimir")
	}
}

func TestCurrentContextReturnsActiveKubernetesContext(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\nprintf '%s\\n' 'production-eu'\n"
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	currentContext, err := NewClientWithBinary(kubectl).CurrentContext(context.Background())
	if err != nil {
		t.Fatalf("CurrentContext() error = %v", err)
	}
	if currentContext != "production-eu" {
		t.Fatalf("CurrentContext() = %q, want production-eu", currentContext)
	}
}

func TestCurrentContextRejectsEmptyContext(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	_, err := NewClientWithBinary(kubectl).CurrentContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "context is empty") {
		t.Fatalf("CurrentContext() error = %v", err)
	}
}

func TestCurrentContextReturnsExplicitContextWithoutRunningKubectl(t *testing.T) {
	client := NewClientWithBinaryAndOptions(
		filepath.Join(t.TempDir(), "missing-kubectl"),
		ClientOptions{Context: "staging"},
	)

	currentContext, err := client.CurrentContext(context.Background())
	if err != nil {
		t.Fatalf("CurrentContext() error = %v", err)
	}
	if currentContext != "staging" {
		t.Fatalf("CurrentContext() = %q, want staging", currentContext)
	}
}

func TestCreatePassesManifestOnStdinAndReturnsNames(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	captured := filepath.Join(dir, "manifest")
	capturedArgs := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + capturedArgs + "\ncat > " + captured + "\nprintf '%s\\n' 'scaledobject.keda.sh/demo' 'metricpredictor.keda.kedify.io/demo'\n"
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	manifest := []byte("kind: ScaledObject\n")
	names, err := NewClientWithBinaryAndOptions(kubectl, ClientOptions{
		Context:    "staging",
		Kubeconfig: "./config/staging.kubeconfig",
	}).Create(context.Background(), manifest)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := []string{"scaledobject.keda.sh/demo", "metricpredictor.keda.kedify.io/demo"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Create() = %#v, want %#v", names, want)
	}
	gotManifest, err := os.ReadFile(captured)
	if err != nil {
		t.Fatalf("read captured manifest: %v", err)
	}
	if string(gotManifest) != string(manifest) {
		t.Fatalf("manifest = %q, want %q", gotManifest, manifest)
	}
	assertCapturedArgs(t, capturedArgs, []string{
		"--context", "staging",
		"--kubeconfig", "./config/staging.kubeconfig",
		"create", "-f", "-", "-o", "name",
	})
}

func TestStartPortForwardReadsRandomPortAndStopsProcess(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	capturedArgs := filepath.Join(dir, "args")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + capturedArgs + `
printf '%s\n' 'Forwarding from 127.0.0.1:45678 -> 9090' >&2
exec sleep 60
`
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	forward, err := NewClientWithBinaryAndOptions(kubectl, ClientOptions{
		Context:    "staging",
		Kubeconfig: "./config/staging.kubeconfig",
	}).StartPortForward(context.Background(), PrometheusService{
		Name:       "mimir-gateway",
		Namespace:  "mimir",
		Port:       80,
		Scheme:     "http",
		PathPrefix: "/prometheus",
	})
	if err != nil {
		t.Fatalf("StartPortForward() error = %v", err)
	}
	if forward.LocalURL != "http://127.0.0.1:45678/prometheus" {
		t.Fatalf("LocalURL = %q", forward.LocalURL)
	}
	if err := forward.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertCapturedArgs(t, capturedArgs, []string{
		"--context", "staging",
		"--kubeconfig", "./config/staging.kubeconfig",
		"port-forward", "--namespace", "mimir", "service/mimir-gateway", ":80",
	})
}

func TestIsLocalAddress(t *testing.T) {
	for _, value := range []string{"http://localhost:9090", "http://127.0.0.1:1234", "http://[::1]:9090"} {
		if !IsLocalAddress(value) {
			t.Fatalf("IsLocalAddress(%q) = false", value)
		}
	}
	if IsLocalAddress("http://prometheus.monitoring.svc:9090") {
		t.Fatal("cluster service URL considered local")
	}
}

func assertCapturedArgs(t *testing.T, path string, want []string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured arguments: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kubectl arguments = %#v, want %#v", got, want)
	}
}
