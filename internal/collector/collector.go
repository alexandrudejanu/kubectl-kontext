package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type Cache struct {
	Dir   string
	Files map[string]string
}

func (c Cache) Path(name string) string {
	return c.Files[name]
}

func (c Cache) Cleanup() error {
	return os.RemoveAll(c.Dir)
}

type task struct {
	name     string
	args     []string
	fallback string
	required bool
}

func Collect(ctx context.Context) (Cache, error) {
	dir, err := os.MkdirTemp("", "kubectl-kontext-go-*")
	if err != nil {
		return Cache{}, fmt.Errorf("create temp dir: %w", err)
	}

	cache := Cache{Dir: dir, Files: map[string]string{}}
	for _, n := range []string{
		"pods.json", "nodes.json", "events.json",
		"storageclasses.txt", "pdb.txt", "limitranges.txt", "quotas.txt",
		"netpol.json", "deployments.json", "statefulsets.json", "daemonsets.json",
		"rollouts.json", "top_nodes.txt", "top_pods_cpu.txt", "top_pods_mem.txt",
		"pending.txt", "failed.txt", "hpa.json",
	} {
		cache.Files[n] = filepath.Join(dir, n)
	}

	phase1 := []task{
		{name: "pods.json", args: []string{"get", "pods", "-A", "-o", "json"}, required: true},
		{name: "nodes.json", args: []string{"get", "nodes", "-o", "json"}, required: true},
		{name: "events.json", args: []string{"get", "events", "-A", "--field-selector", "type=Warning", "-o", "json"}, required: true},
	}
	if err := runTasks(ctx, cache, phase1); err != nil {
		_ = cache.Cleanup()
		return Cache{}, err
	}

	phase2 := []task{
		{name: "storageclasses.txt", args: []string{"get", "storageclasses", "-o", "custom-columns=NAME:.metadata.name,PROVISIONER:.provisioner,DEFAULT:.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class"}},
		{name: "pdb.txt", args: []string{"get", "pdb", "-A", "--no-headers"}},
		{name: "limitranges.txt", args: []string{"get", "limitranges", "-A", "--no-headers"}},
		{name: "quotas.txt", args: []string{"get", "resourcequotas", "-A", "--no-headers"}},
		{name: "netpol.json", args: []string{"get", "networkpolicies", "-A", "-o", "json"}, fallback: `{"items":[]}`},
		{name: "deployments.json", args: []string{"get", "deployments", "-A", "-o", "json"}, fallback: `{"items":[]}`},
		{name: "statefulsets.json", args: []string{"get", "statefulsets", "-A", "-o", "json"}, fallback: `{"items":[]}`},
		{name: "daemonsets.json", args: []string{"get", "daemonsets", "-A", "-o", "json"}, fallback: `{"items":[]}`},
		{name: "rollouts.json", args: []string{"get", "rollouts", "-A", "-o", "json"}, fallback: `{"items":[]}`},
		{name: "top_nodes.txt", args: []string{"top", "nodes", "--no-headers"}},
		{name: "top_pods_cpu.txt", args: []string{"top", "pods", "-A", "--no-headers", "--sort-by=cpu"}},
		{name: "top_pods_mem.txt", args: []string{"top", "pods", "-A", "--no-headers", "--sort-by=memory"}},
		{name: "pending.txt", args: []string{"get", "pods", "-A", "--field-selector=status.phase=Pending", "--no-headers"}},
		{name: "failed.txt", args: []string{"get", "pods", "-A", "--field-selector=status.phase=Failed", "--no-headers"}},
		{name: "hpa.json", args: []string{"get", "hpa", "-A", "-o", "json"}, fallback: `{"items":[]}`},
	}
	if err := runTasks(ctx, cache, phase2); err != nil {
		_ = cache.Cleanup()
		return Cache{}, err
	}

	return cache, nil
}

func runTasks(ctx context.Context, cache Cache, tasks []task) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))

	for _, t := range tasks {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := runKubectl(ctx, t.args...)
			if err != nil {
				if t.required {
					errCh <- fmt.Errorf("%s failed: %w", t.name, err)
					return
				}
				if t.fallback != "" {
					out = []byte(t.fallback)
				} else {
					out = []byte{}
				}
			}
			if writeErr := os.WriteFile(cache.Path(t.name), out, 0o644); writeErr != nil {
				errCh <- fmt.Errorf("write %s: %w", t.name, writeErr)
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func runKubectl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	return cmd.Output()
}
