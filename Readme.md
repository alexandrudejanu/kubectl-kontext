# kubectl-kontext

Kubernetes cluster assessment plugin optimized for AI analysis, and [why this plugin exists](https://medium.com/@dejanualex/kubectl-kontext-48f2cfda1e03) .

**Summary-first** `summary → metrics → details`  design, which matches how AI models process information most effectively. 
 
How it works (3 phases):                                                                                                                                                                                                              
  1. Fetch heavy JSON data in parallel (pods, nodes, events) from the cluster
  2. Run ~15 independent lightweight kubectl calls concurrently
  3. Assemble the summary sequentially from cached data using jq


| Section | Purpose |
|---------|---------|
| Quick Summary | Key metrics for AI |
| Nodes | Count, resources, capacity |
| Node Conditions | MemoryPressure / DiskPressure / PIDPressure per node |
| Resource Allocation | CPU/memory per node (overcommitment %) |
| Cluster-wide Resource Totals | Total allocatable CPU and memory |
| Per-namespace Resource Totals | CPU/memory requests and limits per namespace (capacity planning) |
| Actual Resource Usage | Live usage via `kubectl top` (requires metrics-server) |
| Workload Readiness | Deployments, StatefulSets, DaemonSets, Argo Rollouts, Istio |
| HorizontalPodAutoscalers | Min/max/current replicas, utilization targets, HPAs at max |
| Pods Without Limits/Requests | Resource governance gaps |
| Top Memory Consumers | Heavy workloads |
| Top Pod Restarts | Stability issues |
| Warning Events | Active problems (deduplicated by reason) |
| Problem Pods | Pending and failed pods |
| PDBs/LimitRanges/Quotas | Resource policies |
| Network Policies | Security posture |
| Node Taints | Scheduling controls |

## Install

```bash
# Place kubectl-kontext in your path
export PATH="$PATH:$(pwd)" # or cp kubectl-kontext /usr/local/bin/

# Install kubectl-kontext from index
kubectl krew index add my-index https://github.com/dejanu/kubectl-kontext.git

kubectl krew search my-index

kubectl krew install my-index/kontext
```

## Usage: leveraging Unix composition

* Claude code in headless mode

```bash
# copy to Clipboard 
kubectl kontext | pbcopy 

# Quick assessment
kubectl kontext | claude --model sonnet -p 'List critical issues and recommendations'

kubectl kontext | claude -p 'Analyze this cluster. Prioritize issues by severity (critical/high/medium/low). For each issue provide: problem, impact, fix.' | tee analysis.md

kubectl kontext | claude -p 'Is this cluster over-provisioned? Identify idle or wasted resources and suggest rightsizing.'

# Capacity planning — save report first so Claude and the file use the same snapshot
kubectl kontext > report.md && cat report.md | claude --model sonnet -p '
## Capacity Planning Analysis
### Cluster: <cluster name from report> | <date from report>

---

Produce exactly three sections. Use only data present in the report.
Do not re-sum tables the report has already totalled — read CLUSTER-WIDE
RESOURCE TOTALS directly for allocatable figures.

### Node Utilisation
Table with one row per node from NODE RESOURCE ALLOCATION:
| Node | CPU Req % | CPU Lim % | Mem Req % | Mem Lim % | Status |
Status = "BLOCKED" if CPU or Mem req >90% | "BURST-RISK" if any limit >100% | "OK" otherwise.

### Resource Efficiency
Single table using CLUSTER-WIDE RESOURCE TOTALS (allocatable) and
PER-NAMESPACE RESOURCE TOTALS (sum requested) and ACTUAL RESOURCE USAGE
(sum used from node rows):
| | CPU | Memory |
| Allocatable | | |
| Requested | | % of allocatable |
| Used | | % of allocatable |
| Req / Used ratio | | |

Then: top 5 namespaces by REQ_CPU from PER-NAMESPACE RESOURCE TOTALS.
For any namespace visible in kubectl top data, append actual usage and
flag if REQ > 2x actual.

### Top 3 Actions
One line each: namespace or node | what to change | cores or GiB freed.
Rank by capacity impact, not urgency.'

# K3s evaluation
kubectl kontext | claude --model sonnet -p 'Based on this report, is K8S a suitable alternative for this K3S cluster? Consider: node count, workload complexity, HA requirements.'

# Quick health check (fast/cheap)
kubectl kontext | claude --model haiku -p 'One paragraph: Is this cluster healthy? Top 3 concerns if any.'

```

* Ollama locally (with desire [model](https://ollama.com/library?sort=popular))

```bash
# start ollama locally as docker container with phi3
docker run -d -v ollama:/root/.ollama -p 11434:11434 --name ollama ollama/ollama
docker exec ollama ollama run phi3

kubectl kontext | docker exec -i ollama ollama run phi3 "Analyze this Kubernetes cluster report"
```

## Demo

Claude gets context before raw data, so relatively concise prompts work well — you don't need to re-explain the data shape.


https://github.com/user-attachments/assets/13ce0c64-e428-42b1-a57f-28a233d771a9

