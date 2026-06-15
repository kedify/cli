# Kedify CLI

This repository contains the new `kedify` command-line interface built with [`kong`](https://github.com/alecthomas/kong).

## Current Features

- `kedify login`
  Reads a Kedify API token and stores it in `~/.config/kedify/credentials.json`.
- Hidden token entry in interactive terminals
  When you run `kedify login` directly, the token is not echoed back to the console.
- Piped token input
  You can also provide the token over `stdin`.
- `kedify list clusters`
  Calls the Kedify API and transparently reads all result pages before printing the final cluster list.
- Output formatting
  `kedify list clusters` supports `--output` and `-o` with `json` or `yaml`.

## Build

Build the CLI binary:

```bash
make build
```

This creates:

```bash
./bin/kedify
```

## Authentication

Generate a Kedify API token at:

```text
https://dashboard.dev.kedify.io/api-keys
```

### Interactive Login

```bash
./bin/kedify login
```

The command will prompt for a token and keep the pasted value hidden.

### Piped Login

```bash
printf '%s\n' "$KEDIFY_TOKEN" | ./bin/kedify login
```

Credentials are stored in:

```text
~/.config/kedify/credentials.json
```

## Usage

Show top-level help:

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

Override the API base URL:

```bash
./bin/kedify --apiurl https://api.dev.kedify.io/v1 list clusters
```

Or via environment variable:

```bash
KEDIFY_API_URL=https://api.dev.kedify.io/v1 ./bin/kedify list clusters
```

## Notes

- Cluster pagination is handled internally by the CLI.
- User-facing output for `list clusters` contains only the final cluster list, not pagination metadata.
