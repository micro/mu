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

	// Anthropic. DefaultModel answers for whichever provider is preferred, so
	// it is only Anthropic's answer when Anthropic has a key — asked here
	// rather than trusted.
	if settings.Get("ANTHROPIC_API_KEY") != "" {
		add(DefaultModel(), "Claude — best", ProviderAnthropic)
		add(backgroundAnthropic, "Claude — fast", ProviderAnthropic)
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
func LabelFor(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "Instance default"
	}
	for _, c := range Choices() {
		if strings.EqualFold(c.ID, id) {
			return c.Label
		}
	}
	return id
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
