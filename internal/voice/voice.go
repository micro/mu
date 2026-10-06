// Package voice provides an opt-in, on-device voice input for the web client.
package voice

import (
	_ "embed"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

//go:embed worker.js
var worker []byte

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if _, _, err := auth.RequireSession(r); err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Voice | Micro</title><link rel="stylesheet" href="/mu.css?v=voice-1"><script defer src="/mu.js?v=voice-1"></script></head><body class="voice-frame"><main id="voice-panel" class="section-stack"><h1>Talk to Micro</h1><p>Speech is processed on this device. Only your message is sent to Micro. Normal assistant usage applies.</p><p class="text-sm text-muted">English preview. First use downloads speech models from Hugging Face and runtime files from jsDelivr (over 100 MB). Keep this page open. Performance depends on your device.</p><div class="form-actions"><button id="voice-enable" type="button">Enable voice</button><button id="voice-stop" type="button" hidden>Stop voice</button></div><label class="check-label"><input id="voice-wake" type="checkbox">Listen for “Hey Micro” and send my request</label><label class="check-label"><input id="voice-speak" type="checkbox" checked>Speak replies</label><p id="voice-status" role="status" aria-live="polite">Microphone off.</p><div class="form-actions"><button id="voice-record" type="button" disabled>Record</button><button id="voice-interrupt" type="button" disabled>Stop speaking</button></div><label class="field-label">Your message<textarea id="voice-text" rows="3" maxlength="8000"></textarea></label><div class="form-actions"><button id="voice-send" type="button" disabled>Send</button></div><p id="voice-answer" class="preserve-whitespace" aria-live="polite"></p></main></body></html>`))
}

func Worker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	// Only this worker can load speech dependencies. The application page's
	// script and connection policy remains self-only.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval' https://cdn.jsdelivr.net; connect-src 'self' https://cdn.jsdelivr.net https://huggingface.co https://*.huggingface.co https://*.hf.co; worker-src 'self' blob:")
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(worker)
}
