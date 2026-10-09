# Copilot Instructions for `kubectl-kontext`

## Repository overview

This repository is not a pure single-script project. It is a hybrid Kubernetes cluster-analysis project with both Bash and Go artifacts in active use:

- `kubectl-kontext` is the shell-based kubectl plugin that gathers cluster data (`kubectl`, `jq`) and emits a markdown assessment report.
- `cmd/` and `internal/` indicate a Go-based implementation or build path that complements the shell script.
- `local_mcp_setup/` includes MCP server tooling and Claude integration setup.
- `plugins/kontext.yaml` contains Krew distribution metadata.
- `Makefile` handles build, packaging, and MCP installation tasks in addition to release packaging.

When making changes, prefer the repository's actual structure over assumptions. The shell script remains the most obvious end-user plugin path, but Go tooling and MCP integration are part of the current codebase.

## Primary code paths

- `kubectl-kontext`: primary shell plugin; contains the cluster assessment logic, caching, and markdown report assembly.
- `cmd/`: Go entrypoints / CLI build targets.
- `internal/`: internal Go modules or supporting code for Go-based runtime paths.
- `Makefile`: packaging, Go build, install, and MCP/Claude setup tasks.
- `plugins/kontext.yaml`: Krew plugin metadata and platform-specific download definitions.
- `local_mcp_setup/`: local MCP configuration and example setup scripts for Claude integration.

## Architecture expectations

- Keep the existing reporting flow explicit and sectioned.
- Preserve the current performance pattern when working in the shell script:
  - fetch heavy data once and cache it
  - run independent `kubectl` calls in parallel when possible
  - assemble final markdown output from cached data
- Do not assume the codebase is Bash-only; if you add functionality that should also exist in the Go path, implement it in the appropriate Go module instead of silently changing only the shell script.
- Keep runtime/report logic separated from packaging and distribution metadata. Packaging and plugin metadata belong in `Makefile` and `plugins/kontext.yaml`, not in the report engine.

## Coding conventions

- Follow Bash strict mode exactly in the shell script: `set -euo pipefail`.
- Use Bash-specific conditionals already present in the project: `[[ ... ]]` and `${1:-}` patterns.
- Quote paths and variables (`"$TMPDIR"`, `"$PODS_JSON"`, etc.).
- Keep global script variables upper-case snake case where applicable.
- Preserve section comments and report headers (for example `## CLUSTER OVERVIEW`, `## WORKLOAD READINESS`, etc.) to keep output stable and readable for AI consumers.
- Use null-safe `jq` filters when parsing optional data (`//`, `?`, guarded parsing helpers).
- Preserve explicit fallback handling for optional APIs/resources, such as Rollouts, HPA, metrics-server output, and missing cluster features.
- Do not add broad, unbounded API calls when cached outputs are sufficient.

## Module boundaries

- `kubectl-kontext` is the user-facing report generator for cluster analysis.
- `Makefile` is not just release metadata; it also drives Go builds and MCP installation.
- `plugins/kontext.yaml` is Krew metadata only.
- `cmd/` and `internal/` are code modules and should not be treated as absent simply because the shell script is prominent.
- `local_mcp_setup/` supports local MCP integration and should be kept consistent with `Makefile` wiring.

Boundary rules:

- Do not move report-generation logic into packaging or distribution files.
- Do not add release concerns into the report sections.
- If a change affects report content, keep the same markdown/output structure and section ordering unless the change is intentional.

## Dependency guidelines

Required runtime dependencies observed in the repo:

- `kubectl`
- `jq`
- Go 1.21 for Go-based tooling (`go.mod`)

Optional or integration-specific dependencies observed:

- metrics-server (`kubectl top` output)
- `claude` CLI for usage examples and MCP setup
- Python + `uv` for local MCP server execution in `local_mcp_setup/`

Keep runtime behavior compatible with the repo's current tools and ecosystem. Do not introduce new language/framework dependencies unless the repository maintainers intentionally change architecture.

## Testing guidelines

Current repo state:

- There is no clear unit test suite dedicated to the shell script.
- Go tooling is present, but there is no guarantee a Go test harness is active everywhere.
- No broad coverage thresholds are configured.

Until automated tests are added or the project establishes a test framework, validate changes with manual checks:

- `./kubectl-kontext --help`
- `./kubectl-kontext` against a reachable cluster
- Validate behavior when optional features are missing (metrics-server absent, HPA absent, Rollouts absent)
- If you alter parsing or output structure, verify the critical sections still render and produce valid output rather than crashing
- If Go-related code changes are made, run the project-relevant Go build command from `Makefile` when practical

## Anti-patterns to avoid

- Avoid adding new expensive `kubectl get ... -o json` calls during final report assembly when cached data is already available.
- Avoid unguarded stderr suppression without explicit fallback or user-visible handling.
- Avoid duplicating complex parsing logic when a shared helper or established pattern can be reused safely.
- Avoid assuming the repo is Bash-only; verify whether a Go path is relevant before editing.
- Avoid inconsistent output wording or section naming in generated report text.
- Avoid modifying build/install tooling in ways that drift from the repo's actual toolchain and local MCP setup.

## Preferred workflow for contributions

- Prefer small, targeted edits that respect the current shell-script performance model.
- When editing cluster report logic, keep the output deterministic and readable.
- When editing packaging or build workflows, respect the actual build targets present in `Makefile`.
- When editing MCP or Claude integration, ensure the local setup files and `Makefile` commands remain aligned.
- Favor repo-native conventions over introducing new abstractions that do not match the current architecture.
