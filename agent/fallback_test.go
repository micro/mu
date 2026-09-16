package agent

import (
	"errors"
	"testing"
)

func TestSelectedModelFailureDoesNotSpendOnAnotherProvider(t *testing.T) {
	noProviders(t)
	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "test")
	t.Setenv("ANTHROPIC_API_KEY", "test")
	calls := 0
	failure := errors.New("provider unavailable")
	_, err := tryModels("owner", "question", QueryOpts{}, func(string, string, QueryOpts) (string, error) { calls++; return "", failure })
	if calls != 1 || !errors.Is(err, failure) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
