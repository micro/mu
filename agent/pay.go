package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/pay"
	pb "mu/proto/assistant/v1"
)

// payHandler uses the existing agent capability; it never accepts prices,
// credited owners or billing policy from the integration token.
func payHandler(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/agent/"), "/"), "/")
	if len(parts) < 2 || parts[1] != "pay" {
		return false
	}
	r = api.CredentialRequest(r)
	if !api.AuthorizeProduct(w, r, "agent", r.Method != "GET") {
		return true
	}
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, "Authentication required", 401)
		return true
	}
	id, ok := agentSlugTarget(sess.Account, parts[0])
	a := For(sess.Account, id)
	if !ok || a == nil {
		http.NotFound(w, r)
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	var result *pay.Session
	switch {
	case len(parts) == 2 && r.Method == "POST":
		if err := auth.CheckPostRate(sess.Account); err != nil {
			http.Error(w, "Too many requests", 429)
			return true
		}
		var q pb.CheckoutRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&q); err == nil {
			var tail any
			if dec.Decode(&tail) != io.EOF {
				err = errors.New("send one JSON object")
			}
		}
		if err == nil {
			result, err = pay.Checkout(r.Context(), sess.Account, a.ID, origin.URL(r), a.Pay, a.PaySecret, &q)
		}
	case len(parts) == 3 && r.Method == "GET":
		result, err = pay.Read(sess.Account, a.ID, parts[2])
	case len(parts) == 4 && parts[3] == "start" && r.Method == "POST":
		result, err = pay.Start(sess.Account, a.ID, parts[2])
	default:
		http.Error(w, "Not found", 404)
		return true
	}
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, pay.ErrNotFound) {
			code = 404
		}
		if errors.Is(err, pay.ErrInactive) {
			code = 402
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return true
	}
	app.RespondJSON(w, result)
	return true
}
func SetPay(owner, id string, p *pay.Policy) error {
	if err := pay.Validate(p); err != nil {
		return err
	}
	rosterMutation.Lock()
	defer rosterMutation.Unlock()
	a := For(owner, id)
	if a == nil {
		return errors.New("agent not found")
	}
	a.Pay = p
	if a.PaySecret == "" {
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		a.PaySecret = hex.EncodeToString(secret[:])
	}
	return a.save()
}
func payForm(owner, id string) string {
	p := &pay.Policy{Price: 300, Currency: "gbp", Minutes: 60}
	secret := ""
	if a := For(owner, id); a != nil {
		if a.Pay != nil {
			p = a.Pay
		}
		secret = a.PaySecret
	}
	checked := ""
	if p.Enabled {
		checked = " checked"
	}
	body := `<fieldset><legend>Mu Pay</legend><p>Charge for private, timed sessions. Payments fund usage on your Mu account. Ordinary conversations remain available.</p><label><input type="checkbox" id="b-pay"` + checked + `> Enable paid sessions</label>`
	body += app.Field{ID: "b-pay-price", Type: "number", Label: "Price per participant (minor units: 300 = £3 or $3)", Value: strconv.Itoa(int(p.Price)), Wide: true}.HTML()
	body += app.Field{ID: "b-pay-currency", Label: "Currency", Wide: true, Options: []app.Option{{Value: "gbp", Label: "GBP", On: p.Currency == "gbp"}, {Value: "usd", Label: "USD", On: p.Currency == "usd"}}}.HTML()
	body += app.Field{ID: "b-pay-minutes", Type: "number", Label: "Duration in minutes", Value: strconv.Itoa(int(p.Minutes)), Wide: true}.HTML()
	body += app.Field{ID: "b-pay-return", Label: "App return URL (HTTPS)", Value: p.ReturnUrl, Wide: true}.HTML()
	body += app.Field{ID: "b-pay-notify", Label: "Payment notification URL (optional, HTTPS)", Value: p.NotifyUrl, Wide: true}.HTML()
	if secret != "" {
		body += `<details><summary>Notification signing secret</summary><code>` + html.EscapeString(secret) + `</code><p>Keep this on your app server. Verify Mu-Pay-Signature before granting access.</p></details>`
	}
	return body + `<p>The host starts the clock after payment. Mu sets the credit grant at checkout.</p></fieldset>`
}
