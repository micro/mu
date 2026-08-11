package sms

// Twilio, which is the only provider wired in.
//
// Two things happen here and nothing else: a POST to send a message, and the
// signature check that says an inbound webhook really came from Twilio. Both
// are small enough not to be worth a dependency — the send is a form post and
// the signature is an HMAC — and a dependency here would be one that has to be
// trusted with the auth token.

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/settings"
)

// send hands one message to Twilio and returns its id.
func send(to, body string) (string, error) {
	sid := strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID"))
	token := strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
	from := From()
	if sid == "" || token == "" || from == "" {
		return "", fmt.Errorf("this instance cannot send texts — no number is configured")
	}

	form := url.Values{"To": {to}, "From": {from}, "Body": {body}}
	req, err := http.NewRequest(http.MethodPost,
		"https://api.twilio.com/2010-04-01/Accounts/"+url.PathEscape(sid)+"/Messages.json",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(sid, token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 20 * time.Second}
	rsp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the message provider: %w", err)
	}
	defer rsp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(rsp.Body, 64<<10))
	var out struct {
		SID     string `json:"sid"`
		Message string `json:"message"`
		Status  string `json:"status"`
	}
	json.Unmarshal(b, &out) //nolint:errcheck

	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		// Twilio's own message is the useful one ("The 'To' number is not a
		// valid mobile number"), and it is safe to pass on: it describes the
		// request the caller just made.
		if out.Message != "" {
			return "", fmt.Errorf("%s", out.Message)
		}
		return "", fmt.Errorf("the message provider refused that (%d)", rsp.StatusCode)
	}
	return out.SID, nil
}

// validSignature reports whether a webhook really came from Twilio.
//
// Without this the inbound endpoint is a public form that will write messages
// into anybody's history and, because STOP is honoured on arrival, let a
// stranger opt any number out of ever hearing from this instance again. Twilio
// signs the full URL plus every POST field sorted by name, HMAC-SHA1 under the
// auth token.
//
// The URL has to be the one Twilio built the signature from, which is the
// public one — behind a proxy the request's own scheme and host are the
// proxy's, so MU_DOMAIN is what gets used when it is set.
func validSignature(r *http.Request, publicURL string, form url.Values) bool {
	token := strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
	got := r.Header.Get("X-Twilio-Signature")
	if token == "" || got == "" {
		return false
	}

	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(publicURL)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(form.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(b.String()))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(got))
}
