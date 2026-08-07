package ai

// Whether the model actually answers, as opposed to whether a key is set.
//
// /status reported the agent healthy whenever Configured() was true, which is a
// statement about this process's environment and not about the world. An
// instance pointed at an Ollama that is not running, or carrying an API key that
// has expired or run out of credit, was green while every question a user asked
// came back "Could not generate response". Configuration is the one thing a
// status page can check without leaving the building, and it is the one thing
// that is almost never what broke.
//
// So the signal is the last real call. Nothing synthetic is sent — no probe to
// pay for, no timeout to tune, no traffic on an idle instance. Until something
// has been asked we fall back to Configured(), because "not tried yet" is not
// the same as broken.

import (
	"sync"
	"time"
)

var health struct {
	sync.Mutex
	tried bool
	ok    bool
	err   string
	at    time.Time
}

// recordHealth notes the outcome of a real model call. Called from the two
// paths that talk to a provider, so anything that reaches a model updates it
// and nothing has to remember to.
func recordHealth(err error) {
	health.Lock()
	defer health.Unlock()
	health.tried = true
	health.ok = err == nil
	health.at = time.Now()
	if err != nil {
		health.err = err.Error()
	} else {
		health.err = ""
	}
}

// Status describes the model to an operator: which provider and model an
// interactive question would actually go to, and whether it is answering.
//
// It resolves the provider the same way a real call does rather than reading
// one env var. The status page used to look only for ANTHROPIC_API_KEY, so a
// perfectly healthy instance running on Atlas Cloud or a local Ollama reported
// "Not configured" and dragged the whole instance's health down with it.
func Status() (string, bool) {
	ok, why := Healthy()
	model := DefaultModel()
	provider, _, baseURL, err := resolveProvider(model)
	if err != nil {
		return "Not configured", false
	}
	if provider == "openai" {
		// A local server: the URL is the identifying thing, not "openai".
		provider = "local"
		if baseURL != "" {
			provider = "local (" + baseURL + ")"
		}
	}
	desc := provider + "/" + model
	if !ok && why != "" {
		desc += " — " + why
	}
	return desc, ok
}

// Healthy reports whether the model is answering, and why not when it is not.
func Healthy() (bool, string) {
	health.Lock()
	defer health.Unlock()
	if !health.tried {
		if Configured() {
			return true, ""
		}
		return false, "no AI provider configured"
	}
	if health.ok {
		return true, ""
	}
	return false, health.err
}
