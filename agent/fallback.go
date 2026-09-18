package agent

import (
	"mu/internal/quota"
	"strings"
)

// A configured credential is not permission to switch providers.
// Return failures from the selected model to the caller without replaying work.
func queryWithFallback(account, prompt string, opts QueryOpts) (answer string, err error) {
	settle, err := quota.Reserve(account, quota.OpAgentRun)
	if err != nil {
		return "", err
	}
	completed := false
	defer func() { _ = settle(completed) }()
	answer, err = tryModels(account, prompt, opts, runNative)
	completed = err == nil && strings.TrimSpace(answer) != ""
	return answer, err
}

func tryModels(account, prompt string, opts QueryOpts, run func(string, string, QueryOpts) (string, error)) (string, error) {
	return run(account, prompt, opts)
}

type retryableModelFailure struct{ error }

func (e retryableModelFailure) Unwrap() error { return e.error }
