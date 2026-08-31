package ai

// One meaning for OPENAI_BASE_URL, and one place that converts it.
//
// The setting has always included the version segment. detectOllama returns
// "http://localhost:11434/v1", docs/INSTALL.md tells people to set exactly that,
// and detectLocalModel asks for models at baseURL+"/models" — which is
// "/v1/models", the OpenAI path. That is the convention and it is the right one:
// it is what every provider's own documentation prints, so it is what somebody
// pastes.
//
// The go-micro OpenAI provider wants the other thing. It builds its request as
// strings.TrimRight(BaseURL, "/") + "/v1/chat/completions", so handing it the
// configured value produces /v1/v1/chat/completions and every question comes
// back a 404 — from the endpoint the install guide told the operator to set.
//
// Two conventions for one setting, and both were in use: the detection and the
// docs assumed one, every WithBaseURL call assumed the other. So the fix is not
// to pick a winner and rewrite the docs — the docs are right — it is to say the
// conversion out loud, once, here, and call it at each of the four places a base
// URL reaches the library.

import (
	"strings"

	"mu/internal/settings"
)

// ProviderBaseURL is a configured base URL as the go-micro provider wants it:
// the root, with no version segment, because the provider appends its own.
//
// Idempotent, and forgiving of the trailing slash people paste with a URL. An
// operator who ignores the convention and sets the bare root gets the same
// answer, which is the point of doing this in one function rather than trusting
// four call sites to agree.
func ProviderBaseURL(configured string) string {
	u := strings.TrimSpace(configured)
	if u == "" {
		return ""
	}
	u = strings.TrimRight(u, "/")
	// Only a whole trailing segment. Trimming the string "/v1" anywhere would
	// eat the end of a host that happens to end in it.
	if strings.HasSuffix(u, "/v1") {
		u = strings.TrimSuffix(u, "/v1")
	}
	return u
}

// LocalModel is the model an OpenAI-compatible endpoint should be asked for.
//
// Whatever the operator named, and nothing when they named nothing. There is no
// honest default: this setting exists for Ollama, vLLM, llama.cpp and whatever
// else speaks the protocol, and the model ids on those are whatever that machine
// has pulled. "gpt-4o-mini" was the default in two places and it is a model none
// of them has ever heard of — so the failure was a 404 naming a model the
// operator never mentioned, rather than "you have not said which model".
//
// Empty means not configured, and callers say so rather than guessing.
func LocalModel() string {
	return strings.TrimSpace(settings.Get("OPENAI_MODEL"))
}
