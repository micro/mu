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
func save() {
	snapshot := make(map[string][]*Subscription, len(subs))
	for k, v := range subs {
		snapshot[k] = append([]*Subscription(nil), v...)
	}
	go data.SaveJSON(storeFile, snapshot) //nolint:errcheck
}

// Subscribe records a device, or updates the one already there.
//
// Matched on the endpoint, because that is what the browser regenerates: a
// device that re-subscribes — after a permission change, a reinstall, a key
// rotation — must not become a second entry that gets every notification twice.
func Subscribe(account string, s Subscription) error {
	if account == "" || s.Endpoint == "" || s.P256dh == "" || s.Auth == "" {
		return fmt.Errorf("that subscription is missing its endpoint or its keys")
	}
	if _, err := b64.DecodeString(s.P256dh); err != nil {
		return fmt.Errorf("the subscription key is not base64url")
	}
	if _, err := b64.DecodeString(s.Auth); err != nil {
		return fmt.Errorf("the subscription secret is not base64url")
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
			return nil
		}
	}
	subs[account] = append(subs[account], &s)
	save()
	return nil
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

// Forget removes an account's devices, for account deletion.
func Forget(account string) {
	load()
	mu.Lock()
	defer mu.Unlock()
	delete(subs, account)
	save()
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
		return
	}
	if len(n.Body) > bodyLimit {
		n.Body = n.Body[:bodyLimit] + "…"
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return
	}
	for _, d := range devices {
		go deliver(account, d, payload)
	}
}

// deliver sends to one device.
func deliver(account string, d Subscription, payload []byte) {
	p256dh, err := b64.DecodeString(d.P256dh)
	if err != nil {
		return
	}
	auth, err := b64.DecodeString(d.Auth)
	if err != nil {
		return
	}
	body, err := encrypt(p256dh, auth, payload)
	if err != nil {
		app.Log("push", "could not encrypt for a device: %v", err)
		return
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
		return
	}

	req, err := http.NewRequest(http.MethodPost, d.Endpoint, bytes.NewReader(body))
	if err != nil {
		return
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
		return
	}
	defer rsp.Body.Close()

	switch {
	case rsp.StatusCode == http.StatusNotFound || rsp.StatusCode == http.StatusGone:
		// The device is gone: the browser was uninstalled, the permission
		// revoked, the subscription expired. Keeping it means a failed request
		// per notification forever.
		app.Log("push", "a device is gone, removing it")
		Unsubscribe(account, d.Endpoint)
	case rsp.StatusCode >= 400:
		app.Log("push", "%s refused a notification: %d", short(d.Endpoint), rsp.StatusCode)
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
