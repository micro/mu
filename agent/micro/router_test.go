package micro

// Addressing an agent by name.
//
// These used to be router tests: a keyword table, an LLM fallback, and a
// three-way merge, all of them asserting that "what's the weather in London?"
// reached the weather agent. There is no weather agent. What survives is the
// half that was never a guess — somebody naming the agent they want — and it is
// tested against a registered fixture rather than against whatever this
// instance happens to ship, so it keeps working when that is one.

import "testing"

// withAgent registers an agent for the duration of one test.
//
// The registry is a package-level map with no removal, which is fine for a
// thing filled in at init and never touched again — and means a test that adds
// to it has to take it back out itself, or the next one sees an agent nobody
// registered.
func withAgent(t *testing.T, id string) {
	t.Helper()
	if _, taken := Registry[id]; taken {
		t.Fatalf("%q is already registered, so this fixture would replace a real agent", id)
	}
	Register(&Agent{
		ID:           id,
		Name:         "Probe",
		SystemPrompt: "You are a fixture.",
		Tools:        []string{"news_list", "news_search"},
	})
	t.Cleanup(func() { delete(Registry, id) })
}

func TestAnAddressNamesARegisteredAgent(t *testing.T) {
	withAgent(t, "probe")

	for _, prompt := range []string{
		"@probe what is ETH doing today?",
		"@probe, what is ETH doing today?",
		"  @probe what is ETH doing today?",
		"ask the probe agent about Lisbon tomorrow",
		"use probe to summarize unread messages",
	} {
		if got := MatchDirectAddress(prompt); got != "probe" {
			t.Errorf("MatchDirectAddress(%q) = %q", prompt, got)
		}
	}
}

// A name that names nothing is not an address.
//
// This is the property that let the keyword router be deleted safely: an
// unaddressed prompt and a prompt addressed to an agent that does not exist are
// the same thing, and both run as the default rather than as a guess.
func TestAnUnknownNameIsNotAnAddress(t *testing.T) {
	for _, prompt := range []string{
		"@markets what is ETH doing today?",
		"ask the weather agent about Lisbon tomorrow",
		"what's the weather in London?",
		"use mail to summarize unread messages",
	} {
		if got := MatchDirectAddress(prompt); got != "" {
			t.Errorf("MatchDirectAddress(%q) = %q, and no such agent is registered", prompt, got)
		}
	}
}

func TestStripAddress(t *testing.T) {
	withAgent(t, "probe")

	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{"at mention", "@probe what is ETH doing today?", "what is ETH doing today?"},
		{"leading whitespace", "  @probe what is ETH doing today?", "what is ETH doing today?"},
		{"ask agent about", "ask the probe agent about Lisbon tomorrow", "Lisbon tomorrow"},
		{"use agent", "use probe summarize unread messages", "summarize unread messages"},
		{"unaddressed", "summarize unread messages", "summarize unread messages"},
		// Addressed to nobody, so nothing comes off: the words are the question.
		{"unknown name", "use mail summarize unread messages", "use mail summarize unread messages"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripAddress(tt.prompt); got != tt.want {
				t.Fatalf("StripAddress(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}
