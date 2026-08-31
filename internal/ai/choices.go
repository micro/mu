package ai

// Which models this instance can actually offer.
//
// An agent could already name a model — QueryOpts.Model has been carried down
// to nativeLLMFor the whole time — but only the built-in agents ever set it.
// Your own agents dropped it on the floor, so the instruction "use the cheap
// one for this" was available to Micro and Code and to nobody's own agent.
//
// Derived, never listed. A hand-written dropdown of model ids is a list that is
// wrong within a month: ids change, providers add and retire them, and the one
// place it is written down is not the place anybody updates. Every entry here
// comes from what the instance is already configured to send — DefaultModel,
// AtlasModel, OpenRouterModel and the two cheap ends — so a new key or a
// changed ANTHROPIC_MODEL moves the menu without anybody editing it.
//
// Providers with no key contribute nothing, which is the point: offering a
// Claude model on an instance that has no Anthropic key is offering a failure.

import (
	"mu/internal/settings"
	"sort"
	"strings"
)

// Choice is one model somebody can pick, as a menu needs it.
type Choice struct {
	// ID is the model id sent to the provider, and what is stored on an agent.
	ID string
	// Label is what a person reads. Not the id: "deepseek-ai/deepseek-v4-pro"
	// is an address, and the thing being chosen is closer to "the good one".
	Label string
	// Provider is which key it goes through, for grouping and for saying why
	// a menu is short.
	Provider string
}

// Choices are the models this instance can offer, best of each provider first.
//
// Empty when nothing is configured, which is a real state and not an error: an
// instance with no key has no agent either, and a menu of things that cannot
// run is worse than no menu.
func Choices() []Choice {
	var out []Choice
	seen := map[string]bool{}
	add := func(id, label, provider string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, Choice{ID: id, Label: label, Provider: provider})
	}

	// Anthropic, by name.
	//
	// This offered DefaultModel() as "Claude — best" and the cheap end as
	// "Claude — fast", which was two problems. An instance with an Anthropic
	// key had no way to choose Opus, because Opus was never in the list. And
	// DefaultModel answers for whichever provider is *preferred* — so on an
	// instance that preferred Atlas, the entry labelled Claude carried a
	// DeepSeek id.
	//
	// Named models, like every other provider here. The default is still
	// whatever DefaultModel resolves to and is offered as its own entry below;
	// this is the menu of what can be chosen instead.
	if settings.Get("ANTHROPIC_API_KEY") != "" {
		add(ModelClaudeOpus, "Claude Opus 5", ProviderAnthropic)
		add(ModelClaudeSonnet, "Claude Sonnet 5", ProviderAnthropic)
		add(ModelClaudeHaiku, "Claude Haiku 4.5", ProviderAnthropic)
	}

	if getAtlasAPIKey() != "" {
		add(AtlasModel(), "DeepSeek — best", ProviderAtlasCloud)
		add(ModelDeepSeekFlash, "DeepSeek — fast", ProviderAtlasCloud)
		add(ModelQwenPlus, "Qwen", ProviderAtlasCloud)
	}

	if getGeminiAPIKey() != "" {
		add(GeminiModel(), "Gemini — best", ProviderGemini)
		add(ModelGeminiFlash, "Gemini — fast", ProviderGemini)
	}

	if getOpenRouterAPIKey() != "" {
		add(OpenRouterModel(), "OpenRouter", ProviderOpenRouter)
	}

	// A local model has no menu: whatever is running is the only thing it can
	// serve, and naming it in a list beside three cloud models suggests a
	// choice that is not there.
	return out
}

// Offered reports whether a model id is one this instance would offer.
//
// The guard on anything that stores a model against an agent. A stored id that
// no provider here serves is a run that fails at the model call — long after
// the person who typed it has gone — so it is refused at the point it is set,
// where there is somebody to tell.
//
// The empty string is offered, and means the instance's own default. That is
// the value an agent has until somebody chooses otherwise and the one it goes
// back to when a provider is removed.
func Offered(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return true
	}
	for _, c := range Choices() {
		if strings.EqualFold(c.ID, id) {
			return true
		}
	}
	return false
}

// LabelFor is what to call a model on screen, or the id itself when it is not
// one of ours — an operator who set AGENT_MODEL by hand should see what they
// set rather than nothing.
//
// The empty string names the instance's default *and says which model that
// is*. "Instance default" alone is a menu entry that does not say what it
// selects, on the one screen where the whole point is knowing what is running.
func LabelFor(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return DefaultLabel()
	}
	for _, c := range Choices() {
		if strings.EqualFold(c.ID, id) {
			return c.Label
		}
	}
	return id
}

// DefaultLabel names the instance's default in a menu: what it is, and which
// model that turns out to be.
//
// Resolved rather than assumed. DefaultModel reads ANTHROPIC_MODEL, then the
// preferred provider, then falls back — so the answer depends on how the
// instance is configured and is exactly the thing somebody choosing a model
// wants to see before deciding they need something else.
//
// Only when this instance actually offers it. DefaultModel ends at a Claude id
// whether or not there is an Anthropic key, and modelFor swaps it at the call
// for whatever the instance can really reach — so on an Atlas-only install the
// name here would be a model that never runs. Unofferable, and it goes back to
// the words it had, which promise nothing and are therefore not wrong.
func DefaultLabel() string {
	m := strings.TrimSpace(DefaultModel())
	for _, c := range Choices() {
		if strings.EqualFold(c.ID, m) {
			return "Default — " + c.Label
		}
	}
	return "Instance default"
}

// ByProvider groups the choices for a menu with headings, providers in a
// stable order so the list does not reshuffle between page loads.
func ByProvider() map[string][]Choice {
	out := map[string][]Choice{}
	for _, c := range Choices() {
		out[c.Provider] = append(out[c.Provider], c)
	}
	for _, group := range out {
		sort.SliceStable(group, func(i, j int) bool { return group[i].Label < group[j].Label })
	}
	return out
}
