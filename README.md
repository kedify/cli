# Kedify CLI

This repository contains an experimental `kedify` CLI built with `bubbletea` and `lipgloss` for the interactive login flow.

## Current Features

- `kedify login`
  Reads a Kedify API token and stores it in `~/.config/kedify/credentials.json`.
- Interactive hidden token entry
  When run in a terminal, `login` uses a Bubble Tea prompt and keeps the token hidden.
- Piped token input
  You can also provide a token via `stdin`.
- `kedify list clusters`
  Calls the Kedify API and transparently reads all pages before printing the final cluster list.
- `kedify get cluster [name]`
  Prints one cluster by name or id, and shows an interactive picker when no name is provided.
- Output formatting
  `kedify list clusters` and `kedify get cluster` support `-o` and `--output` with `json` or `yaml`.

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
./bin/kedify login
```

Piped login:

```bash
printf '%s\n' "$KEDIFY_TOKEN" | ./bin/kedify login
```

Credentials are stored in:

```text
~/.config/kedify/credentials.json
```

## Usage

Show help:

```bash
./bin/kedify --help
```

List clusters as JSON:

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
