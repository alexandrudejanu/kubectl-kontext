package render

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dejanu/kubectl-kontext/internal/collector"
)

type kList[T any] struct {
	Items []T `json:"items"`
}

type meta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
}

type container struct {
	Name      string `json:"name"`
	Resources struct {
		Limits   map[string]string `json:"limits"`
		Requests map[string]string `json:"requests"`
	} `json:"resources"`
}

type pod struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Containers []container `json:"containers"`
		NodeName   string      `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		ContainerStatuses []struct {
			RestartCount int `json:"restartCount"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type node struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Taints []struct {
			Key string `json:"key"`
		} `json:"taints"`
	} `json:"spec"`
	Status struct {
		Allocatable map[string]string `json:"allocatable"`
		NodeInfo    struct {
			KubeletVersion string `json:"kubeletVersion"`
		} `json:"nodeInfo"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

type deployment struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Replicas int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas     int `json:"readyReplicas"`
		UpdatedReplicas   int `json:"updatedReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

type statefulSet struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Replicas int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas int `json:"readyReplicas"`
	} `json:"status"`
}

type daemonSet struct {
	Metadata meta `json:"metadata"`
	Status   struct {
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		NumberAvailable        int `json:"numberAvailable"`
		NumberMisscheduled     int `json:"numberMisscheduled"`
	} `json:"status"`
}

type rollout struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		Replicas int                    `json:"replicas"`
		Strategy map[string]interface{} `json:"strategy"`
	} `json:"spec"`
	Status struct {
		Phase         string `json:"phase"`
		ReadyReplicas int    `json:"readyReplicas"`
	} `json:"status"`
}

type hpa struct {
	Metadata meta `json:"metadata"`
	Spec     struct {
		MinReplicas    int `json:"minReplicas"`
		MaxReplicas    int `json:"maxReplicas"`
		ScaleTargetRef struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"scaleTargetRef"`
	} `json:"spec"`
	Status struct {
		CurrentReplicas int `json:"currentReplicas"`
	} `json:"status"`
}

type event struct {
	Reason         string `json:"reason"`
	Count          int    `json:"count"`
	Last           string `json:"lastTimestamp"`
	Msg            string `json:"message"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
	Metadata struct {
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
}

type networkPolicy struct {
	Metadata meta `json:"metadata"`
}

func Build(ctx context.Context, cache collector.Cache) (string, error) {
	pods, err := readJSON[kList[pod]](cache.Path("pods.json"))
	if err != nil {
		return "", fmt.Errorf("decode pods: %w", err)
	}
	nodes, err := readJSON[kList[node]](cache.Path("nodes.json"))
	if err != nil {
		return "", fmt.Errorf("decode nodes: %w", err)
	}
	events, _ := readJSON[kList[event]](cache.Path("events.json"))
	deployments, _ := readJSON[kList[deployment]](cache.Path("deployments.json"))
	statefulsets, _ := readJSON[kList[statefulSet]](cache.Path("statefulsets.json"))
	daemonsets, _ := readJSON[kList[daemonSet]](cache.Path("daemonsets.json"))
	rollouts, _ := readJSON[kList[rollout]](cache.Path("rollouts.json"))
	hpas, _ := readJSON[kList[hpa]](cache.Path("hpa.json"))
	netpols, _ := readJSON[kList[networkPolicy]](cache.Path("netpol.json"))

	var b strings.Builder
	write := func(s string) { b.WriteString(s) }
	line := func(s string) { b.WriteString(s + "\n") }

	line("=== KUBERNETES CLUSTER ASSESSMENT REPORT ===")
	clusterName := strings.TrimSpace(runOrFallback(ctx, "unknown", "kubectl", "config", "current-context"))
	clusterServer := strings.TrimSpace(runOrFallback(ctx, "unknown", "kubectl", "config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}"))
	line("Cluster: " + clusterName)
	line("Generated: " + time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	line("")

	podsNoLimits := 0
	podsNoRequests := 0
	highRestarts := 0
	runningPods := 0
	istioCount := 0
	for _, p := range pods.Items {
		if p.Status.Phase == "Running" {
			runningPods++
			if hasIstioProxy(p) {
				istioCount++
			}
			if hasMissingLimits(p) {
				podsNoLimits++
			}
			if hasMissingRequests(p) {
				podsNoRequests++
			}
		}
		if sumRestarts(p) > 10 {
			highRestarts++
		}
	}
	pendingPods := countNonEmptyLines(cache.Path("pending.txt"))

	reasonSet := map[string]struct{}{}
	for _, e := range events.Items {
		reasonSet[e.Reason] = struct{}{}
	}

	line("## QUICK SUMMARY (for AI)")
	line("- Cluster Name: " + clusterName)
	line("- Cluster Server: " + clusterServer)
	line(fmt.Sprintf("- Nodes: %d", len(nodes.Items)))
	line(fmt.Sprintf("- Total Pods: %d", len(pods.Items)))
	line(fmt.Sprintf("- Pods without resource limits: %d", podsNoLimits))
	line(fmt.Sprintf("- Pods without resource requests: %d", podsNoRequests))
	line(fmt.Sprintf("- Warning event types (deduplicated): %d", len(reasonSet)))
	line(fmt.Sprintf("- Pending pods: %d", pendingPods))
	line(fmt.Sprintf("- High restart pods (>10): %d", highRestarts))
	line("")

	line("## CLUSTER OVERVIEW")
	version := runOrFallback(ctx, "", "kubectl", "version", "--short")
	if strings.TrimSpace(version) == "" {
		version = runOrFallback(ctx, "Unavailable", "kubectl", "version")
	}
	write(version)
	if !strings.HasSuffix(version, "\n") {
		line("")
	}
	line("")

	line("## NODES")
	line("NAME\tSTATUS\tROLES\tVERSION\tCPU\tMEMORY\tPODS_CAPACITY")
	for _, n := range nodes.Items {
		line(strings.Join([]string{
			n.Metadata.Name,
			nodeReady(n),
			nodeRoles(n),
			n.Status.NodeInfo.KubeletVersion,
			n.Status.Allocatable["cpu"],
			n.Status.Allocatable["memory"],
			n.Status.Allocatable["pods"],
		}, "\t"))
	}
	line("")
	line("Node conditions:")
	line("NODE\tMEMORY_PRESSURE\tDISK_PRESSURE\tPID_PRESSURE")
	for _, n := range nodes.Items {
		line(fmt.Sprintf("%s\t%s\t%s\t%s", n.Metadata.Name, cond(n, "MemoryPressure"), cond(n, "DiskPressure"), cond(n, "PIDPressure")))
	}
	line("")

	line("## NODE RESOURCE ALLOCATION")
	line("Computed from requests/limits per node (close-parity summary):")
	for _, n := range nodes.Items {
		reqCPU, limCPU := 0, 0
		reqMem, limMem := int64(0), int64(0)
		podCount := 0
		for _, p := range pods.Items {
			if p.Spec.NodeName != n.Metadata.Name || p.Status.Phase == "Failed" || p.Status.Phase == "Succeeded" {
				continue
			}
			podCount++
			for _, c := range p.Spec.Containers {
				reqCPU += parseCPU(c.Resources.Requests["cpu"])
				limCPU += parseCPU(c.Resources.Limits["cpu"])
				reqMem += parseMemKi(c.Resources.Requests["memory"])
				limMem += parseMemKi(c.Resources.Limits["memory"])
			}
		}
		allocCPU := parseCPU(n.Status.Allocatable["cpu"])
		allocMem := parseMemKi(n.Status.Allocatable["memory"])
		line(fmt.Sprintf("--- %s ---", n.Metadata.Name))
		line(fmt.Sprintf("  cpu req/lim: %s (%d%%) / %s (%d%%)", fmtCPU(reqCPU), pct(reqCPU, allocCPU), fmtCPU(limCPU), pct(limCPU, allocCPU)))
		line(fmt.Sprintf("  mem req/lim: %s (%d%%) / %s (%d%%)", fmtMemKi(reqMem), pct64(reqMem, allocMem), fmtMemKi(limMem), pct64(limMem, allocMem)))
		line(fmt.Sprintf("  pods: %d/%s", podCount, n.Status.Allocatable["pods"]))
	}
	line("")

	totalAllocCPU := 0
	totalAllocMemMi := int64(0)
	for _, n := range nodes.Items {
		totalAllocCPU += parseCPU(n.Status.Allocatable["cpu"])
		totalAllocMemMi += parseMemKi(n.Status.Allocatable["memory"]) / 1024
	}
	line("## CLUSTER-WIDE RESOURCE TOTALS")
	line(fmt.Sprintf("Total Allocatable CPU: %dm", totalAllocCPU))
	line(fmt.Sprintf("Total Allocatable Memory: %dMi", totalAllocMemMi))
	line("")

	line("## PER-NAMESPACE RESOURCE TOTALS (running pods)")
	type nsTotal struct {
		Pods     int
		ReqCPUm  int
		ReqMemKi int64
		LimCPUm  int
		LimMemKi int64
	}
	nsTotals := map[string]*nsTotal{}
	for _, p := range pods.Items {
		if p.Status.Phase != "Running" {
			continue
		}
		ns := p.Metadata.Namespace
		if nsTotals[ns] == nil {
			nsTotals[ns] = &nsTotal{}
		}
		nsTotals[ns].Pods++
		for _, c := range p.Spec.Containers {
			nsTotals[ns].ReqCPUm += parseCPU(c.Resources.Requests["cpu"])
			nsTotals[ns].ReqMemKi += parseMemKi(c.Resources.Requests["memory"])
			nsTotals[ns].LimCPUm += parseCPU(c.Resources.Limits["cpu"])
			nsTotals[ns].LimMemKi += parseMemKi(c.Resources.Limits["memory"])
		}
	}
	line("NAMESPACE\tPODS\tREQ_CPU\tREQ_MEM\tLIM_CPU\tLIM_MEM")
	nsNames := make([]string, 0, len(nsTotals))
	for ns := range nsTotals {
		nsNames = append(nsNames, ns)
	}
	sort.Strings(nsNames)
	for _, ns := range nsNames {
		v := nsTotals[ns]
		line(fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t%s", ns, v.Pods, fmtCPU(v.ReqCPUm), fmtMemKi(v.ReqMemKi), fmtCPU(v.LimCPUm), fmtMemKi(v.LimMemKi)))
	}
	line("")

	line("## ACTUAL RESOURCE USAGE")
	topNodes := strings.TrimSpace(readText(cache.Path("top_nodes.txt")))
	if topNodes == "" {
		line("Metrics not available (metrics-server not installed or not ready)")
	} else {
		line("Node usage:")
		line("NAME  CPU(cores)  CPU%  MEMORY(bytes)  MEMORY%")
		line(topNodes)
		line("")
		line("Top 10 pods by CPU:")
		line(firstNLines(cache.Path("top_pods_cpu.txt"), 10))
		line("")
		line("Top 10 pods by memory:")
		line(firstNLines(cache.Path("top_pods_mem.txt"), 10))
	}
	line("")

	line("## RESOURCE SUMMARY")
	line("Pods by namespace:")
	for _, item := range topNamespaceCounts(pods.Items, 15) {
		line(fmt.Sprintf("%d %s", item.Count, item.Namespace))
	}
	line("")

	line("## WORKLOAD READINESS")
	line("Active Deployments (replicas > 0):")
	line("NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE")
	scaledZero := 0
	unhealthy := []string{}
	for _, d := range deployments.Items {
		if d.Spec.Replicas == 0 {
			scaledZero++
			continue
		}
		line(fmt.Sprintf("%s\t%s\t%d/%d\t%d\t%d", d.Metadata.Namespace, d.Metadata.Name, d.Status.ReadyReplicas, d.Spec.Replicas, d.Status.UpdatedReplicas, d.Status.AvailableReplicas))
		if d.Status.ReadyReplicas < d.Spec.Replicas {
			unhealthy = append(unhealthy, fmt.Sprintf("%s\t%s\t%d/%d", d.Metadata.Namespace, d.Metadata.Name, d.Status.ReadyReplicas, d.Spec.Replicas))
		}
	}
	line(fmt.Sprintf("Scaled-to-zero deployments: %d", scaledZero))
	line("")
	line("Unhealthy Deployments (ready < desired):")
	if len(unhealthy) == 0 {
		line("None")
	} else {
		for _, u := range unhealthy {
			line(u)
		}
	}
	line("")
	line("StatefulSets:")
	line("NAMESPACE\tNAME\tREADY")
	for _, s := range statefulsets.Items {
		line(fmt.Sprintf("%s\t%s\t%d/%d", s.Metadata.Namespace, s.Metadata.Name, s.Status.ReadyReplicas, s.Spec.Replicas))
	}
	line("")
	line("DaemonSets:")
	line("NAMESPACE\tNAME\tDESIRED\tREADY\tAVAILABLE\tMISSCHEDULED")
	for _, d := range daemonsets.Items {
		line(fmt.Sprintf("%s\t%s\t%d\t%d\t%d\t%d", d.Metadata.Namespace, d.Metadata.Name, d.Status.DesiredNumberScheduled, d.Status.NumberReady, d.Status.NumberAvailable, d.Status.NumberMisscheduled))
	}
	line("")
	if len(rollouts.Items) > 0 {
		active, healthy, zero := 0, 0, 0
		for _, r := range rollouts.Items {
			if r.Spec.Replicas == 0 {
				zero++
			} else {
				active++
				if r.Status.Phase == "Healthy" {
					healthy++
				}
			}
		}
		line("Argo Rollouts:")
		line(fmt.Sprintf("- Total: %d, Active: %d, Healthy: %d, Scaled-to-zero: %d", len(rollouts.Items), active, healthy, zero))
		line("")
		line("Non-healthy Rollouts (Degraded, Progressing, Paused, etc.):")
		line("NAMESPACE\tNAME\tSTRATEGY\tREADY\tSTATUS")
		wrote := false
		for _, r := range rollouts.Items {
			if r.Spec.Replicas > 0 && r.Status.Phase != "Healthy" {
				strategy := "unknown"
				for k := range r.Spec.Strategy {
					strategy = k
					break
				}
				line(fmt.Sprintf("%s\t%s\t%s\t%d/%d\t%s", r.Metadata.Namespace, r.Metadata.Name, strategy, r.Status.ReadyReplicas, r.Spec.Replicas, r.Status.Phase))
				wrote = true
			}
		}
		if !wrote {
			line("None")
		}
		line("")
	}
	line("Istio sidecar injection:")
	line(fmt.Sprintf("- Pods with istio-proxy: %d/%d running pods", istioCount, runningPods))
	line("")
	line("HorizontalPodAutoscalers:")
	if len(hpas.Items) == 0 {
		line("None configured")
	} else {
		line("NAMESPACE\tNAME\tTARGET\tMIN\tMAX\tCURRENT")
		atMax := 0
		for _, h := range hpas.Items {
			line(fmt.Sprintf("%s\t%s\t%s/%s\t%d\t%d\t%d", h.Metadata.Namespace, h.Metadata.Name, h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name, max(1, h.Spec.MinReplicas), h.Spec.MaxReplicas, h.Status.CurrentReplicas))
			if h.Status.CurrentReplicas == h.Spec.MaxReplicas {
				atMax++
			}
		}
		line(fmt.Sprintf("HPAs at max replicas: %d/%d", atMax, len(hpas.Items)))
	}
	line("")

	line("## PODS WITHOUT RESOURCE LIMITS")
	writeMissingPodsSection(&b, pods.Items, hasMissingLimits)
	line("")
	line("## PODS WITHOUT RESOURCE REQUESTS")
	writeMissingPodsSection(&b, pods.Items, hasMissingRequests)
	line("")

	line("## TOP 10 MEMORY CONSUMERS (by limit)")
	for _, s := range topMemory(pods.Items, 10) {
		line(s)
	}
	line("")

	line("## TOP 10 POD RESTARTS")
	for _, s := range topRestarts(pods.Items, 10) {
		line(s)
	}
	line("")

	line("## RECENT WARNING EVENTS (deduplicated by reason)")
	for _, l := range dedupWarnings(events.Items) {
		line(l)
	}
	if len(events.Items) == 0 {
		line("None")
	}
	line("")

	line("## PROBLEM PODS")
	line("Pending:")
	pendingText := strings.TrimSpace(readText(cache.Path("pending.txt")))
	if pendingText == "" {
		line("None")
	} else {
		line(pendingText)
	}
	line("")
	line("Failed:")
	failedText := strings.TrimSpace(readText(cache.Path("failed.txt")))
	if failedText == "" {
		line("None")
	} else {
		line(failedText)
	}
	line("")

	line("## STORAGE CLASSES")
	sc := strings.TrimSpace(readText(cache.Path("storageclasses.txt")))
	if sc == "" {
		line("None configured")
	} else {
		line(sc)
	}
	line("")

	line("## POD DISRUPTION BUDGETS")
	writeTextOrNone(&b, cache.Path("pdb.txt"), "None configured")
	line("")

	line("## LIMIT RANGES")
	writeTextOrNone(&b, cache.Path("limitranges.txt"), "None configured")
	line("")

	line("## RESOURCE QUOTAS")
	writeTextOrNone(&b, cache.Path("quotas.txt"), "None configured")
	line("")

	line("## NETWORK POLICIES")
	line(fmt.Sprintf("Count: %d", len(netpols.Items)))
	line("NAMESPACE\tNAME")
	for i, np := range netpols.Items {
		if i >= 10 {
			break
		}
		line(np.Metadata.Namespace + "\t" + np.Metadata.Name)
	}
	line("")

	line("## NODE TAINTS")
	line("NODE\tTAINTS")
	for _, n := range nodes.Items {
		taints := []string{}
		for _, t := range n.Spec.Taints {
			taints = append(taints, t.Key)
		}
		if len(taints) == 0 {
			taints = []string{"<none>"}
		}
		line(n.Metadata.Name + "\t" + strings.Join(taints, ","))
	}
	line("")

	line("## K3S CONFIG (if applicable)")
	if len(nodes.Items) > 0 {
		if v := nodes.Items[0].Metadata.Annotations["k3s.io/node-args"]; v != "" {
			line(v)
		} else {
			line("Not a K3s cluster or config not exposed")
		}
	} else {
		line("Not a K3s cluster or config not exposed")
	}
	line("")

	line("=== END OF REPORT ===")
	return b.String(), nil
}

func readJSON[T any](path string) (T, error) {
	var out T
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return out, nil
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func runOrFallback(ctx context.Context, fallback string, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.Output()
	if err != nil {
		return fallback
	}
	return string(b)
}

func nodeReady(n node) string {
	for _, c := range n.Status.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return "Ready"
		}
	}
	return "NotReady"
}

func nodeRoles(n node) string {
	var roles []string
	for k := range n.Metadata.Labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

func cond(n node, kind string) string {
	for _, c := range n.Status.Conditions {
		if c.Type == kind {
			return c.Status
		}
	}
	return "Unknown"
}

func parseCPU(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "m") {
		n, _ := strconv.Atoi(strings.TrimSuffix(v, "m"))
		return n
	}
	f, _ := strconv.ParseFloat(v, 64)
	return int(f * 1000)
}

func parseMemKi(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	switch {
	case strings.HasSuffix(v, "Ki"):
		n, _ := strconv.ParseInt(strings.TrimSuffix(v, "Ki"), 10, 64)
		return n
	case strings.HasSuffix(v, "Mi"):
		n, _ := strconv.ParseInt(strings.TrimSuffix(v, "Mi"), 10, 64)
		return n * 1024
	case strings.HasSuffix(v, "Gi"):
		n, _ := strconv.ParseInt(strings.TrimSuffix(v, "Gi"), 10, 64)
		return n * 1024 * 1024
	default:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n / 1024
	}
}

func pct(used, total int) int {
	if total == 0 {
		return 0
	}
	return (used * 100) / total
}

func pct64(used, total int64) int {
	if total == 0 {
		return 0
	}
	return int((used * 100) / total)
}

func fmtCPU(m int) string {
	if m >= 1000 {
		return fmt.Sprintf("%.1f", float64(m)/1000)
	}
	return fmt.Sprintf("%dm", m)
}

func fmtMemKi(ki int64) string {
	if ki >= 1024*1024 {
		return fmt.Sprintf("%.1fGi", float64(ki)/1024.0/1024.0)
	}
	if ki >= 1024 {
		return fmt.Sprintf("%dMi", ki/1024)
	}
	return fmt.Sprintf("%dKi", ki)
}

func sumRestarts(p pod) int {
	total := 0
	for _, c := range p.Status.ContainerStatuses {
		total += c.RestartCount
	}
	return total
}

func hasMissingLimits(p pod) bool {
	for _, c := range p.Spec.Containers {
		if len(c.Resources.Limits) == 0 {
			return true
		}
	}
	return false
}

func hasMissingRequests(p pod) bool {
	for _, c := range p.Spec.Containers {
		if len(c.Resources.Requests) == 0 {
			return true
		}
	}
	return false
}

func hasIstioProxy(p pod) bool {
	for _, c := range p.Spec.Containers {
		if c.Name == "istio-proxy" {
			return true
		}
	}
	return false
}

func countNonEmptyLines(path string) int {
	count := 0
	for _, l := range strings.Split(strings.TrimSpace(readText(path)), "\n") {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	return count
}

func readText(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func firstNLines(path string, n int) string {
	lines := strings.Split(strings.TrimSpace(readText(path)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "None"
	}
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

type nsCount struct {
	Namespace string
	Count     int
}

func topNamespaceCounts(pods []pod, n int) []nsCount {
	m := map[string]int{}
	for _, p := range pods {
		m[p.Metadata.Namespace]++
	}
	out := make([]nsCount, 0, len(m))
	for ns, c := range m {
		out = append(out, nsCount{Namespace: ns, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > n {
		return out[:n]
	}
	return out
}

func writeMissingPodsSection(b *strings.Builder, pods []pod, fn func(pod) bool) {
	type item struct{ Ns, Name string }
	var missing []item
	for _, p := range pods {
		if p.Status.Phase == "Running" && fn(p) {
			missing = append(missing, item{p.Metadata.Namespace, p.Metadata.Name})
		}
	}
	if len(missing) == 0 {
		b.WriteString("None\n")
		return
	}
	if len(missing) <= 20 {
		b.WriteString(fmt.Sprintf("Count: %d\n", len(missing)))
		for _, m := range missing {
			b.WriteString(m.Ns + "\t" + m.Name + "\n")
		}
		return
	}
	b.WriteString(fmt.Sprintf("Count: %d (grouped by namespace):\n", len(missing)))
	counts := map[string]int{}
	for _, m := range missing {
		counts[m.Ns]++
	}
	names := make([]string, 0, len(counts))
	for ns := range counts {
		names = append(names, ns)
	}
	sort.Strings(names)
	for _, ns := range names {
		label := "pods"
		if counts[ns] == 1 {
			label = "pod"
		}
		b.WriteString(fmt.Sprintf("%s\t%d %s\n", ns, counts[ns], label))
	}
}

func topMemory(pods []pod, n int) []string {
	type item struct {
		Ns   string
		Name string
		Mi   int64
	}
	var out []item
	for _, p := range pods {
		if p.Status.Phase != "Running" {
			continue
		}
		var memKi int64
		for _, c := range p.Spec.Containers {
			memKi += parseMemKi(c.Resources.Limits["memory"])
		}
		out = append(out, item{Ns: p.Metadata.Namespace, Name: p.Metadata.Name, Mi: memKi / 1024})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mi > out[j].Mi })
	if len(out) > n {
		out = out[:n]
	}
	lines := make([]string, 0, len(out))
	for _, v := range out {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%dMi", v.Ns, v.Name, v.Mi))
	}
	if len(lines) == 0 {
		return []string{"None"}
	}
	return lines
}

func topRestarts(pods []pod, n int) []string {
	type item struct {
		Ns       string
		Name     string
		Restarts int
	}
	var out []item
	for _, p := range pods {
		r := sumRestarts(p)
		if r > 0 {
			out = append(out, item{Ns: p.Metadata.Namespace, Name: p.Metadata.Name, Restarts: r})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Restarts > out[j].Restarts })
	if len(out) > n {
		out = out[:n]
	}
	lines := make([]string, 0, len(out))
	for _, v := range out {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%d", v.Ns, v.Name, v.Restarts))
	}
	if len(lines) == 0 {
		return []string{"None"}
	}
	return lines
}

func dedupWarnings(events []event) []string {
	type agg struct {
		Total   int
		Last    string
		Objects map[string]struct{}
		NS      map[string]struct{}
		Msg     string
	}
	m := map[string]*agg{}
	for _, e := range events {
		if m[e.Reason] == nil {
			m[e.Reason] = &agg{
				Objects: map[string]struct{}{},
				NS:      map[string]struct{}{},
				Msg:     e.Msg,
			}
		}
		entry := m[e.Reason]
		if e.Count > 0 {
			entry.Total += e.Count
		} else {
			entry.Total++
		}
		last := e.Last
		if last == "" {
			last = e.Metadata.CreationTimestamp
		}
		if last > entry.Last {
			entry.Last = last
		}
		entry.Objects[e.InvolvedObject.Kind+"/"+e.InvolvedObject.Name] = struct{}{}
		ns := e.InvolvedObject.Namespace
		if ns == "" {
			ns = "cluster"
		}
		entry.NS[ns] = struct{}{}
	}
	type pair struct {
		Reason string
		A      *agg
	}
	var pairs []pair
	for reason, a := range m {
		pairs = append(pairs, pair{Reason: reason, A: a})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].A.Total > pairs[j].A.Total })
	var out []string
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("[%dx] %s in %s (last: %s)", p.A.Total, p.Reason, joinKeys(p.A.NS), p.A.Last))
		out = append(out, "  Objects: "+joinKeys(p.A.Objects))
		out = append(out, "  Example: "+truncateMsg(p.A.Msg, 200))
	}
	return out
}

func joinKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func truncateMsg(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	idx := strings.LastIndex(s, " ")
	if idx > 0 {
		return s[:idx]
	}
	return s
}

func writeTextOrNone(b *strings.Builder, path string, none string) {
	v := strings.TrimSpace(readText(path))
	if v == "" {
		b.WriteString(none + "\n")
		return
	}
	b.WriteString(v + "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
