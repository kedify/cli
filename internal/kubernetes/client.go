package kubernetes

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultKubectl       = "kubectl"
	portForwardTimeout   = 15 * time.Second
	portForwardStopLimit = 3 * time.Second
)

var prometheusServiceLabelSelectors = []prometheusServiceLabelSelector{
	{
		labels: map[string]string{
			"app.kubernetes.io/name":      "mimir",
			"app.kubernetes.io/component": "gateway",
		},
		display:       "app.kubernetes.io/component=gateway,app.kubernetes.io/name=mimir",
		pathPrefix:    "/prometheus",
		preferredPort: 80,
		mimir:         true,
	},
	{labels: map[string]string{"app": "kube-prometheus-stack-prometheus"}, display: "app=kube-prometheus-stack-prometheus"},
	{labels: map[string]string{"app.kubernetes.io/component": "server", "app.kubernetes.io/name": "prometheus"}, display: "app.kubernetes.io/component=server,app.kubernetes.io/name=prometheus"},
	{labels: map[string]string{"app": "prometheus", "component": "server"}, display: "app=prometheus,component=server"},
	{labels: map[string]string{"app": "prometheus-server"}, display: "app=prometheus-server"},
	{labels: map[string]string{"app": "prometheus-operator-prometheus"}, display: "app=prometheus-operator-prometheus"},
	{labels: map[string]string{"app": "rancher-monitoring-prometheus"}, display: "app=rancher-monitoring-prometheus"},
	{labels: map[string]string{"app": "prometheus-prometheus"}, display: "app=prometheus-prometheus"},
}

var forwardingPattern = regexp.MustCompile(`Forwarding from (?:127\.0\.0\.1|\[::1\]):([0-9]+)`)

type prometheusServiceLabelSelector struct {
	labels        map[string]string
	display       string
	pathPrefix    string
	preferredPort int
	mimir         bool
}

type Client struct {
	binary     string
	context    string
	kubeconfig string
}

type ClientOptions struct {
	Context    string
	Kubeconfig string
}

type PrometheusService struct {
	Name          string
	Namespace     string
	Port          int
	PortName      string
	Scheme        string
	MatchedLabels string
	PathPrefix    string
	Mimir         bool
}

func (s PrometheusService) DisplayName() string {
	port := strconv.Itoa(s.Port)
	if s.PortName != "" {
		port = s.PortName + ":" + port
	}
	return s.Namespace + "/" + s.Name + "  " + port
}

func (s PrometheusService) ClusterURL() string {
	return fmt.Sprintf(
		"%s://%s.%s.svc.cluster.local:%d%s",
		s.Scheme,
		s.Name,
		s.Namespace,
		s.Port,
		s.normalizedPathPrefix(),
	)
}

func (s PrometheusService) normalizedPathPrefix() string {
	pathPrefix := strings.Trim(strings.TrimSpace(s.PathPrefix), "/")
	if pathPrefix == "" {
		return ""
	}
	return "/" + pathPrefix
}

type PortForward struct {
	LocalURL string

	cancel context.CancelFunc
	cmd    *exec.Cmd
	done   chan error

	mu          sync.Mutex
	diagnostics []string
}

type serviceList struct {
	Items []service `json:"items"`
}

type service struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Ports []servicePort `json:"ports"`
	} `json:"spec"`
}

type servicePort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func NewClient() *Client {
	return NewClientWithOptions(ClientOptions{})
}

func NewClientWithOptions(options ClientOptions) *Client {
	return newClient(defaultKubectl, options)
}

func NewClientWithBinary(binary string) *Client {
	return NewClientWithBinaryAndOptions(binary, ClientOptions{})
}

func NewClientWithBinaryAndOptions(binary string, options ClientOptions) *Client {
	return newClient(binary, options)
}

func newClient(binary string, options ClientOptions) *Client {
	return &Client{
		binary:     binary,
		context:    options.Context,
		kubeconfig: options.Kubeconfig,
	}
}

func (c *Client) DiscoverPrometheusServices(ctx context.Context) ([]PrometheusService, error) {
	output, err := c.run(ctx, nil, "get", "services", "--all-namespaces", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("discover Prometheus services: %w", err)
	}

	var list serviceList
	if err := json.Unmarshal(output, &list); err != nil {
		return nil, fmt.Errorf("decode Kubernetes services: %w", err)
	}

	matches := make([]PrometheusService, 0)
	for _, item := range list.Items {
		selector, ok := matchingPrometheusServiceSelector(item.Metadata.Labels)
		if !ok {
			continue
		}

		port, ok := bestServicePortForSelector(item.Spec.Ports, selector)
		if !ok {
			continue
		}
		matches = append(matches, PrometheusService{
			Name:          item.Metadata.Name,
			Namespace:     item.Metadata.Namespace,
			Port:          port.Port,
			PortName:      port.Name,
			Scheme:        serviceScheme(port),
			MatchedLabels: selector.display,
			PathPrefix:    selector.pathPrefix,
			Mimir:         selector.mimir,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Namespace == matches[j].Namespace {
			return matches[i].Name < matches[j].Name
		}
		return matches[i].Namespace < matches[j].Namespace
	})
	return matches, nil
}

func (c *Client) CurrentNamespace(ctx context.Context) string {
	output, err := c.run(ctx, nil, "config", "view", "--minify", "-o", "jsonpath={..namespace}")
	if err != nil {
		return "default"
	}
	namespace := strings.TrimSpace(string(output))
	if namespace == "" {
		return "default"
	}
	return namespace
}

func (c *Client) CurrentContext(ctx context.Context) (string, error) {
	if c.context != "" {
		return c.context, nil
	}

	output, err := c.run(ctx, nil, "config", "current-context")
	if err != nil {
		return "", fmt.Errorf("get current Kubernetes context: %w", err)
	}
	currentContext := strings.TrimSpace(string(output))
	if currentContext == "" {
		return "", errors.New("current Kubernetes context is empty")
	}
	return currentContext, nil
}

func (c *Client) StartPortForward(parent context.Context, service PrometheusService) (*PortForward, error) {
	ctx, cancel := context.WithCancel(parent)
	target := "service/" + service.Name
	mapping := ":" + strconv.Itoa(service.Port)
	args := c.commandArgs("port-forward", "--namespace", service.Namespace, target, mapping)
	cmd := exec.CommandContext(ctx, c.binary, args...) // #nosec G204 -- kubectl is fixed by the caller and arguments are passed without a shell.

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("capture kubectl port-forward stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("capture kubectl port-forward stderr: %w", err)
	}

	forward := &PortForward{
		cancel: cancel,
		cmd:    cmd,
		done:   make(chan error, 1),
	}
	ready := make(chan int, 1)
	lines := make(chan string, 16)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start kubectl port-forward: %w", err)
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go forward.scanOutput(stdout, ready, lines, &readers)
	go forward.scanOutput(stderr, ready, lines, &readers)
	go func() {
		err := cmd.Wait()
		readers.Wait()
		forward.done <- err
		close(forward.done)
	}()

	timer := time.NewTimer(portForwardTimeout)
	defer timer.Stop()

	for {
		select {
		case port := <-ready:
			forward.LocalURL = fmt.Sprintf(
				"%s://127.0.0.1:%d%s",
				service.Scheme,
				port,
				service.normalizedPathPrefix(),
			)
			return forward, nil
		case err := <-forward.done:
			cancel()
			return nil, fmt.Errorf("kubectl port-forward stopped before it was ready: %s", portForwardError(err, forward.Diagnostics()))
		case line := <-lines:
			forward.addDiagnostic(line)
		case <-timer.C:
			cancel()
			return nil, fmt.Errorf("timed out waiting for kubectl port-forward: %s", strings.Join(forward.Diagnostics(), "; "))
		case <-parent.Done():
			cancel()
			return nil, parent.Err()
		}
	}
}

func (c *Client) Create(ctx context.Context, manifest []byte) ([]string, error) {
	output, err := c.run(ctx, manifest, "create", "-f", "-", "-o", "name")
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes resources: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	resources := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			resources = append(resources, line)
		}
	}
	return resources, nil
}

func (p *PortForward) Stop() error {
	if p == nil || p.cancel == nil {
		return nil
	}
	p.cancel()

	timer := time.NewTimer(portForwardStopLimit)
	defer timer.Stop()
	select {
	case err := <-p.done:
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	case <-timer.C:
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		return nil
	}
}

func (p *PortForward) Diagnostics() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.diagnostics...)
}

func (p *PortForward) scanOutput(reader io.Reader, ready chan<- int, lines chan<- string, done *sync.WaitGroup) {
	defer done.Done()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if match := forwardingPattern.FindStringSubmatch(line); len(match) == 2 {
			port, err := strconv.Atoi(match[1])
			if err == nil {
				select {
				case ready <- port:
				default:
				}
			}
		}
		select {
		case lines <- line:
		default:
			p.addDiagnostic(line)
		}
	}
	if err := scanner.Err(); err != nil {
		p.addDiagnostic(err.Error())
	}
}

func (p *PortForward) addDiagnostic(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.diagnostics) == 20 {
		copy(p.diagnostics, p.diagnostics[1:])
		p.diagnostics = p.diagnostics[:19]
	}
	p.diagnostics = append(p.diagnostics, line)
}

func (c *Client) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.binary, c.commandArgs(args...)...) // #nosec G204 -- kubectl is fixed by the caller and arguments are passed without a shell.
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (c *Client) commandArgs(args ...string) []string {
	commandArgs := make([]string, 0, len(args)+4)
	if c.context != "" {
		commandArgs = append(commandArgs, "--context", c.context)
	}
	if c.kubeconfig != "" {
		commandArgs = append(commandArgs, "--kubeconfig", c.kubeconfig)
	}
	return append(commandArgs, args...)
}

func matchingSelector(labels map[string]string) string {
	selector, ok := matchingPrometheusServiceSelector(labels)
	if !ok {
		return ""
	}
	return selector.display
}

func matchingPrometheusServiceSelector(labels map[string]string) (prometheusServiceLabelSelector, bool) {
	for _, selector := range prometheusServiceLabelSelectors {
		matches := true
		for key, want := range selector.labels {
			if labels[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return selector, true
		}
	}
	return prometheusServiceLabelSelector{}, false
}

func bestServicePortForSelector(
	ports []servicePort,
	selector prometheusServiceLabelSelector,
) (servicePort, bool) {
	if selector.preferredPort != 0 {
		for _, port := range ports {
			if port.Port == selector.preferredPort {
				return port, true
			}
		}
	}
	return bestServicePort(ports)
}

func bestServicePort(ports []servicePort) (servicePort, bool) {
	if len(ports) == 0 {
		return servicePort{}, false
	}

	score := func(port servicePort) int {
		name := strings.ToLower(port.Name)
		switch {
		case port.Port == 9090:
			return 100
		case name == "web" || name == "http-web":
			return 90
		case name == "http" || name == "prometheus":
			return 80
		case strings.Contains(name, "web") || strings.Contains(name, "http"):
			return 70
		default:
			return 0
		}
	}

	best := ports[0]
	bestScore := score(best)
	for _, port := range ports[1:] {
		if candidate := score(port); candidate > bestScore {
			best = port
			bestScore = candidate
		}
	}
	return best, true
}

func serviceScheme(port servicePort) string {
	name := strings.ToLower(port.Name)
	if port.Port == 443 || strings.Contains(name, "https") {
		return "https"
	}
	return "http"
}

func portForwardError(err error, diagnostics []string) string {
	parts := diagnostics
	if err != nil {
		parts = append(parts, err.Error())
	}
	if len(parts) == 0 {
		return "unknown error"
	}
	return strings.Join(parts, "; ")
}

func IsLocalAddress(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}
