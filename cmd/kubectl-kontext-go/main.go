package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/dejanu/kubectl-kontext/internal/collector"
	"github.com/dejanu/kubectl-kontext/internal/render"
)

func usage() string {
	return `kubectl kontext (Go) - Cluster kontext for AI analysis

Usage:
  go run ./cmd/kubectl-kontext-go
  go run ./cmd/kubectl-kontext-go --help

Examples:
  go run ./cmd/kubectl-kontext-go | claude -p 'List critical issues and recommendations'
`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Print(usage())
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n\n%s", os.Args[1], usage())
			os.Exit(2)
		}
	}

	for _, dep := range []string{"kubectl", "jq"} {
		if _, err := exec.LookPath(dep); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s is required but not installed\n", dep)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cache, err := collector.Collect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
		os.Exit(1)
	}
	defer cache.Cleanup()

	report, err := render.Build(ctx, cache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(report)
}
