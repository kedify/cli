package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/kubernetes"
	metricresources "github.com/kedify/cli/internal/metrics"
	"github.com/kedify/cli/internal/prometheus"
	"github.com/kedify/cli/internal/tui"
)

const requestTimeout = 30 * time.Second

var dnsNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)

type MetricsCmd struct {
	Server     string `name:"server" help:"Prometheus server URL. When omitted, choose Kubernetes discovery or enter a URL interactively." placeholder:"URL" xor:"prometheus-source"`
	Disco      bool   `name:"disco" help:"Discover Prometheus in Kubernetes, select the first service, and start port-forwarding without prompting." xor:"prometheus-source"`
	Context    string `name:"context" help:"Kubernetes context forwarded to kubectl for discovery, port-forwarding, and resource creation." placeholder:"NAME"`
	Kubeconfig string `name:"kubeconfig" help:"Kubeconfig path forwarded to kubectl for discovery, port-forwarding, and resource creation." placeholder:"PATH"`
	Namespace  string `name:"namespace" short:"n" help:"Namespace for resources created at the end of the session. Defaults to the active kubeconfig namespace."`
	Filter     string `name:"filter" help:"Initial editable substring filter for the metric list." placeholder:"TEXT"`
	Query      string `name:"query" help:"PromQL query to validate instead of opening the metric selector." placeholder:"PROMQL"`
	Visualize  bool   `name:"visualize" help:"With --query, skip the visualize-or-continue prompt and select a time horizon."`
	Horizon    string `name:"horizon" help:"With --query and --visualize, immediately graph a 6h, 1d, 3d, 1w, or 30d horizon." placeholder:"DURATION"`
	Print      bool   `name:"print" help:"Print generated YAML without asking whether to edit it first."`
}

type prometheusEndpoint struct {
	sessionURL string
	clusterURL string
	mimir      bool
}

type resourceCreator interface {
	Create(context.Context, []byte) ([]string, error)
}

func (c *MetricsCmd) Run(app *clictx.Context) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := tui.RequireInteractive(app.Stdin); err != nil {
		return err
	}

	kubeClient := kubernetes.NewClientWithOptions(kubernetes.ClientOptions{
		Context:    c.Context,
		Kubeconfig: c.Kubeconfig,
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint, portForward, err := c.resolveServer(runCtx, app, kubeClient)
	if err != nil {
		return err
	}
	if portForward != nil {
		defer func() {
			_ = portForward.Stop()
		}()
	}

	var client *prometheus.Client
	if endpoint.mimir {
		client, err = prometheus.NewMimirClient(endpoint.sessionURL)
	} else {
		client, err = prometheus.NewClient(endpoint.sessionURL)
	}
	if err != nil {
		return err
	}

	var metricNames []string
	if strings.TrimSpace(c.Query) == "" {
		metricNames, err = tui.RunWithSpinner(
			app.Stderr,
			fmt.Sprintf("Loading metrics from %s…", endpoint.sessionURL),
			func() ([]string, error) {
				requestCtx, requestCancel := context.WithTimeout(runCtx, requestTimeout)
				defer requestCancel()
				return client.MetricNames(requestCtx)
			},
		)
		if err != nil {
			return err
		}
		if len(metricNames) == 0 {
			return errors.New("Prometheus returned no active metrics for selector {__name__=~\".+\"}")
		}
	}

	result, err := tui.RunMetricsExplorer(
		app.Stdin,
		app.Stdout,
		app.Stderr,
		client,
		metricNames,
		tui.MetricsExplorerOptions{
			Filter:    c.Filter,
			Query:     c.Query,
			Visualize: c.Visualize,
			Horizon:   c.Horizon,
		},
	)
	if err != nil {
		return err
	}
	if !result.GenerateScaledObject && !result.GenerateMetricPredictor {
		_, err := fmt.Fprintln(app.Stdout, result.Query)
		return err
	}

	namespace := strings.TrimSpace(c.Namespace)
	if namespace == "" {
		namespaceCtx, namespaceCancel := context.WithTimeout(runCtx, requestTimeout)
		namespace = kubeClient.CurrentNamespace(namespaceCtx)
		namespaceCancel()
	}
	if err := validateKubernetesName(namespace); err != nil {
		return fmt.Errorf("invalid namespace %q: %w", namespace, err)
	}

	options := metricresources.ResourceOptions{
		Query:         result.Query,
		PrometheusURL: endpoint.clusterURL,
		Namespace:     namespace,
	}
	if result.GenerateScaledObject {
		options.ScaledObjectName, err = tui.PromptText(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"ScaledObject name",
			"The resource will be created in namespace "+namespace+".",
			"metric-scaler",
			validateKubernetesName,
		)
		if err != nil {
			return err
		}
		options.ScaleTargetName, err = tui.PromptText(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"Scale target name",
			"Name of the Deployment or other scalable workload referenced by spec.scaleTargetRef.",
			options.ScaledObjectName,
			validateKubernetesName,
		)
		if err != nil {
			return err
		}
	}

	if result.GenerateMetricPredictor {
		rangeEnd := time.Now().UTC()
		options.PrometheusStart = rangeEnd.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
		options.PrometheusEnd = rangeEnd.Format(time.RFC3339)
		options.PrometheusStep = "30s"
		options.MetricPredictorName, err = tui.PromptText(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"MetricPredictor name",
			"The resource will be created in namespace "+namespace+".",
			"metric-predictor",
			validateKubernetesName,
		)
		if err != nil {
			return err
		}
		if kubernetes.IsLocalAddress(options.PrometheusURL) {
			options.PrometheusURL, err = tui.PromptText(
				app.Stdin,
				app.Stdout,
				app.Stderr,
				"Prometheus URL for MetricPredictor",
				"This URL must be reachable from inside the Kubernetes cluster; localhost usually is not.",
				options.PrometheusURL,
				validatePrometheusURL,
			)
			if err != nil {
				return err
			}
		}
	}

	manifest, err := metricresources.BuildResources(options)
	if err != nil {
		return err
	}
	manifest, err = c.editManifestIfRequested(app, manifest)
	if err != nil {
		return err
	}
	createResources := result.CreateResources
	if createResources {
		contextCtx, contextCancel := context.WithTimeout(runCtx, requestTimeout)
		currentContext, contextErr := kubeClient.CurrentContext(contextCtx)
		contextCancel()
		if contextErr != nil {
			return contextErr
		}
		createResources, err = tui.ConfirmResourceCreation(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			string(manifest),
			currentContext,
			namespace,
		)
		if err != nil {
			return err
		}
	}
	return outputResources(
		runCtx,
		app.Stdout,
		app.Stderr,
		kubeClient,
		namespace,
		manifest,
		createResources,
	)
}

func (c *MetricsCmd) editManifestIfRequested(app *clictx.Context, manifest []byte) ([]byte, error) {
	if c.Print {
		return manifest, nil
	}

	edit, err := tui.Confirm(
		app.Stdin,
		app.Stdout,
		app.Stderr,
		"Edit generated YAML before printing?",
		"The edited YAML will also be used if 'create in k8s cluster' is enabled.",
		false,
	)
	if err != nil {
		return nil, err
	}
	if !edit {
		return manifest, nil
	}

	edited, err := tui.EditYAML(app.Stdin, app.Stdout, app.Stderr, string(manifest))
	if err != nil {
		return nil, err
	}
	return []byte(edited), nil
}

func (c *MetricsCmd) Validate() error {
	if strings.TrimSpace(c.Server) != "" && c.Disco {
		return errors.New("--server and --disco cannot be used together")
	}
	if c.Visualize && strings.TrimSpace(c.Query) == "" {
		return errors.New("--visualize requires --query")
	}
	if horizon := strings.TrimSpace(c.Horizon); horizon != "" {
		if !c.Visualize {
			return errors.New("--horizon requires --visualize")
		}
		switch horizon {
		case "6h", "1d", "3d", "1w", "30d":
		default:
			return fmt.Errorf("unsupported --horizon %q; use 6h, 1d, 3d, 1w, or 30d", horizon)
		}
	}
	return nil
}

func outputResources(
	ctx context.Context,
	stdout, stderr io.Writer,
	creator resourceCreator,
	namespace string,
	manifest []byte,
	create bool,
) error {
	written, err := stdout.Write(manifest)
	if err != nil {
		return err
	}
	if written != len(manifest) {
		return io.ErrShortWrite
	}
	if !create {
		return nil
	}

	_, _ = fmt.Fprintf(stderr, "Creating resources in namespace %s…\n", namespace)
	createCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	created, err := creator.Create(createCtx, manifest)
	cancel()
	if err != nil {
		return err
	}
	for _, resource := range created {
		if _, err := fmt.Fprintf(stderr, "Created %s\n", resource); err != nil {
			return err
		}
	}
	return nil
}

func (c *MetricsCmd) resolveServer(
	ctx context.Context,
	app *clictx.Context,
	kubeClient *kubernetes.Client,
) (prometheusEndpoint, *kubernetes.PortForward, error) {
	if strings.TrimSpace(c.Server) != "" {
		server, err := prometheus.NormalizeURL(c.Server)
		if err != nil {
			return prometheusEndpoint{}, nil, err
		}
		return directPrometheusEndpoint(server), nil, nil
	}

	if !c.Disco {
		choice, err := tui.Select(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"Connect to Prometheus",
			"Use the active kubeconfig to discover a service, or enter a URL.",
			[]string{"Discover in Kubernetes", "Enter Prometheus URL"},
		)
		if err != nil {
			return prometheusEndpoint{}, nil, err
		}
		if choice == 1 {
			endpoint, err := promptServerURL(app)
			return endpoint, nil, err
		}
	}

	services, err := tui.RunWithSpinner(
		app.Stderr,
		"Discovering Prometheus services with the active kubeconfig…",
		func() ([]kubernetes.PrometheusService, error) {
			discoveryCtx, cancel := context.WithTimeout(ctx, requestTimeout)
			defer cancel()
			return kubeClient.DiscoverPrometheusServices(discoveryCtx)
		},
	)
	if err != nil {
		return prometheusEndpoint{}, nil, err
	}
	if len(services) == 0 {
		if c.Disco {
			return prometheusEndpoint{}, nil, errors.New("no Kubernetes services matched a known Prometheus label selector")
		}
		_, _ = fmt.Fprintln(app.Stderr, "No services matched a known Prometheus label selector; enter the server URL instead.")
		endpoint, promptErr := promptServerURL(app)
		return endpoint, nil, promptErr
	}

	selectedIndex := 0
	if !c.Disco {
		options := make([]string, len(services))
		for index, service := range services {
			options[index] = service.DisplayName() + "  (" + service.MatchedLabels + ")"
		}
		var err error
		selectedIndex, err = tui.Select(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"Select Prometheus service",
			"Services matching known Prometheus labels.",
			options,
		)
		if err != nil {
			return prometheusEndpoint{}, nil, err
		}
	}
	service := services[selectedIndex]

	if !c.Disco {
		approved, err := tui.Confirm(
			app.Stdin,
			app.Stdout,
			app.Stderr,
			"Start a kubectl port-forward?",
			fmt.Sprintf("%s will be forwarded to a random local port for this session.", service.DisplayName()),
			true,
		)
		if err != nil {
			return prometheusEndpoint{}, nil, err
		}
		if !approved {
			endpoint, promptErr := promptServerURL(app)
			return endpoint, nil, promptErr
		}
	}

	portForward, err := tui.RunWithSpinner(
		app.Stderr,
		fmt.Sprintf("Starting port-forward to %s…", service.DisplayName()),
		func() (*kubernetes.PortForward, error) {
			return kubeClient.StartPortForward(ctx, service)
		},
	)
	if err != nil {
		return prometheusEndpoint{}, nil, err
	}
	return prometheusEndpoint{
		sessionURL: portForward.LocalURL,
		clusterURL: service.ClusterURL(),
		mimir:      service.Mimir,
	}, portForward, nil
}

func promptServerURL(app *clictx.Context) (prometheusEndpoint, error) {
	server, err := tui.PromptText(
		app.Stdin,
		app.Stdout,
		app.Stderr,
		"Prometheus server URL",
		"Example: http://localhost:9090",
		"http://localhost:9090",
		validatePrometheusURL,
	)
	if err != nil {
		return prometheusEndpoint{}, err
	}
	normalized, err := prometheus.NormalizeURL(server)
	if err != nil {
		return prometheusEndpoint{}, err
	}
	return directPrometheusEndpoint(normalized), nil
}

func directPrometheusEndpoint(server string) prometheusEndpoint {
	return prometheusEndpoint{
		sessionURL: server,
		clusterURL: server,
		mimir:      strings.HasSuffix(strings.TrimRight(server, "/"), "/prometheus"),
	}
}

func validatePrometheusURL(value string) error {
	_, err := prometheus.NormalizeURL(value)
	return err
}

func validateKubernetesName(value string) error {
	if len(value) == 0 {
		return errors.New("name is required")
	}
	if len(value) > 253 {
		return errors.New("name must be at most 253 characters")
	}
	if !dnsNamePattern.MatchString(value) {
		return errors.New("use lowercase letters, numbers, and hyphens; start and end with a letter or number")
	}
	return nil
}
