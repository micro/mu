package agent

import (
	"context"
	"errors"
	"testing"
)

func TestFallbackOnlyBeforeWork(t *testing.T) {
	for _, tc := range []struct {
		err     error
		allowed bool
	}{
		{retryableModelFailure{errors.New("parameters.steps.items: missing field")}, true},
		{retryableModelFailure{context.DeadlineExceeded}, true},
		{retryableModelFailure{context.Canceled}, false},
		{errors.New("missing field after tool ran"), false},
		{nil, false},
	} {
		if fallbackAllowed(tc.err) != tc.allowed {
			t.Errorf("unexpected fallback for %v", tc.err)
		}
	}
}
func TestFallbackUsesAnotherConfiguredModel(t *testing.T) {
	noProviders(t)
	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "test")
	t.Setenv("ANTHROPIC_API_KEY", "test")
	calls := 0
	opts := QueryOpts{System: "Keep this persona", Tools: []string{"news"}}
	answer, err := tryModels("owner", "question", opts, func(a, p string, o QueryOpts) (string, error) {
		calls++
		if a != "owner" || p != "question" || o.System != opts.System || len(o.Tools) != 1 {
			t.Fatal("fallback lost context")
		}
		if calls == 1 {
			return "", retryableModelFailure{errors.New("missing field")}
		}
		if o.Model == "" {
			t.Fatal("no fallback model")
		}
		return "answer", nil
	})
	if err != nil || answer != "answer" || calls != 2 {
		t.Fatalf("%s %v calls=%d", answer, err, calls)
	}
}
