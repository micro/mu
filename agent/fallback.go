package agent

// A configured credential is not permission to switch providers.
// Return failures from the selected model to the caller without replaying work.
func queryWithFallback(account, prompt string, opts QueryOpts) (string, error) {
	return tryModels(account, prompt, opts, runNative)
}

func tryModels(account, prompt string, opts QueryOpts, run func(string, string, QueryOpts) (string, error)) (string, error) {
	return run(account, prompt, opts)
}

type retryableModelFailure struct{ error }

func (e retryableModelFailure) Unwrap() error { return e.error }
