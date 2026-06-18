# Apply Recommendations

This document describes the v1 design for applying Kedify rightsizing recommendations to a user's Helm chart.

## Goal

The goal is to let CI fetch Kedify recommendations for Kubernetes workloads and generate a safe, reviewable change that can be proposed to the user as a pull request.

For v1, the CLI will:

- discover available recommendations through `kedify list recommendations`
- apply recommendations to Helm values only
- target one workload and one container per invocation
- update up to four resource settings in a single run
- produce machine-readable output and patch artifacts suitable for CI

PR creation is out of scope for the CLI itself. CI can open a PR separately, for example by using `gh`.

## Command Shape

The new command is:

```bash
kedify apply recommendations deployment/<NAME> \
  --namespace <NAMESPACE> \
  --container <CONTAINER> \
  --chart-path <PATH> \
  --values-file <PATH> \
  [--resources cpu-requests,cpu-limits,memory-requests,memory-limits] \
  [--recommendations-file <PATH>] \
  [--min-confidence 60] \
  [--format diff|override|json] \
  [--dry-run]
```

Notes:

- `recommendations` is plural
- the positional target uses workload kind and workload name, for example `deployment/my-app`
- `--namespace` is required
- `--container` is required
- `--chart-path` is required in Helm mode
- `--values-file` is required in Helm mode
- `--resources` is optional

## Resource Selection

Supported resource identifiers are:

- `cpu-requests`
- `cpu-limits`
- `memory-requests`
- `memory-limits`

The `--resources` flag accepts a comma-separated list:

```bash
--resources cpu-requests,cpu-limits
```

If `--resources` is omitted, the CLI should apply all recommendations available for the selected workload and container.

At most four recommendations can be applied in one invocation, one for each supported resource identifier.

## Recommendation Sources

The command can consume recommendations from two sources:

1. Live Kedify API
2. A previously saved recommendations file

When `--recommendations-file` is provided, the command should read recommendations from that file instead of fetching them from the API.

When no file is provided, the command should internally fetch recommendations in the same spirit as `kedify list recommendations` and then identify the selected recommendation by:

- workload kind
- workload name
- namespace
- container
- resource type

Only recommendations with status `waiting` are considered applicable.

## Confidence Threshold

Confidence filtering is controlled by:

```bash
--min-confidence <N>
```

Rules:

- the threshold is inclusive
- the default value is `60`
- recommendations below the threshold are not applied

## Helm v1 Scope

v1 is Helm-first.

The command should:

- render the chart as part of the workflow
- patch exactly one file provided through `--values-file`
- update Helm values only

The command should not:

- patch rendered manifests directly
- patch Helm templates directly
- patch multiple values files in one run

## Matching Strategy

The v1 matching strategy is heuristic.

The CLI should:

- render the Helm chart using `--chart-path` and `--values-file`
- find the rendered workload matching the requested kind, name, and namespace
- find the matching container by explicit container name
- scan templates and values usage for matching container resource blocks

Rules:

- patch only when container mapping is explicit
- do not guess when multiple possible mappings exist
- fail when the recommendation cannot be mapped safely to the values file

## Ambiguity and Safety

Default ambiguity behavior is strict.

If the CLI cannot safely map a recommendation to the values file, it should fail.

Examples of failure conditions:

- no matching recommendation found
- rendered workload not found
- container not found
- workload kind unsupported by the patcher
- resources are not exposed through values in an explicit way
- multiple candidate mappings exist

Unsupported workload kinds should be reported in output even when they are not patchable in v1.

## Output Formats

The command supports these output formats:

- `diff`
- `override`
- `json`

### `diff`

Emit a unified diff showing the change that would be made to the values file.

### `override`

Emit a small generated Helm override values file containing only the recommended changes.

### `json`

Emit machine-readable patch plan or patch result JSON.

The JSON output is not just raw recommendation data. It should describe what the patcher tried to do and what happened.

## Dry Run

`--dry-run` means:

- do not write files
- do not mutate the workspace
- still emit the selected output format

Examples:

- `--format diff --dry-run` prints the diff only
- `--format override --dry-run` prints or emits the generated override content without writing it
- `--format json --dry-run` prints the patch plan/result JSON without writing changes

## Machine-Readable Result Semantics

The JSON result should expose stable reason codes so CI can react consistently.

At minimum, results should report these states:

- `matched`
- `patched`
- `ambiguous`
- `unsupported`
- `not_found`
- `below_confidence_threshold`

These codes should be stable and suitable for CI logic.

## Exit Codes

Expected v1 behavior:

- exit `0` when patch generation succeeds
- exit non-zero when no safe mapping is found
- exit non-zero when the selected recommendation cannot be applied safely

This is intended to make CI fail fast when the recommendation cannot be converted into a safe patch.

## Example CI Flow

The happy path in CI is:

1. Run Kedify recommendation discovery.
2. Select a workload, namespace, container, and resource set.
3. Run `kedify apply recommendations ...`.
4. Generate either a diff, override file, or JSON patch result.
5. Commit the resulting change in CI.
6. Open a pull request with an external tool such as `gh`.

## Non-Goals for v1

The following are out of scope for v1:

- patching arbitrary raw YAML or Kustomize sources
- patching Helm templates directly
- editing more than one values file per invocation
- automatic PR creation inside the CLI
- applying recommendations when resource-to-values mapping is ambiguous

## Future Direction

After Helm v1, the same recommendation model should eventually support broader Kubernetes source layouts, including:

- Helm charts beyond simple explicit values mapping
- plain YAML manifests
- Kustomize-based repositories
- additional workload kinds such as Jobs or StatefulSets when patching support is implemented
