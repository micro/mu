package agent

import "testing"

// providerKeys is every variable that can put a model behind this package.
//
// One list, because it grows. It was three names, then getAtlasAPIKey learned
// to read ATLASCLOUD_API_KEY and OPENAI_API_KEY, then Gemini arrived — and each
// time, a test that cleared the names it knew about kept passing on a machine
// with no keys and started failing on one with them. The failure never points
// at the missing variable either: the test routes somewhere unexpected and
// trips an assertion about something else.
//
// Anything added to ai.Configured or nativeLLMFor belongs here the same day.
var providerKeys = []string{
	"ANTHROPIC_API_KEY",
	"ATLASCLOUD_API_KEY",
	"ATLAS_API_KEY",
	"OPENAI_API_KEY",
	"OPENAI_BASE_URL",
	"GEMINI_API_KEY",
	"OPENROUTER_API_KEY",
	"AI_PROVIDER",
	"AGENT_MODEL",
	"ANTHROPIC_MODEL",
	"ATLAS_MODEL",
	"GEMINI_MODEL",
	"OPENROUTER_MODEL",
}

// noProviders puts the test on a box with no model behind it, whatever the
// developer has exported. Restored by t.Setenv when the test ends.
//
// A test about what happens with no model has to make that true rather than
// assume it — otherwise it is a test about the machine it ran on.
func noProviders(t *testing.T) {
	t.Helper()
	for _, k := range providerKeys {
		t.Setenv(k, "")
	}
}

// onlyProvider clears every key and sets one, for a test about a particular
// provider being the one available.
func onlyProvider(t *testing.T, key, value string) {
	t.Helper()
	noProviders(t)
	t.Setenv(key, value)
}
