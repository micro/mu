package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"mu/internal/exercise"
)

// runExercise drives the synthetic monitor at a target Mu and prints a JSON
// report (the operator loop's exerciser — see internal/docs/OPERATOR.md).
//
//	mu exercise                 → probe the configured server once
//	mu exercise --target URL     → probe a specific target (e.g. a canary)
//	mu exercise --runs 20        → repeat the battery for latency percentiles
//	mu exercise --deep           → also exercise the agent (LLM cost; rate-limited)
//
// Exit code is non-zero when any probe failed, so CI / the operator can gate on
// it and a canary-vs-stable comparison is a diff of two reports.
func runExercise(rest []string, rc *ResolvedConfig) int {
	target := rc.URL
	runs := 1
	deep := false

	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--target", "--url":
			if i+1 < len(rest) {
				target = rest[i+1]
				i++
			}
		case "--runs":
			if i+1 < len(rest) {
				if n, err := strconv.Atoi(rest[i+1]); err == nil {
					runs = n
				}
				i++
			}
		case "--deep":
			deep = true
		default:
			// A bare positional is treated as the target URL.
			if target == rc.URL && rest[i] != "" && rest[i][0] != '-' {
				target = rest[i]
			}
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "no target: set --target URL, MU_URL, or run `mu login`")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	report := exercise.Run(ctx, target, exercise.Battery(deep), runs, nil)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)

	if !report.Healthy() {
		return 1
	}
	return 0
}
