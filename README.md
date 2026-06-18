# Kedify CLI

This repository contains an experimental `kedify` CLI built with `kong` for command parsing and `bubbletea` plus `lipgloss` for interactive terminal flows.

## Current Features

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
- `kedify list recommendations <cluster-id>`
  Prints the recommendations payload for a cluster id.
- Output formatting
  `kedify list clusters`, `kedify get cluster`, and `kedify list recommendations` support `-o` and `--output` with `text`, `json`, or `yaml`. `text` is the default.

## Build

```bash
make build
```

The binary will be available at `./bin/kedify`.

## Authentication

Generate a Kedify API token at:

```text
https://dashboard.dev.kedify.io/api-keys
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

Credentials are stored in:

```text
OS credential store when available
~/.config/kedify/credentials.json as fallback
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

List recommendations for a cluster as JSON:

```bash
./bin/kedify list recommendations fc6af0dc-685b-4055-805d-0d3e0ead1596 -o json
```

Pick a cluster interactively:

```bash
./bin/kedify get cluster
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
