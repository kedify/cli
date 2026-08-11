package metrics

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/kubernetes"
)

type fakeResourceCreator struct {
	calls    int
	manifest []byte
	names    []string
	err      error
}

func (f *fakeResourceCreator) Create(_ context.Context, manifest []byte) ([]string, error) {
	f.calls++
	f.manifest = append([]byte(nil), manifest...)
	return f.names, f.err
}

func TestMetricsCmdRequiresInteractiveTerminalBeforeNetworkAccess(t *testing.T) {
	cmd := MetricsCmd{Server: "http://127.0.0.1:1"}
	err := cmd.Run(&clictx.Context{
		Stdin:  bytes.NewBuffer(nil),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestMetricsCmdValidatesAutomationFlagCombinations(t *testing.T) {
	tests := []struct {
		name string
		cmd  MetricsCmd
		want string
	}{
		{
			name: "server and discovery",
			cmd:  MetricsCmd{Server: "http://localhost:9090", Disco: true},
			want: "--server and --disco",
		},
		{
			name: "visualize without query",
			cmd:  MetricsCmd{Visualize: true},
			want: "--visualize requires --query",
		},
		{
			name: "horizon without visualize",
			cmd:  MetricsCmd{Query: "up", Horizon: "3d"},
			want: "--horizon requires --visualize",
		},
		{
			name: "unsupported horizon",
			cmd:  MetricsCmd{Query: "up", Visualize: true, Horizon: "2d"},
			want: "unsupported --horizon",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cmd.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}

	valid := MetricsCmd{
		Disco:     true,
		Filter:    "memory_",
		Query:     "sum(foobar)",
		Visualize: true,
		Horizon:   "3d",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMetricsCmdDiscoUsesFirstServiceAndStartsPortForward(t *testing.T) {
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	script := `#!/bin/sh
case "$1" in
get)
  printf '%s' '{"items":[
    {"metadata":{"name":"z-prom","namespace":"z","labels":{"app":"prometheus-server"}},"spec":{"ports":[{"name":"web","port":9090}]}},
    {"metadata":{"name":"a-prom","namespace":"a","labels":{"app":"rancher-monitoring-prometheus"}},"spec":{"ports":[{"name":"https","port":443}]}}
  ]}'
  ;;
port-forward)
  printf '%s\n' 'Forwarding from 127.0.0.1:45678 -> 443' >&2
  exec sleep 60
  ;;
*)
  exit 1
  ;;
esac
`
	if err := os.WriteFile(kubectl, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake kubectl: %v", err)
	}

	stderr := &bytes.Buffer{}
	cmd := MetricsCmd{Disco: true}
	endpoint, forward, err := cmd.resolveServer(
		context.Background(),
		&clictx.Context{
			Stdin:  bytes.NewBuffer(nil),
			Stdout: &bytes.Buffer{},
			Stderr: stderr,
		},
		kubernetes.NewClientWithBinary(kubectl),
	)
	if err != nil {
		t.Fatalf("resolveServer() error = %v", err)
	}
	t.Cleanup(func() {
		if err := forward.Stop(); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	if endpoint.sessionURL != "https://127.0.0.1:45678" {
		t.Fatalf("session URL = %q", endpoint.sessionURL)
	}
	if endpoint.clusterURL != "https://a-prom.a.svc.cluster.local:443" {
		t.Fatalf("cluster URL = %q", endpoint.clusterURL)
	}
	if endpoint.mimir {
		t.Fatal("regular Prometheus endpoint was marked as Mimir")
	}
	if !strings.Contains(stderr.String(), "Starting port-forward to a/a-prom") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDirectPrometheusEndpointDetectsMimirPrefix(t *testing.T) {
	tests := []struct {
		server string
		mimir  bool
	}{
		{server: "http://prometheus.monitoring:9090", mimir: false},
		{server: "http://mimir-gateway.mimir/prometheus", mimir: true},
		{server: "http://mimir-gateway.mimir/prometheus/", mimir: true},
	}

	for _, test := range tests {
		endpoint := directPrometheusEndpoint(test.server)
		if endpoint.mimir != test.mimir {
			t.Fatalf("directPrometheusEndpoint(%q).mimir = %t, want %t", test.server, endpoint.mimir, test.mimir)
		}
	}
}

func TestMetricsCmdPrintSkipsManifestEditPrompt(t *testing.T) {
	manifest := []byte("kind: ScaledObject\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := MetricsCmd{Print: true}

	result, err := cmd.editManifestIfRequested(
		&clictx.Context{
			Stdin:  bytes.NewBuffer(nil),
			Stdout: stdout,
			Stderr: stderr,
		},
		manifest,
	)
	if err != nil {
		t.Fatalf("editManifestIfRequested() error = %v", err)
	}
	if string(result) != string(manifest) {
		t.Fatalf("manifest = %q, want %q", result, manifest)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want no prompt output", stdout.String(), stderr.String())
	}
}

func TestValidateKubernetesName(t *testing.T) {
	for _, name := range []string{"demo", "metric-predictor", "a1"} {
		if err := validateKubernetesName(name); err != nil {
			t.Fatalf("validateKubernetesName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"", "-demo", "Demo", "demo_", "demo-"} {
		if err := validateKubernetesName(name); err == nil {
			t.Fatalf("validateKubernetesName(%q) returned nil error", name)
		}
	}
}

func TestOutputResourcesPrintsYAMLWithoutCreatingByDefault(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	creator := &fakeResourceCreator{}
	manifest := []byte("kind: ScaledObject\n")

	err := outputResources(
		context.Background(),
		stdout,
		stderr,
		creator,
		"default",
		manifest,
		false,
	)
	if err != nil {
		t.Fatalf("outputResources() error = %v", err)
	}
	if stdout.String() != string(manifest) {
		t.Fatalf("stdout = %q, want YAML", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if creator.calls != 0 {
		t.Fatalf("Create() calls = %d, want 0", creator.calls)
	}
}

func TestOutputResourcesCreatesOnlyWhenExplicitlyEnabled(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	creator := &fakeResourceCreator{
		names: []string{"scaledobject.keda.sh/demo"},
	}
	manifest := []byte("kind: ScaledObject\n")

	err := outputResources(
		context.Background(),
		stdout,
		stderr,
		creator,
		"apps",
		manifest,
		true,
	)
	if err != nil {
		t.Fatalf("outputResources() error = %v", err)
	}
	if stdout.String() != string(manifest) {
		t.Fatalf("stdout = %q, want YAML", stdout.String())
	}
	if creator.calls != 1 {
		t.Fatalf("Create() calls = %d, want 1", creator.calls)
	}
	if !strings.Contains(stderr.String(), "Created scaledobject.keda.sh/demo") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
