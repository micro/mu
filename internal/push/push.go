package push

// The subscriptions, and sending to them.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
)

// Subscription is one browser on one device that has agreed to be told things.
type Subscription struct {
	// Endpoint is the push service's URL for this device. It is the identity —
	// resubscribing the same browser produces the same one, so it is what a
	// duplicate is matched on.
	Endpoint string `json:"endpoint"`
	// P256dh is the device's public key, base64url, from the subscription.
	P256dh string `json:"p256dh"`
	// Auth is the device's shared secret, base64url, from the subscription.
	Auth string `json:"auth"`
	// Label is what the device is, as far as the browser will say. For a page
	// listing them, so somebody can tell which one to remove.
	Label   string    `json:"label,omitempty"`
	Added   time.Time `json:"added"`
	Account string    `json:"account"`

	// What happened the last time anything was sent here.
	//
	// Because "I turned it on and I have never received one" was not a
	// question this could answer. The card said "On for one device" from the
	// fact that a row existed, which is a claim about the store rather than
	// about anything arriving: a browser that rotated its endpoint, an
	// instance restored from an older backup, a push service quietly refusing
	// — all of them look identical to a row sitting in a file.
	//
	// Sent is when a push service last accepted one. Failed is what it said if
	// it did not. Whichever is later is the truth about this device.
	Sent   time.Time `json:"sent,omitempty"`
	Failed time.Time `json:"failed,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// Notification is what a person sees.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is where tapping it goes. A notification you cannot act on is a
	// notification that trained somebody to ignore the next one.
	URL string `json:"url,omitempty"`
	// Tag collapses notifications about the same thing. Two arrivals in one
	// conversation should replace each other rather than stack.
	Tag string `json:"tag,omitempty"`
	// From is who asked for it — "mail", "reminder", an agent's name.
	//
	// Not on the wire: the handset shows a title and two lines and this is
	// neither. It is for the record, so a list of what interrupted you says
	// what did the interrupting. Somebody who cannot tell which of five things
	// is being noisy turns off all five.
	From string `json:"-"`
}

// ttl is how long a push service should hold a notification for a device that
// is offline. A day: a briefing that turns up two days late is worse than one
// that never arrives.
const ttl = 24 * time.Hour

// bodyLimit trims what is sent. A notification is two lines on a lock screen
// and the rest is payload nobody reads — and the record has to fit one
// encrypted block. See maxPayload.
const bodyLimit = 300

const storeFile = "push_subscriptions.json"

var (
	mu   sync.RWMutex
	subs = map[string][]*Subscription{} // account -> devices
	once sync.Once
)

func load() {
	once.Do(func() {
		data.LoadJSON(storeFile, &subs) //nolint:errcheck
		if subs == nil {
			subs = map[string][]*Subscription{}
		}
	})
}

// save persists the store. Caller holds mu.
//
// Synchronous, and it used to be a goroutine. The write is a small JSON file
// and it happens when somebody subscribes a device or loses one — once per
// device, not once per notification — so the goroutine bought nothing and cost
// determinism: the file landed after the caller had moved on. In the tests that
// meant a write arriving during t.TempDir cleanup and failing the run
// intermittently, which is the visible half. The invisible half is worse — a
// process that exits between the subscribe and the write silently loses the
// device, and the reader is told it worked.
//
// A snapshot is still taken under the caller's lock, because SaveJSON iterates
// and the map must not be written while it does.
func save() {
	snapshot := make(map[string][]*Subscription, len(subs))
	for k, v := range subs {
		snapshot[k] = append([]*Subscription(nil), v...)
	}
	data.SaveJSON(storeFile, snapshot) //nolint:errcheck
}

// Subscribe records a device, or updates the one already there. It reports
// whether this device is new to the account.
//
// Matched on the endpoint, because that is what the browser regenerates: a
// device that re-subscribes — after a permission change, a reinstall, a key
// rotation — must not become a second entry that gets every notification twice.
//
// The bool is for the caller that greets a new device. The page re-registers
// whatever subscription the browser is holding every time it loads, which is
// what makes this survive the server forgetting; without a way to tell a
// re-registration from a first one, that greeting would arrive on every page
// view.
func Subscribe(account string, s Subscription) (bool, error) {
	if account == "" || s.Endpoint == "" || s.P256dh == "" || s.Auth == "" {
		return false, fmt.Errorf("that subscription is missing its endpoint or its keys")
	}
	if _, err := b64.DecodeString(s.P256dh); err != nil {
		return false, fmt.Errorf("the subscription key is not base64url")
	}
	if _, err := b64.DecodeString(s.Auth); err != nil {
		return false, fmt.Errorf("the subscription secret is not base64url")
	}
	s.Account = account
	if s.Added.IsZero() {
		s.Added = time.Now().UTC()
	}

	load()
	mu.Lock()
	defer mu.Unlock()
	for i, have := range subs[account] {
		if have.Endpoint == s.Endpoint {
			s.Added = have.Added
			subs[account][i] = &s
			save()
			return false, nil
		}
	}
	subs[account] = append(subs[account], &s)
	save()
	return true, nil
}

// Knows reports whether an account already has this exact device.
func Knows(account, endpoint string) bool {
	load()
	mu.RLock()
	defer mu.RUnlock()
	for _, have := range subs[account] {
		if have.Endpoint == endpoint {
			return true
		}
	}
	return false
}

// record notes what happened when something was sent to one device.
//
// Best effort, like the send itself: a notification that arrived matters more
// than the note saying so, and this must never be the reason one fails.
func record(account, endpoint string, err string) {
	load()
	mu.Lock()
	defer mu.Unlock()
	for _, have := range subs[account] {
		if have.Endpoint != endpoint {
			continue
		}
		if err == "" {
			have.Sent, have.Error = time.Now().UTC(), ""
		} else {
			have.Failed, have.Error = time.Now().UTC(), err
		}
		save()
		return
	}
}

// Unsubscribe removes one device, by endpoint.
func Unsubscribe(account, endpoint string) {
	load()
	mu.Lock()
	defer mu.Unlock()
	kept := subs[account][:0]
	for _, have := range subs[account] {
		if have.Endpoint != endpoint {
			kept = append(kept, have)
		}
	}
	if len(kept) == 0 {
		delete(subs, account)
	} else {
		subs[account] = kept
	}
	save()
}

// Devices is what an account has subscribed, for a page that lists them.
func Devices(account string) []Subscription {
	load()
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Subscription, 0, len(subs[account]))
	for _, s := range subs[account] {
		out = append(out, *s)
	}
	return out
}

// Subscribed reports whether an account has anywhere to be notified.
func Subscribed(account string) bool { return len(Devices(account)) > 0 }

// Forget removes an account's devices — every device, which is what "turn it
// off" means when the browser cannot say which one it is holding.
//
// Not the history. Turning notifications off is a decision about the future and
// says nothing about the record of what you were already told; erasing that
// would answer "why did I get woken at four" with silence.
func Forget(account string) {
	load()
	mu.Lock()
	defer mu.Unlock()
	delete(subs, account)
	save()
}

// DeleteAll removes everything this package holds for an account, for account
// deletion.
//
// Separate from Forget because Forget is also the "turn it off" button, and one
// function meaning both "stop sending to my phone" and "erase what I was told"
// is how a person pressing the first quietly gets the second.
func DeleteAll(account string) {
	Forget(account)
	ForgetHistory(account)
}

// Send delivers a notification to every device an account has.
//
// Best effort and never blocking the caller: this is called from mail delivery
// and from the briefing, and a push service that is slow must not hold up the
// thing that actually happened. Failures are logged, and a device the service
// says is gone is removed.
func Send(account string, n Notification) {
	if account == "" || strings.TrimSpace(n.Title) == "" || !Configured() {
		return
	}
	devices := Devices(account)
	if len(devices) == 0 {
		// Written down even so. "Nothing was sent because you have no device"
		// is the answer to "why did I not hear about that", and it is invisible
		// unless somebody records the attempt.
		note(account, asSent(n, false, "no device is registered"))
		return
	}
	if len(n.Body) > bodyLimit {
		n.Body = n.Body[:bodyLimit] + "…"
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return
	}
	// One goroutine over all the devices rather than one each, so there is a
	// place for the outcome to be written down once the outcome is known. The
	// caller still does not wait.
	go func() {
		var last error
		ok := false
		for _, d := range devices {
			if err := deliver(account, d, payload); err != nil {
				last = err
				continue
			}
			ok = true
		}
		note(account, asSent(n, ok, reason(ok, last)))
	}()
}

// asSent is one notification as the history keeps it.
func asSent(n Notification, ok bool, why string) Sent {
	return Sent{Title: n.Title, Body: n.Body, URL: n.URL,
		From: source(n.From), OK: ok, Error: why}
}

func reason(ok bool, err error) string {
	if ok || err == nil {
		return ""
	}
	return err.Error()
}

// Test delivers one notification and waits for the answer.
//
// Send is deliberately fire-and-forget: mail delivery must not block on a push
// service. But that makes it the wrong thing behind a button whose entire
// purpose is to say whether this works — /push/test called Send, which cannot
// fail, and answered ok. Pressing it did nothing, said nothing, and left the
// only evidence in a log and in a line on the card that needed a reload to
// change. "I tried send a test, it did nothing" is the correct reading of that.
//
// So this one waits. It reports nil if any device took it, and otherwise the
// reason the last one gave — which is the sentence somebody needs in order to
// do anything about it.
func SendNow(account string, n Notification) error {
	if !Configured() {
		return fmt.Errorf("this instance has no push keys set")
	}
	devices := Devices(account)
	if len(devices) == 0 {
		return fmt.Errorf("no device is registered yet")
	}
	if len(n.Body) > bodyLimit {
		n.Body = n.Body[:bodyLimit] + "…"
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("could not build the notification")
	}
	var last error
	sent := 0
	for _, d := range devices {
		// Every device, not the first that answers.
		//
		// It stopped at the first success and returned, which made the answer
		// "somebody's device took it" while the caller reads it as "mine did".
		// A test button that proves a *different* handset is working is worse
		// than no button: it sends you looking for the fault on the device that
		// was never sent anything.
		if err := deliver(account, d, payload); err != nil {
			last = err
			continue
		}
		sent++
	}
	if sent == 0 {
		note(account, asSent(n, false, reason(false, last)))
		return last
	}
	note(account, asSent(n, true, ""))
	return nil
}

// SendToDevice is one notification to one subscription, and no other.
//
// The test button needs this and nothing else does. "It should appear on this
// device" is a claim about the device holding the page, and the only way to
// make it true is to send to the endpoint that device is holding — an account
// can have several, and which one answers first is not a fact about who is
// looking at the screen.
//
// A stranger's endpoint is not an error, it is a miss: Devices is scoped to
// the account, so an endpoint that is not on it simply is not found.
func SendToDevice(account, endpoint string, n Notification) error {
	if !Configured() {
		return fmt.Errorf("this instance has no push keys set")
	}
	endpoint = strings.TrimSpace(endpoint)
	var target Subscription
	found := false
	for _, d := range Devices(account) {
		if d.Endpoint == endpoint {
			target, found = d, true
			break
		}
	}
	if !found {
		return fmt.Errorf("this device is not registered here — turn notifications off and on again")
	}
	if len(n.Body) > bodyLimit {
		n.Body = n.Body[:bodyLimit] + "…"
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("could not build the notification")
	}
	err = deliver(account, target, payload)
	note(account, asSent(n, err == nil, reason(err == nil, err)))
	return err
}

// Where is the push service behind a device, for a message that has to say
// which one it reached. Apple or Google is the difference between a phone and
// a laptop, and knowing which arrived is most of the diagnosis.
func Where(endpoint string) string { return short(endpoint) }

// LastResult is what happened the last time this account was sent anything,
// across all its devices, for the card on /account to say.
//
// Three answers and they are genuinely different: never sent to, sent and
// accepted, sent and refused. The card used to be able to say only "on", which
// is the one thing that is true in all three.
func LastResult(account string) (sent time.Time, failed time.Time, reason string) {
	for _, d := range Devices(account) {
		if d.Sent.After(sent) {
			sent = d.Sent
		}
		if d.Failed.After(failed) {
			failed, reason = d.Failed, d.Error
		}
	}
	return sent, failed, reason
}

// deliver sends to one device and says what became of it.
//
// The error is for Test, which has somebody waiting on the answer. Send ignores
// it — the record and the log are what it leaves behind.
func deliver(account string, d Subscription, payload []byte) error {
	p256dh, err := b64.DecodeString(d.P256dh)
	if err != nil {
		return fmt.Errorf("that device's key is not base64url")
	}
	auth, err := b64.DecodeString(d.Auth)
	if err != nil {
		return fmt.Errorf("that device's secret is not base64url")
	}
	body, err := encrypt(p256dh, auth, payload)
	if err != nil {
		app.Log("push", "could not encrypt for a device: %v", err)
		record(account, d.Endpoint, "could not encrypt")
		return fmt.Errorf("could not encrypt for that device")
	}
	who := contact()
	if who == "" {
		// A push service requires a contact and rejects the request without
		// one. Better a URL that says which instance than no header at all.
		who = "https://github.com/micro/mu"
	}
	header, err := authorization(d.Endpoint, who)
	if err != nil {
		app.Log("push", "could not sign for %s: %v", short(d.Endpoint), err)
		record(account, d.Endpoint, "could not sign the request")
		return fmt.Errorf("could not sign the request — check the push keys")
	}

	req, err := http.NewRequest(http.MethodPost, d.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("that device's endpoint is not a URL we can post to")
	}
	req.Header.Set("Authorization", header)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.Itoa(int(ttl.Seconds())))
	// Urgency: a notification is worth waking a sleeping handset for, which is
	// the whole point. The default is "normal" and a phone may batch it.
	req.Header.Set("Urgency", "high")

	client := &http.Client{Timeout: 20 * time.Second}
	rsp, err := client.Do(req)
	if err != nil {
		app.Log("push", "could not reach %s: %v", short(d.Endpoint), err)
		record(account, d.Endpoint, "could not reach the push service")
		return fmt.Errorf("could not reach %s", short(d.Endpoint))
	}
	defer rsp.Body.Close()

	switch {
	case rsp.StatusCode == http.StatusNotFound || rsp.StatusCode == http.StatusGone:
		// The device is gone: the browser was uninstalled, the permission
		// revoked, the subscription expired. Keeping it means a failed request
		// per notification forever.
		app.Log("push", "a device is gone, removing it")
		Unsubscribe(account, d.Endpoint)
		return fmt.Errorf("that device is gone — turn it on again")
	case rsp.StatusCode >= 400:
		app.Log("push", "%s refused a notification: %d", short(d.Endpoint), rsp.StatusCode)
		record(account, d.Endpoint, "the push service refused it ("+strconv.Itoa(rsp.StatusCode)+")")
		return fmt.Errorf("%s refused it (%d)", short(d.Endpoint), rsp.StatusCode)
	default:
		// Said out loud, because silence on the happy path is what made "I
		// never receive any" impossible to tell from "none were ever sent".
		app.Log("push", "sent to %s for %s", short(d.Endpoint), account)
		record(account, d.Endpoint, "")
		return nil
	}
}

// short is a push endpoint reduced to its host, for a log line. The path is the
// subscription and is a secret.
func short(endpoint string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	if i := strings.IndexByte(rest, '/'); i > 0 {
		return rest[:i]
	}
	return rest
}
