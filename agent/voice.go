package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"mu/internal/abuse"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/voice"
)

// VoiceHandler converts audio and text; the normal composer still owns sending.
func VoiceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		http.Error(w, "Sign in to use voice", 401)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/agent/voice" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"enabled": voice.Configured()})
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", 405)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "Invalid CSRF token", 403)
		return
	}
	if !voiceAllowed(acc.ID) {
		http.Error(w, auth.PostBlockReason(acc.ID), 403)
		return
	}
	if !voice.Configured() {
		http.Error(w, "Voice is not configured on this server", 503)
		return
	}
	if err := auth.CheckPostRate(acc.ID); err != nil {
		http.Error(w, err.Error(), 429)
		return
	}
	release, err := abuse.Start(acc.ID, "voice", 60, 300, 1)
	if err != nil {
		http.Error(w, err.Error(), 429)
		return
	}
	defer release()
	op := "voice_transcribe"
	if r.URL.Path == "/agent/voice/speak" {
		op = "voice_speak"
	} else if r.URL.Path != "/agent/voice/transcribe" {
		http.NotFound(w, r)
		return
	}
	settle, err := quota.Reserve(acc.ID, op)
	if err != nil {
		http.Error(w, err.Error(), 402)
		return
	}
	success := false
	defer func() { _ = settle(success) }()
	if op == "voice_transcribe" {
		r.Body = http.MaxBytesReader(w, r.Body, 48000*2*30)
		pcm, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Recording is too large", 413)
			return
		}
		rate, _ := strconv.Atoi(r.Header.Get("X-Audio-Rate"))
		text, err := voice.Transcribe(r.Context(), pcm, rate, r.Header.Get("X-Audio-Language"))
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		success = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": text})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 24<<10)
	var input struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Text) > 4500 || input.Text == "" {
		http.Error(w, "Invalid speech text", 400)
		return
	}
	audio, err := voice.Speak(r.Context(), input.Text)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	success = true
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Write(audio)
}

func voiceAllowed(owner string) bool {
	acc, err := auth.GetAccount(owner)
	return err == nil && acc != nil && !acc.Banned && auth.CanPost(owner)
}
