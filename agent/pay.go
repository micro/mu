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

// PayRequest identifies the payment operations that the server may forward to
// this handler with an API token. Reserved names can have their own handlers.
func PayRequest(r *http.Request) bool {
	parts, ok := payParts(r)
	return ok && !reservedName(strings.ToLower(parts[0]))
}

// A non-nil slice also identifies an unsupported request in the pay namespace,
// so payHandler can return its usual 404 instead of opening the agent page.
func payParts(r *http.Request) ([]string, bool) {
	p, ok := strings.CutPrefix(r.URL.Path, "/agent/")
	if !ok {
		return nil, false
	}
	parts := strings.SplitN(strings.TrimSuffix(p, "/"), "/", 5)
	if len(parts) < 2 || parts[1] != "pay" {
		return nil, false
	}
	if parts[0] == "" || strings.Contains(p, `\`) || strings.Contains(strings.ToLower(r.URL.RawPath), "%2f") {
		return parts, false
	}
	switch len(parts) {
	case 2:
		return parts, r.Method == http.MethodPost
	case 3:
		return parts, r.Method == http.MethodGet && parts[2] != ""
	case 4:
		return parts, r.Method == http.MethodPost && parts[2] != "" && parts[3] == "start"
	}
	return parts, false
}

// payHandler checks the agent capability and the owner before serving any booking.
func payHandler(w http.ResponseWriter, r *http.Request) bool {
	parts, supported := payParts(r)
	if parts == nil {
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
	if !supported || !ok || a == nil {
		http.NotFound(w, r)
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	var result *pay.Session
	switch len(parts) {
	case 2:
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
	case 3:
		result, err = pay.Read(sess.Account, a.ID, parts[2])
	case 4:
		result, err = pay.Start(sess.Account, a.ID, parts[2])
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
	body += app.Field{ID: "b-pay-return", Label: "App return URL (HTTPS; leave empty for the hosted agent app)", Value: p.ReturnUrl, Wide: true}.HTML()
	body += app.Field{ID: "b-pay-notify", Label: "Payment notification URL (optional, HTTPS)", Value: p.NotifyUrl, Wide: true}.HTML()
	if secret != "" {
		body += `<details><summary>Notification signing secret</summary><code>` + html.EscapeString(secret) + `</code><p>Keep this on your app server. Verify Mu-Pay-Signature before granting access.</p></details>`
	}
	return body + `<p>The host starts the clock after payment. Mu sets the credit grant at checkout.</p></fieldset>`
}
