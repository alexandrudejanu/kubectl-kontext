# Architecture

## System Overview

`kubectl-kontext` is a single-file Bash kubectl plugin that generates a structured Kubernetes cluster assessment report optimized for AI analysis. It reads cluster state via `kubectl` and `jq`, then emits a plain-text report to stdout designed for piping into Claude, Ollama, or other LLMs.

Current version: `v1.0.3`

---

## Repository Structure

```
kubectl-kontext          Executable Bash script — the entire plugin
Makefile                 Build: tarballs, sha256 checksums, release, clean
plugins/kontext.yaml     Krew plugin manifest (platform selectors + sha256)
.github/
  workflows/
    release-on-tag.yml.disabled   Disabled tag-triggered GitHub release workflow
  copilot-instructions.md
Readme.md                Install instructions and example Claude prompts
CLAUDE.md                Claude Code project instructions
LICENSE
```

No src/ packages or modules; runtime logic is a single Bash script with no generated code.

---

## Core Module

The entire system is `kubectl-kontext` (~526 lines). It has no modules or internal libraries.

**Logical sections inside the script:**

| Phase | What happens |
|-------|-------------|
| Startup | `--help` flag handling; `jq` dependency check; `mktemp -d` temp dir creation |
| Phase 1 | 3 heavy `kubectl get ... -o json` calls run in parallel background jobs |
| Phase 2 | ~15 independent lightweight `kubectl` calls run concurrently |
| Phase 3 | Sequential report assembly from cached temp files using `jq` |
| Cleanup | `trap 'rm -rf "$TMPDIR"' EXIT` ensures temp dir removal on exit |

---

## Key Data Flow

```
kubectl API server
      │
      ├─ Phase 1 (parallel, heavy JSON)
      │     pods.json ──────────────────┐
      │     nodes.json ─────────────────┤
      │     events.json                 │
      │                                 │
      ├─ Phase 2 (parallel, lightweight)│
      │     deployments.json            │
      │     statefulsets.json           ├──► $TMPDIR (temp cache)
      │     daemonsets.json             │
      │     rollouts.json               │
      │     hpa.json                    │
      │     storageclasses.txt          │
      │     pdb.txt, quotas.txt, ...    │
      │     top_nodes.txt, top_pods_*   │
      │                                 │
      └─ Phase 3 (sequential assembly) ◄┘
            jq transforms cached JSON
            column -t for alignment
                  │
                  ▼
             stdout (plain text report)
                  │
       ┌──────────┴──────────┐
       ▼                     ▼
  claude -p '...'       > report.md
  ollama run phi3       pbcopy / tee
```

**Read-once, reuse:** `pods.json` and `nodes.json` are fetched once each and referenced across ~6 and ~4 report sections respectively.

---

## Report Sections (output order)

1. Quick Summary (key metrics for AI)
2. Cluster Overview (`kubectl version`)
3. Nodes — status, roles, kubelet version, allocatable resources
4. Node Conditions — MemoryPressure / DiskPressure / PIDPressure
5. Node Resource Allocation — CPU/memory requests and limits per node (overcommitment %)
6. Cluster-wide Resource Totals
7. Per-namespace Resource Totals
8. Actual Resource Usage (`kubectl top`, requires metrics-server)
9. Workload Readiness — Deployments, StatefulSets, DaemonSets, Argo Rollouts, Istio sidecar count
10. HorizontalPodAutoscalers
11. Pods Without Resource Limits / Requests
12. Top 10 Memory Consumers (by limit)
13. Top 10 Pod Restarts
14. Warning Events (deduplicated by reason, grouped with counts)
15. Problem Pods (Pending, Failed)
16. Storage Classes
17. Pod Disruption Budgets
18. LimitRanges / ResourceQuotas
19. Network Policies
20. Node Taints
21. K3s config (if applicable)

---

## Technology Stack

| Component | Detail |
|-----------|--------|
| Language | Bash (`#!/bin/bash`, `set -euo pipefail`) |
| JSON processing | `jq` (hard dependency, checked at startup) |
| Cluster access | `kubectl` (uses current kubeconfig context) |
| Table formatting | `column -t` (system utility) |
| Build | GNU Make |
| Distribution | kubectl krew custom index |

No compiled code. No runtime packages or lockfiles.

---

## Architectural Patterns

- **Single-file CLI tool** — zero modules, zero config files, zero state
- **Parallel fetch, sequential assemble** — background jobs (`&`) with `wait` gates separate I/O-bound collection from CPU-bound formatting
- **Temp-dir caching** — avoids redundant API calls; all inter-phase data passes through `$TMPDIR`
- **Unix composition** — output is plain text to stdout; designed for pipe chains (`|`, `tee`, `>`)
- **Graceful degradation** — optional resources (Argo Rollouts, HPA, metrics-server) use `|| echo fallback` to avoid hard failures

---

## External Dependencies and Integrations

| Dependency | Type | Required |
|------------|------|----------|
| `kubectl` | CLI tool | Yes — cluster access |
| `jq` | CLI tool | Yes — all JSON processing |
| `column` | System utility | Yes — table alignment |
| `kubectl top` / metrics-server | Cluster add-on | Optional — skips section if absent |
| Argo Rollouts CRD | Cluster add-on | Optional — skips section if absent |
| Istio | Cluster add-on | Optional — counted by container name |
| Claude CLI (`claude`) | External AI | Optional — example usage only |
| Ollama | External AI | Optional — example usage only |
| kubectl krew | Plugin manager | Optional — for krew-based install |

---

## Risk-Sensitive Areas

- **Untracked report files** (`report.md`, `computev2-ovh-*.md`) contain real cluster data and are not gitignored; accidental commit would expose cluster topology.
- **Krew manifest sha256** — `plugins/kontext.yaml` lists the same sha256 for both `darwin-arm64` and `linux-arm64` because the `Makefile` currently packages the same `kubectl-kontext` Bash script into both tarballs; if platform-specific artifacts are introduced later, the manifest and checksums must be updated accordingly.
- **Disabled CI** — `release-on-tag.yml.disabled` is inert; releases require manual `make release` execution.
- **Platform coverage** — Krew manifest covers only `arm64`; `amd64` / `x86_64` platforms are not listed.
- **No input validation** — script accepts no user-supplied input beyond `--help`; all data comes from `kubectl` API responses (read-only operations).
- **RBAC scope** — the plugin issues only read operations (`get`, `top`); it requires cluster-wide read access to pods, nodes, events, deployments, and related resources.
