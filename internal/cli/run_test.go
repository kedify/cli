package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricsCommandKeepsInteractiveErrorsOffStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := Run(
		[]string{"metrics", "--server=http://127.0.0.1:1"},
		bytes.NewBuffer(nil),
		stdout,
		stderr,
	)
	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMetricsCommandAcceptsLessInteractiveFlags(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := Run(
		[]string{
			"metrics",
			"--server=http://127.0.0.1:1",
			"--filter=memory_",
			"--query=sum(foobar)",
			"--visualize",
			"--horizon=3d",
			"--print",
			"--context=staging",
			"--kubeconfig=./testdata/kubeconfig",
		},
		bytes.NewBuffer(nil),
		stdout,
		stderr,
	)
	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
