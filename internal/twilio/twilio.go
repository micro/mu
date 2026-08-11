// Package twilio is the provider underneath anything this instance sends as a
// message: a phone number today, WhatsApp beside it, and both through the same
// endpoint.
//
// It holds three things and no opinions: the credentials, the POST that hands a
// message over, and the signature check that says an inbound webhook really
// came from Twilio. What may be sent, to whom, at what price and under whose
// name belongs to the service asking — text and WhatsApp answer those
// differently and share every line below.
//
// No dependency for any of it. The send is a form post and the signature is an
// HMAC, and a library here would be one more thing trusted with the auth token.
package twilio

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

// Result is what came back from a send.
type Result struct {
	SID  string // the provider's id for the message
	Code int    // the provider's error code, when it refused
}

// Send hands one message over. The caller supplies To, Body and either From or
// MessagingServiceSid — this knows nothing about which.
func Send(form url.Values) (Result, error) {
	sid := PathSID()
	user, pass := Credentials()
	if sid == "" || user == "" || pass == "" {
		return Result{}, fmt.Errorf("no message provider is configured")
	}

	req, err := http.NewRequest(http.MethodPost,
		"https://api.twilio.com/2010-04-01/Accounts/"+url.PathEscape(sid)+"/Messages.json",
		strings.NewReader(form.Encode()))
	if err != nil {
		return Result{}, err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rsp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("could not reach the message provider: %w", err)
	}
	defer rsp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(rsp.Body, 64<<10))
	var out struct {
		SID     string `json:"sid"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	json.Unmarshal(b, &out) //nolint:errcheck

	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		// Twilio's own message is the useful one ("The 'To' number is not a
		// valid mobile number"), and it is safe to pass on: it describes the
		// request the caller just made. The code goes back too, because some of
		// them mean something to the service — an opt-out, a closed window.
		if out.Message != "" {
			return Result{Code: out.Code}, fmt.Errorf("%s", out.Message)
		}
		return Result{Code: out.Code}, fmt.Errorf("the message provider refused that (%d)", rsp.StatusCode)
	}
	return Result{SID: out.SID}, nil
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

// PathSID is the SID the send URL is built from.
//
// AccountSID is empty when the account slot holds a key, which is the right
// answer for "can a signature be verified" and the wrong one here: Twilio
// resolves a key in that position, and reading it strictly for both questions
// stopped every outbound message on an instance that had been sending happily.
func PathSID() string {
	if sid := AccountSID(); sid != "" {
		return sid
	}
	return strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID"))
}

// Credentials are what the API call authenticates with: an API key and secret
// where there is one, and the account and its auth token otherwise.
func Credentials() (string, string) {
	key := strings.TrimSpace(settings.Get("TWILIO_API_KEY"))
	secret := strings.TrimSpace(settings.Get("TWILIO_API_SECRET"))

	// A key left in TWILIO_ACCOUNT_SID is still a key. Read it as one rather
	// than fail: the alternative is refusing to send for a setup that was
	// sending a minute ago.
	if key == "" {
		if sid := strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID")); strings.HasPrefix(sid, "SK") {
			key, secret = sid, strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
		}
	}
	if key != "" && secret != "" {
		return key, secret
	}
	return AccountSID(), AuthToken()
}

// AuthToken is the account's auth token, which is the only thing a webhook
// signature is ever made with — an API key secret will not do.
//
// Empty when TWILIO_ACCOUNT_SID holds a key, because then the token beside it
// is that key's secret, and using it would only produce a signature that never
// matches.
func AuthToken() string {
	if strings.HasPrefix(strings.TrimSpace(settings.Get("TWILIO_ACCOUNT_SID")), "SK") {
		return ""
	}
	return strings.TrimSpace(settings.Get("TWILIO_AUTH_TOKEN"))
}

// LooksLikeAuthToken reports whether a value has the shape of an account auth
// token: thirty-two characters, hexadecimal, no more.
//
// An API key secret is the same length and is not hex, and the two are
// indistinguishable by eye in a settings form — which is how one ended up in
// the other's slot and cost an afternoon.
func LooksLikeAuthToken(t string) bool {
	if len(t) != 32 {
		return false
	}
	for _, r := range t {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// ValidSignature reports whether a webhook really came from Twilio, against
// every address the request might have been signed as.
//
// A list rather than one URL, because the signature covers the URL as Twilio
// called it and this process cannot see that: behind a proxy the scheme is
// https outside and http in here, and the host is the proxy's. Twilio's own
// helper libraries take the URL as an argument for exactly this reason — the
// reconstruction is the part that goes wrong, and it fails closed and silently,
// which from outside looks like the endpoint being down.
func ValidSignature(r *http.Request, candidates []string, form url.Values) bool {
	token := AuthToken()
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

	for _, u := range candidates {
		mac := hmac.New(sha1.New, []byte(token))
		mac.Write([]byte(u + params.String()))
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(want), []byte(got)) {
			return true
		}
	}
	return false
}

// SignedURLs is every address a request might have been signed as.
func SignedURLs(r *http.Request) []string {
	path := r.URL.RequestURI()

	var out []string
	add := func(u string) {
		for _, seen := range out {
			if seen == u {
				return
			}
		}
		out = append(out, u)
	}

	// What the operator says it is, which ends any disagreement.
	if u := strings.TrimSpace(settings.Get("TWILIO_WEBHOOK_URL")); u != "" {
		add(strings.TrimSuffix(u, "/"))
	}
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" && d != "localhost" {
		d = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://"), "/")
		add("https://" + d + path)
		add("http://" + d + path)
		add("https://www." + d + path)
	}
	if r.Host != "" {
		add("https://" + r.Host + path)
		add("http://" + r.Host + path)
	}
	return out
}

// VerifyInbound reports whether an arriving message must prove it is genuine.
//
// On by default, because unverified an inbound endpoint lets anybody who knows
// the URL write into a person's message history. Off is a real choice with a
// real cost — an instance authenticating with an API key has no auth token and
// so nothing to check against — and it belongs to whoever runs the instance.
func VerifyInbound() bool {
	v := strings.ToLower(strings.TrimSpace(settings.Get("SMS_VERIFY_INBOUND")))
	return v != "0" && v != "false" && v != "off" && v != "no"
}
