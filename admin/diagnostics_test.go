package admin

import (
	"strings"
	"testing"
	"time"
)

// An error naming an XML element survives being rendered.
//
// The federation check reported `starttls refused: <required>` and the page
// showed "starttls refused:" with nothing after it — the element name, which
// was the entire diagnosis, parsed as a tag and vanished. A page that deletes
// the informative half of an error is worse than one that shows nothing,
// because it reads as the error being empty.
func TestADetailWithAngleBracketsIsNotEatenByTheBrowser(t *testing.T) {
	const detail = "starttls refused: <required>"
	out := renderChecks([]healthCheck{{Name: "Federation", Status: "error", Detail: detail}})
	if !strings.Contains(out, "&lt;required&gt;") {
		t.Errorf("the element name is not escaped, so a browser will swallow it:\n%s", out)
	}
	if strings.Contains(out, "<required>") {
		t.Error("the detail went out as raw markup")
	}
}

// The provider named is the one that will answer, not the one whose key exists.
//
// This page used to read the keys itself and print "Anthropic Claude" whenever
// an ANTHROPIC_API_KEY was set — on an instance configured for DeepSeek, whose
// operator was reading the page precisely to find out what it was running.
// AI_PROVIDER is what the runtime honours and this never looked at it.
func TestTheProviderShownIsTheOneConfigured(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-not-a-real-key")
	t.Setenv("ATLAS_API_KEY", "atlas-not-a-real-key")
	t.Setenv("AI_PROVIDER", "atlas")

	if got := providerLabel(); got != "Atlas Cloud" {
		t.Errorf("provider = %q, want %q — AI_PROVIDER says atlas and an "+
			"Anthropic key merely exists", got, "Atlas Cloud")
	}

	// The other way round, so this is about the preference and not about which
	// branch happens to come first.
	t.Setenv("AI_PROVIDER", "claude")
	if got := providerLabel(); got != "Anthropic" {
		t.Errorf("provider = %q, want %q", got, "Anthropic")
	}
}

// With no preference set, whichever key exists decides — the runtime's own
// fallback order, not a different one written out again here.
func TestWithNoPreferenceTheKeyDecides(t *testing.T) {
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("ATLAS_API_KEY", "atlas-not-a-real-key")

	if got := providerLabel(); got != "Atlas Cloud" {
		t.Errorf("provider = %q, want %q", got, "Atlas Cloud")
	}
}

// A count is reported with the period it covers.
//
// "412 articles" answers nothing on its own — an hour, a week, all time. The
// operator reading it cannot tell whether it is healthy.
func TestACountComesWithItsPeriod(t *testing.T) {
	c := checkNews()
	if c.Detail == "" {
		t.Fatal("no detail")
	}
	if strings.Contains(c.Detail, "articles") &&
		!strings.Contains(c.Detail, "going back") && !strings.Contains(c.Detail, "No articles") {
		t.Errorf("a bare count with no period: %q", c.Detail)
	}
}

// Durations read as something a person would say.
func TestDurationsAreReadable(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "under a minute"},
		{90 * time.Second, "1m"},
		{3 * time.Hour, "3h"},
		{74*time.Hour + 12*time.Minute, "3d"},
	} {
		if got := roughly(tt.d); got != tt.want {
			t.Errorf("roughly(%s) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
