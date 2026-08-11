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

// send hands one message to the provider and returns its id.
//
// A variable so a test can stand in front of it. Everything interesting about
// this service is what happens either side of the provider call — the refusals
// before it and the bookkeeping after — and none of that was reachable by a
// test while the only way to get past this line was a live account.
var send = deliver

// deliver is the real thing.
func deliver(to, body string) (string, error) {
	sid := AccountSID()
	user, pass := credentials()
	if sid == "" || user == "" || pass == "" {
		return "", fmt.Errorf("this instance cannot send texts — no provider is configured")
	}

	form := url.Values{"To": {to}, "Body": {body}}
	if svc := messagingService(); svc != "" {
		// The sender pool picks the number, which is what it is for: with
		// Geomatch on it sends from the one in the handset's own country, and
		// it knows which of them are registered for what.
		form.Set("MessagingServiceSid", svc)
	} else if from := FromFor(to); from != "" {
		form.Set("From", from)
	} else {
		return "", fmt.Errorf("no number is configured for that country — this instance sends from %s",
			strings.Join(Senders(), ", "))
	}
	req, err := http.NewRequest(http.MethodPost,
		"https://api.twilio.com/2010-04-01/Accounts/"+url.PathEscape(sid)+"/Messages.json",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(user, pass)
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
		Code    int    `json:"code"`
	}
	json.Unmarshal(b, &out) //nolint:errcheck

	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		// 21610 is Twilio's own opt-out list saying no. A Messaging Service
		// with Advanced Opt-Out answers STOP itself, so that message never
		// reaches our webhook and our list never learns — this is where it
		// finds out, and after this the refusal costs nothing.
		if out.Code == 21610 {
			OptOut(to)
		}
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

// AccountSID is the account that owns the numbers, always an AC.
//
// An API Key SID starts SK and is a credential, not an account. Twilio accepts
// one for sending, so putting a key here works and looks configured — and then
// the webhook signature, which is made with the account's own auth token,
// cannot be checked against anything. Outbound fine, inbound refused, and no
// obvious connection between the two. So a key is treated as a key.
func AccountSID() string {
	sid := strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID"))
	if strings.HasPrefix(sid, "AC") {
		return sid
	}
	return ""
}

// credentials are what the API call authenticates with: an API key and secret
// where there is one, and the account and its auth token otherwise.
func credentials() (string, string) {
	key := strings.TrimSpace(settings.Get("TWILIO_API_KEY"))
	secret := strings.TrimSpace(settings.Get("TWILIO_API_SECRET"))

	// A key left in TWILIO_ACCOUNT_SID is still a key. Read it as one rather
	// than fail, and say so elsewhere — the alternative is refusing to send for
	// a setup that was sending a minute ago.
	if key == "" {
		if sid := strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID")); strings.HasPrefix(sid, "SK") {
			key, secret = sid, strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
		}
	}
	if key != "" && secret != "" {
		return key, secret
	}
	return AccountSID(), authToken()
}

// authToken is the account's auth token, which is the only thing a webhook
// signature is ever made with — an API key secret will not do.
//
// Empty when TWILIO_ACCOUNT_SID holds a key, because then the token beside it
// is that key's secret and using it would only produce a signature that never
// matches, which is the failure this whole detour was.
func authToken() string {
	if strings.HasPrefix(strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID")), "SK") {
		return ""
	}
	return strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
}

// validSignature reports whether a webhook really came from Twilio, against
// every address the request might have been signed as.
//
// A list rather than one URL, because the signature covers the URL as Twilio
// called it and this process cannot see that. Behind a proxy the scheme is
// https out there and http in here, the host is the proxy's, and a
// configured address may or may not carry the www. Twilio's own helper
// libraries take the URL as an argument for exactly this reason — the
// reconstruction is the part that goes wrong, and it fails closed and silently,
// which looks from the outside like the endpoint being down.
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
func validSignature(r *http.Request, candidates []string, form url.Values) bool {
	token := authToken()
	got := r.Header.Get("X-Twilio-Signature")
	if token == "" || got == "" {
		return false
	}

	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var params strings.Builder
	for _, k := range keys {
		params.WriteString(k)
		params.WriteString(form.Get(k))
	}

	for _, url := range candidates {
		mac := hmac.New(sha1.New, []byte(token))
		mac.Write([]byte(url + params.String()))
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(want), []byte(got)) {
			return true
		}
	}
	return false
}
