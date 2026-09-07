# Kedify CLI

`kedify` is a command-line interface for working with the Kedify API from your terminal.

The CLI currently focuses on authentication, cluster inspection, and applying recommendation data to Helm values files.

## Features

- `kedify auth login`
  Reads a Kedify API token and stores it in the OS credential store when available, with a file fallback.
- `kedify auth token`
  Prints the current auth token to stdout.
- Interactive hidden token entry
  When run in a terminal, `auth login` uses a Bubble Tea prompt and keeps the token hidden.
- Piped token input
  You can also provide a token via `stdin`.
- CI-friendly token injection
  Commands can also use `--token` or `KEDIFY_TOKEN` instead of the stored credentials file.
- `kedify list clusters`
  Calls the Kedify API and transparently reads all pages before printing the final cluster list.
- `kedify get cluster [name-or-id]`
  Prints one cluster by name or id, and shows an interactive picker when no name is provided.
- `kedify delete cluster [name-or-id]`
  Deletes one cluster by name or id, and shows an interactive picker when no name is provided.
- `kedify list recommendations <cluster-id>`
  Prints the recommendations payload for a cluster id.
- `kedify apply recommendations <kind/name>`
  Applies recommendations from a saved JSON or YAML file to a Helm values file and can emit `json`, `diff`, or `override` output.
- `kedify analyze recommendations <snapshot-request>`
  Runs a separately installed `kedify-analyzer` against a versioned normalized snapshot without using Kedify SaaS.
- `kedify metrics`
  Opens an interactive Prometheus metric explorer, builds and validates a PromQL query, previews it as an ASCII graph, and generates YAML for a `ScaledObject`, a `MetricPredictor`, or both. Creating the resources in Kubernetes is an explicit opt-in.
- Output formatting
  `kedify list clusters`, `kedify get cluster`, and `kedify list recommendations` support `-o` and `--output` with `text`, `json`, or `yaml`. `text` is the default. `kedify delete cluster` prints its confirmation message to `stderr` and keeps `stdout` empty for shell-friendly usage.

## Build

Build the CLI locally with:

```bash
make build
```

The binary will be available at `./bin/kedify`.

### Requirements

- Go toolchain version from `go.mod`
- `make`
- `kubectl` when using Prometheus discovery, port-forwarding, or resource creation

### Offline recommendation analysis

The offline command consumes a versioned normalized snapshot request and prints the
analyzer's JSON result unchanged. The request envelope is:

```json
{
  "protocolVersion": "kedify-analyzer/v1",
  "input": {
    "schemaVersion": "resource-analysis-input/v1",
    "observedIntervalHours": 24,
    "containers": []
  },
  "policy": {}
}
```

Run it from a file or standard input:

```bash
./bin/kedify analyze recommendations ./snapshot-request.json
cat ./snapshot-request.json | ./bin/kedify analyze recommendations -
```

Install a matching `kedify-analyzer` release before entering an air-gapped
environment and verify its published SHA-256 checksum. The CLI resolves an explicit
`--analyzer` path first, then `kedify-analyzer` beside the CLI executable, then
`kedify-analyzer` on `PATH`:

```bash
./bin/kedify analyze recommendations ./snapshot-request.json \
  --analyzer ./tools/kedify-analyzer
```

The command requires no Kedify token, makes no SaaS request, and never downloads an
analyzer. It accepts only protocol `kedify-analyzer/v1`, input/output schemas
`resource-analysis-input/v1` and `resource-analysis-output/v1`, and engine version
`1`. CPU `aggregatedUsage` in the request must already reflect the policy's `max` or
`percentile` selection. Analyzer diagnostics stay on `stderr`; the validated JSON
result is the only `stdout` output.

## Authentication

Generate a Kedify API token at:

```text
https://dashboard.dev.kedify.io/api-keys
```

The CLI stores credentials in:

```text
OS credential store when available
~/.config/kedify/credentials.json as fallback
```

Interactive login:

```bash
./bin/kedify auth login
```

Login with a global token flag:

```bash
./bin/kedify --token "$KEDIFY_TOKEN" auth login
```

Login with a positional token argument:

```bash
./bin/kedify auth login "$KEDIFY_TOKEN"
```

Print the current token:

```bash
./bin/kedify auth token
```

Piped login:

```bash
printf '%s\n' "$KEDIFY_TOKEN" | ./bin/kedify auth login
```

## Usage

Show help:

```bash
./bin/kedify --help
```

List clusters in the default human-readable text format:

```bash
./bin/kedify list clusters
```

List clusters as YAML:

```bash
./bin/kedify list clusters -o yaml
```

Get a cluster by name:

```bash
./bin/kedify get cluster my-cluster
```

Get a cluster as JSON:

```bash
./bin/kedify get cluster my-cluster -o json
```

Delete a cluster by name:

```bash
./bin/kedify delete cluster my-cluster
```

Delete a cluster by UUID:

```bash
./bin/kedify delete cluster fc6af0dc-685b-4055-805d-0d3e0ead1596
```

List recommendations for a cluster as JSON:

```bash
./bin/kedify list recommendations fc6af0dc-685b-4055-805d-0d3e0ead1596 -o json
```

Apply recommendations to a Helm values file and print the patch plan as JSON:

```bash
./bin/kedify apply recommendations deployment/my-app \
  --namespace my-namespace \
  --chart-path ./chart \
  --values-file ./chart/values.yaml \
  --recommendations-file ./recommendations.json \
  --resources cpu-requests,memory-limits \
  --format json \
  --dry-run
```

Apply recommendations and write an override file:

```bash
./bin/kedify apply recommendations deployment/my-app \
  --namespace my-namespace \
  --chart-path ./chart \
  --values-file ./chart/values.yaml \
  --recommendations-file ./recommendations.json \
  --resources cpu-requests,memory-limits \
  --format override \
  --output-file ./override-values.yaml
```

Explore metrics using Kubernetes service discovery:

```bash
./bin/kedify metrics
```

Connect directly to Prometheus and generate manifests for a specific namespace:

```bash
./bin/kedify metrics \
  --server=http://localhost:9090 \
  --namespace=my-app
```

Preselect parts of the interactive flow:

```bash
# Discover Prometheus, use the first matching service, and port-forward it.
./bin/kedify metrics --disco --filter=memory_

# Use a specific kubectl context and kubeconfig for discovery and port-forwarding.
./bin/kedify metrics \
  --disco \
  --context=staging \
  --kubeconfig=./config/staging.kubeconfig

# Validate this query, select a horizon, and visualize it.
./bin/kedify metrics \
  --server=http://localhost:9090 \
  --query='sum(foobar)' \
  --visualize

# Skip both choices and immediately load the 3-day graph.
./bin/kedify metrics \
  --server=http://localhost:9090 \
  --query='sum(foobar)' \
  --visualize \
  --horizon=3d \
  --print
```

`--server` and `--disco` are mutually exclusive. `--visualize` requires `--query`, and `--horizon` requires `--visualize`. Supported horizon values are `6h`, `1d`, `3d`, `1w`, and `30d`. `--context` and `--kubeconfig` are forwarded to every `kubectl` invocation made by the metrics command. A value passed through `--filter` remains editable in the metric browser. Before generated resources are printed, the CLI offers to open a line-numbered YAML editor; `--print` skips this final question and prints directly.

The metrics explorer:

- discovers Prometheus services and Mimir gateways from the active kubeconfig using common labels and can manage a temporary `kubectl port-forward`; Mimir API requests use its `/prometheus` prefix and `X-Scope-OrgID: kedify-agent`;
- retrieves metric names with the `{__name__=~".+"}` selector;
- supports arrow and Page Up/Page Down navigation, plus case-insensitive substring filtering as you type;
- shows label names and values as you drill into a metric;
- starts from a single-series `sum(metric{label="value"})` expression and lets you edit and validate the final PromQL;
- can graph the last 6 hours, day, 3 days, week, or month of samples in the terminal, using a range-appropriate Prometheus query step;
- generates a `kedify-otel` `ScaledObject` with a default target value of `1`, and prompts for its scale target;
- generates a `MetricPredictor` with a one-week Prometheus range at a `30s` step so it has historical samples for initial training;
- prints the selected resource manifests by default and only runs `kubectl create` when the creation checkbox is enabled and a final review of the exact YAML, active Kubernetes context, and target namespace is confirmed.

Interactive terminal UX, progress, and optional creation confirmations are written to `stderr`. If no resource manifest is selected, the completed PromQL expression is printed to `stdout`; otherwise, the generated YAML is printed to `stdout`.

Notes for `apply recommendations`:

- The command is Helm-only in v1.
- `--recommendations-file`, `--chart-path`, and `--values-file` are required.
- `--container` is optional. If omitted, the CLI matches all recommendation-bearing containers in the workload.
- All matched containers must be safely patchable for the run to succeed.
- `--output-file` is required for `--format override` unless `--dry-run` is set.
- JSON output includes top-level `containers` and per-entry `container` fields for multi-container runs.

Pick a cluster interactively:

```bash
./bin/kedify get cluster
```

Pick a cluster interactively and delete it:

```bash
./bin/kedify delete cluster
```

Override the API URL:

```bash
./bin/kedify --apiurl https://api.dev.kedify.io/v1 list clusters
```

Or with an environment variable:

```bash
KEDIFY_API_URL=https://api.dev.kedify.io/v1 ./bin/kedify list clusters
```

Pass the auth token explicitly in CI:

```bash
./bin/kedify --token "$KEDIFY_TOKEN" list clusters
```

Or via environment variable:

```bash
KEDIFY_TOKEN="$KEDIFY_TOKEN" ./bin/kedify get cluster my-cluster
```

## Development Notes

- The CLI keeps command output on `stdout` so it remains script-friendly.
- Interactive prompts and terminal UX are sent to `stderr`.
- Paginated API responses are read across all pages automatically before output is printed.

## License

Licensed under the Apache License v2.0. See [LICENSE](LICENSE).
