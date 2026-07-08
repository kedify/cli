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
