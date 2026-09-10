package agent

import (
	"context"
	"errors"
	"strings"

	gmai "go-micro.dev/v6/model"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/settings"
)

// Only a failed attempt which has emitted no text or tools may be replayed.
type retryableModelFailure struct{ error }

func (e retryableModelFailure) Unwrap() error { return e.error }

func fallbackAllowed(err error) bool {
	var safe retryableModelFailure
	if !errors.As(err, &safe) || errors.Is(err, context.Canceled) {
		return false
	}
	switch gmai.ClassifyError(err) {
	case gmai.ErrorKindTimeout, gmai.ErrorKindRateLimited, gmai.ErrorKindUnavailable, gmai.ErrorKindAuth, gmai.ErrorKindConfiguration, gmai.ErrorKindProvider:
		return true
	}
	text := strings.ToLower(err.Error())
	for _, code := range []string{"400", "401", "403", "404", "429", "500", "502", "503", "504"} {
		if strings.Contains(text, "api error ("+code+" ") {
			return true
		}
	}
	return false
}

func queryWithFallback(account, prompt string, opts QueryOpts) (string, error) {
	return tryModels(account, prompt, opts, runNative)
}
func tryModels(account, prompt string, opts QueryOpts, run func(string, string, QueryOpts) (string, error)) (string, error) {
	answer, err := run(account, prompt, opts)
	if !fallbackAllowed(err) {
		return answer, err
	}
	provider, _, model, _, _ := nativeLLMFor(opts.Model, opts.Public)
	seen := map[string]bool{provider + "/" + model: true}
	for _, candidate := range fallbackModels(opts.Public) {
		p, _, m, _, ok := nativeLLMFor(candidate, opts.Public)
		if !ok || seen[p+"/"+m] {
			continue
		}
		seen[p+"/"+m] = true
		app.Log("ai", "model fallback from=%s/%s to=%s/%s reason=%s", provider, model, p, m, gmai.ClassifyError(err))
		next := opts
		next.Model = candidate
		answer, err = run(account, prompt, next)
		if !fallbackAllowed(err) {
			return answer, err
		}
		provider, model = p, m
	}
	return answer, err
}

func fallbackModels(fast bool) []string {
	var out []string
	if settings.Get("ANTHROPIC_API_KEY") != "" {
		model := strings.TrimSpace(settings.Get("ANTHROPIC_MODEL"))
		if model == "" {
			model = ai.ModelClaudeSonnet
			if fast {
				model = ai.ModelClaudeHaiku
			}
		}
		out = append(out, model)
	}
	if ai.GeminiKey() != "" {
		out = append(out, ai.PreferredModel("gemini", fast))
	}
	if ai.AtlasKey() != "" {
		out = append(out, ai.PreferredModel("atlascloud", fast))
	}
	if ai.OpenRouterKey() != "" {
		out = append(out, ai.OpenRouterModel())
	}
	return out
}
