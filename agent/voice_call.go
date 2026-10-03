package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/internal/abuse"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/phone"
	"mu/internal/quota"
	"mu/internal/settings"
	"mu/internal/twilio"
	"mu/internal/voice"
)

type callGrant struct {
	owner, from, to, secret string
	until                   time.Time
	connected               bool
}

var calls = struct {
	sync.Mutex
	grants map[string]callGrant
}{grants: map[string]callGrant{}}

func VoiceCodeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		http.Error(w, "Sign in", 401)
		return
	}
	if !auth.StrictCSRF(r) || !voiceAllowed(acc.ID) {
		http.Error(w, "Forbidden", 403)
		return
	}
	if !voice.CallsConfigured() || len(phone.Numbers(acc.ID)) == 0 {
		http.Error(w, "Verify your phone number before calling Micro", 400)
		return
	}
	if err := auth.CheckPostRate(acc.ID); err != nil {
		http.Error(w, err.Error(), 429)
		return
	}
	code, err := voice.NewCode(acc.ID)
	if err != nil {
		http.Error(w, "Could not create a call code", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": code, "number": settings.Get("TWILIO_VOICE_FROM")})
}

// CallWebhookHandler authenticates the provider and the caller before exposing Agent.
func CallWebhookHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if r.ParseForm() != nil {
		http.Error(w, "Invalid call", 400)
		return
	}
	base := origin.Self()
	if !voice.CallsConfigured() || base == "" || twilio.AccountSID() == "" {
		http.Error(w, "Calls are not configured", 503)
		return
	}
	if !twilio.ValidSignature(r, []string{base + "/voice/webhook"}, r.PostForm) || r.PostForm.Get("AccountSid") != twilio.AccountSID() {
		http.Error(w, "Invalid signature", 403)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	hangup := func() {
		fmt.Fprint(w, `<Response><Say>This call could not be verified. Get a new call code from your Micro account and try again.</Say><Hangup/></Response>`)
	}
	from, to, sid := phone.Normalise(r.PostForm.Get("From")), phone.Normalise(r.PostForm.Get("To")), r.PostForm.Get("CallSid")
	if from == "" || to != phone.Normalise(settings.Get("TWILIO_VOICE_FROM")) || !strings.HasPrefix(sid, "CA") || len(sid) > 64 {
		hangup()
		return
	}
	owner := phone.Owner(from)
	if owner == "" || !phone.Verified(owner, from) || !voiceAllowed(owner) {
		hangup()
		return
	}
	calls.Lock()
	defer calls.Unlock()
	for id, c := range calls.grants {
		if time.Now().After(c.until) {
			delete(calls.grants, id)
		}
	}
	c, exists := calls.grants[sid]
	if !exists {
		// A callback with digits must follow a challenge created by this process.
		if r.PostForm.Get("Digits") != "" || len(calls.grants) >= 1000 {
			hangup()
			return
		}
		if err := auth.CheckPostRate(owner); err != nil {
			hangup()
			return
		}
		calls.grants[sid] = callGrant{owner: owner, from: from, to: to, until: time.Now().Add(time.Minute)}
		fmt.Fprint(w, `<Response><Gather input="dtmf" numDigits="6" timeout="15" action="/voice/webhook" method="POST"><Say>Enter the six digit call code from your Micro account.</Say></Gather><Hangup/></Response>`)
		return
	}
	if c.owner != owner || c.from != from || c.to != to || c.secret != "" || c.connected || !voice.ConsumeCode(owner, r.PostForm.Get("Digits")) {
		hangup()
		return
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		hangup()
		return
	}
	c.secret = hex.EncodeToString(secret[:])
	c.until = time.Now().Add(time.Minute)
	calls.grants[sid] = c
	fmt.Fprintf(w, `<Response><Connect><ConversationRelay url="%s/voice/relay" welcomeGreeting="Hello. What's on your mind?" interruptible="any"><Parameter name="key" value="%s"/></ConversationRelay></Connect><Hangup/></Response>`, html.EscapeString(strings.Replace(base, "https://", "wss://", 1)), c.secret)
}

type callMessage struct {
	Type       string            `json:"type"`
	AccountSID string            `json:"accountSid"`
	CallSID    string            `json:"callSid"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	Parameters map[string]string `json:"customParameters"`
	Prompt     string            `json:"voicePrompt"`
	Last       bool              `json:"last"`
}

func CallRelayHandler(w http.ResponseWriter, r *http.Request) {
	base := origin.Self()
	if r.Method != "GET" || !voice.CallsConfigured() || base == "" {
		http.Error(w, "Calls unavailable", 503)
		return
	}
	url := base + "/voice/relay"
	wsURL := strings.Replace(url, "https://", "wss://", 1)
	if !twilio.ValidSignature(r, []string{url, wsURL, url + "/", wsURL + "/"}, nil) {
		http.Error(w, "Invalid signature", 403)
		return
	}
	// A provider signature is the credential here; browser cookies are never used.
	conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(32 << 10)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var setup callMessage
	if conn.ReadJSON(&setup) != nil || setup.Type != "setup" || setup.AccountSID != twilio.AccountSID() {
		return
	}
	calls.Lock()
	grant, ok := calls.grants[setup.CallSID]
	if !ok || grant.connected || grant.secret == "" || grant.secret != setup.Parameters["key"] || grant.from != phone.Normalise(setup.From) || grant.to != phone.Normalise(setup.To) || time.Now().After(grant.until) {
		calls.Unlock()
		return
	}
	grant.connected = true
	grant.until = time.Now().Add(15 * time.Minute)
	calls.grants[setup.CallSID] = grant
	calls.Unlock()
	// Keep the used call ID as a tombstone until expiry, so signed retries cannot rerun it.
	if !phone.Verified(grant.owner, grant.from) || !voiceAllowed(grant.owner) {
		return
	}
	release, err := abuse.Start(grant.owner, "voice calls", 10, 30, 1)
	if err != nil {
		return
	}
	defer release()
	var writes sync.Mutex
	send := func(value any) error {
		writes.Lock()
		defer writes.Unlock()
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteJSON(value)
	}
	bill := func() bool {
		settle, err := quota.Reserve(grant.owner, "voice_call")
		if err != nil {
			return false
		}
		return settle(true) == nil
	}
	if !bill() {
		send(map[string]any{"type": "text", "token": "There is not enough credit for this call.", "last": true})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
	events := make(chan callMessage, 16)
	go func() {
		defer close(events)
		for {
			var m callMessage
			if conn.ReadJSON(&m) != nil {
				return
			}
			select {
			case events <- m:
			case <-ctx.Done():
				return
			}
		}
	}()
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()
	type result struct {
		text        string
		err         error
		interrupted bool
	}
	done := make(chan result, 1)
	var running, interrupted bool
	var stop context.CancelFunc
	var pending, partial string
	start := func(text string) {
		running = true
		interrupted = false
		turn, stopTurn := context.WithCancel(ctx)
		stop = stopTurn
		go func() {
			defer stopTurn()
			// Final answers only: intermediate model rounds may be replaced after tool use.
			answer, err := Ask(AskRequest{RunContext: turn, Account: grant.owner, Client: "voice", Thread: setup.CallSID, Text: text, Trigger: "phone call", System: "This is a spoken phone conversation. Give short, natural answers, without markdown, tables or reading out URLs. Ask one question at a time. A caller may interrupt; do not claim an action was undone merely because they interrupted."})
			done <- result{text: answer.Text, err: err, interrupted: turn.Err() != nil}
		}()
	}
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-minute.C:
			if ctx.Err() != nil {
				return
			}
			if !phone.Verified(grant.owner, grant.from) || !voiceAllowed(grant.owner) || !bill() {
				return
			}
		case m, ok := <-events:
			if !ok {
				return
			}
			switch m.Type {
			case "interrupt":
				interrupted = true
				if stop != nil {
					stop()
				}
			case "prompt":
				partial = strings.TrimSpace(partial + " " + m.Prompt)
				if len(partial) > 4096 {
					return
				}
				if !m.Last {
					continue
				}
				text := strings.TrimSpace(partial)
				partial = ""
				if text == "" {
					continue
				}
				if !phone.Verified(grant.owner, grant.from) || !voiceAllowed(grant.owner) {
					return
				}
				if running {
					interrupted = true
					if stop != nil {
						stop()
					}
					pending = text
				} else {
					start(text)
				}
			case "error":
				return
			}
		case res := <-done:
			running = false
			stop = nil
			if !res.interrupted && !interrupted {
				text := res.text
				if res.err != nil {
					text = "I couldn't complete that request. Please try again."
				}
				if err := send(map[string]any{"type": "text", "token": text, "last": true, "interruptible": true, "preemptible": true}); err != nil {
					return
				}
			}
			if pending != "" {
				text := pending
				pending = ""
				start(text)
			}
		}
	}
}
