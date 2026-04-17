# Architecture

## Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Claude Desktop                           │
│                                                                 │
│   User prompt ──► Claude LLM ──► tool call decision             │
│                        ▲                │                       │
│                        │         MCP client (stdio)             │
└────────────────────────│────────────────│────────────────────── ┘
                         │                │ spawn + stdio pipe
                         │                ▼
┌────────────────────────│────────────────────────────────────────┐
│                   mcp_server.py  (FastMCP)                      │
│                                                                 │
│   tools:                                                        │
│   ├── get_cluster_report   ──► subprocess.run(kubectl-kontext)  │
│   ├── get_current_context  ──► kubectl config current-context   │
│   └── switch_context       ──► kubectl config use-context       │
│                                                                 │
└─────────────────────────────────┬───────────────────────────────┘
                                  │ subprocess
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              kubectl-kontext  (Go binary)                       │
│                                                                 │
│   Phase 1 (parallel):  kubectl get pods/nodes/events            │
│   Phase 2 (parallel):  kubectl get deployments/hpa/rollouts/…   │
│   Phase 3 (sequential): assemble report via jq                  │
│                                                                 │
└─────────────────────────────────┬───────────────────────────────┘
                                  │ kubectl + kubeconfig
                                  ▼
                    ┌─────────────────────────┐
                    │   Kubernetes Cluster    │
                    │   (API server :6443)    │
                    └─────────────────────────┘
```

## Sequence

```mermaid
sequenceDiagram
    actor User
    participant CD as Claude Desktop
    participant MCP as mcp_server.py
    participant KK as kubectl-kontext
    participant K8s as Kubernetes API

    User->>CD: "Analyze my cluster"
    CD->>MCP: tool call: get_cluster_report()
    MCP->>KK: subprocess.run()
    par Phase 1
        KK->>K8s: kubectl get pods -A -o json
        KK->>K8s: kubectl get nodes -o json
        KK->>K8s: kubectl get events -A -o json
    end
    par Phase 2
        KK->>K8s: kubectl get deployments/hpa/rollouts/…
    end
    KK-->>MCP: report (stdout)
    MCP-->>CD: report string
    CD-->>User: analysis + recommendations
```

## Process Lifecycle

Claude Desktop reads `claude_desktop_config.json` on startup and spawns `mcp_server.py`
as a child process connected via **stdio pipe** (not a TCP port). The server stays alive
for the session and handles tool calls on demand. Switching contexts with `switch_context`
modifies the kubeconfig file in place — no server restart needed.

```
claude_desktop_config.json
  └── command: uv run /path/to/mcp_server.py
        └── env: KUBECONFIG=/path/to/.kube/config
```
