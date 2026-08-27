package push

// The subscription store, and the header a push service reads.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func device(endpoint string) Subscription {
	return Subscription{
		Endpoint: endpoint,
		P256dh:   "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth:     "BTBZMqHH6r4Tts7J_aSIgg",
	}
}

// A browser that re-subscribes is the same device, not a second one.
//
// It regenerates its subscription after a permission change, a reinstall, a key
// rotation. Matched on anything but the endpoint, each of those turns into
// another entry and the person gets every notification twice, then three times.
func TestResubscribingIsTheSameDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "push-dupe"

	added, err := Subscribe(who, device("https://fcm.example/x"))
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("the first device is not reported as new, so nothing greets it")
	}
	// The same browser again, which is what the page does on every load now.
	added, err = Subscribe(who, device("https://fcm.example/x"))
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("a device the account already has is reported as new, so it is " +
			"greeted with a notification every time the page loads")
	}
	if got := Devices(who); len(got) != 1 {
		t.Errorf("%d devices after subscribing twice, want 1", len(got))
	}

	// A different endpoint is a different device — a phone as well as a laptop.
	if _, err := Subscribe(who, device("https://fcm.example/y")); err != nil {
		t.Fatal(err)
	}
	if got := Devices(who); len(got) != 2 {
		t.Errorf("%d devices, want the phone and the laptop", len(got))
	}

	Unsubscribe(who, "https://fcm.example/x")
	if got := Devices(who); len(got) != 1 || got[0].Endpoint != "https://fcm.example/y" {
		t.Errorf("unsubscribing removed the wrong one: %+v", got)
	}
}

// A subscription missing a key is refused, rather than stored as one that can
// never be encrypted for.
func TestAnIncompleteSubscriptionIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, s := range []Subscription{
		{P256dh: "x", Auth: "y"},
		{Endpoint: "https://fcm.example/z", Auth: "y"},
		{Endpoint: "https://fcm.example/z", P256dh: "x"},
		{Endpoint: "https://fcm.example/z", P256dh: "not base64!!", Auth: "BTBZMqHH6r4Tts7J_aSIgg"},
	} {
		if _, err := Subscribe("push-bad", s); err == nil {
			t.Errorf("accepted %+v", s)
		}
	}
	if got := Devices("push-bad"); len(got) != 0 {
		t.Errorf("%d devices stored from refused subscriptions", len(got))
	}
}

// Devices go when the account does. A subscription outliving its owner is a
// stranger's phone still receiving somebody's mail.
func TestDevicesGoWithTheAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "push-gone"
	Subscribe(who, device("https://fcm.example/a")) //nolint:errcheck

	Forget(who)
	if got := Devices(who); len(got) != 0 {
		t.Errorf("%d devices survived the account", len(got))
	}
}

// The VAPID header, which a push service refuses if any part of it is wrong.
func TestTheVapidHeaderIsWhatAPushServiceExpects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VAPID_PRIVATE_KEY", "")

	header, err := authorization("https://fcm.googleapis.com/fcm/send/abc123", "mailto:ops@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(header, "vapid t=") || !strings.Contains(header, ", k=") {
		t.Fatalf("the header is not in the vapid scheme: %q", header)
	}

	token := strings.TrimPrefix(strings.SplitN(header, ", k=", 2)[0], "vapid t=")
	key := strings.SplitN(header, ", k=", 2)[1]

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("the token has %d parts, want three", len(parts))
	}
	var head map[string]string
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &head) != nil {
		t.Fatalf("the header is not base64url JSON: %v", err)
	}
	if head["alg"] != "ES256" || head["typ"] != "JWT" {
		t.Errorf("the token declares %+v", head)
	}

	var claims map[string]any
	raw, _ = base64.RawURLEncoding.DecodeString(parts[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	// The origin and nothing else. A push service rejects a token whose aud
	// carries the path, and the path is the subscription — a secret it has.
	if claims["aud"] != "https://fcm.googleapis.com" {
		t.Errorf("aud is %v, want the endpoint's origin alone", claims["aud"])
	}
	if claims["sub"] != "mailto:ops@example.com" {
		t.Errorf("sub is %v", claims["sub"])
	}
	if _, ok := claims["exp"]; !ok {
		t.Error("no expiry, which every push service requires")
	}

	// Raw r||s, 64 bytes. ASN.1 is what SignASN1 gives and is not what ES256
	// is — a push service reading DER here sees a bad signature.
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 64 {
		t.Errorf("the signature is %d bytes, want 64 raw r||s", len(sig))
	}

	// And the key it names is the one a browser subscribes with: an
	// uncompressed P-256 point.
	pub, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(pub) != 65 || pub[0] != 4 {
		t.Errorf("k= is not an uncompressed P-256 point: %d bytes, err %v", len(pub), err)
	}
	if pk := PublicKey(); pk != key {
		t.Errorf("the header names a different key from the page: %s vs %s", key, pk)
	}
}

// The key is minted once and kept. A new one silently invalidates every
// subscription anybody has made, because a browser binds its subscription to
// the key it was created with.
func TestTheKeyDoesNotChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VAPID_PRIVATE_KEY", "")

	first := PublicKey()
	if first == "" {
		t.Fatal("no key was minted")
	}
	if second := PublicKey(); second != first {
		t.Errorf("asking twice gave two keys:\n%s\n%s", first, second)
	}
}

// A push endpoint is reduced to its host for a log line. The path is the
// subscription and is a credential.
func TestALogLineDoesNotCarryTheSubscription(t *testing.T) {
	const endpoint = "https://fcm.googleapis.com/fcm/send/cJ3xk-secret-token"
	if got := short(endpoint); got != "fcm.googleapis.com" {
		t.Errorf("short() gave %q", got)
	}
	if strings.Contains(short(endpoint), "secret") {
		t.Error("the subscription is in the log line")
	}
}

// A subscription the server has forgotten comes back on its own.
//
// The server can stop knowing about a device in several ordinary ways: the
// browser rotates its endpoint and the old one starts returning 410, which
// prunes it; an instance is restored from a backup taken before you subscribed;
// a deploy goes out from a machine with a stale store. None of them touch the
// browser, which is still holding a good subscription — so the only way back
// used to be a person noticing the card had gone quiet and pressing the button
// again. That is "every time I turn on notifications it gets reset".
//
// The page now re-posts whatever the browser holds on every load. This is the
// property that makes that safe and sufficient: re-registering restores the
// device, and says it is not new, so nobody is greeted twice.
func TestAForgottenDeviceComesBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "push-forgotten"
	held := device("https://fcm.example/held")

	if _, err := Subscribe(who, held); err != nil {
		t.Fatal(err)
	}
	// The server loses it — a 410 from the push service is exactly this call.
	Unsubscribe(who, held.Endpoint)
	if Knows(who, held.Endpoint) {
		t.Fatal("the device was not removed, so this proves nothing")
	}

	// The page loads and hands over what the browser still has.
	added, err := Subscribe(who, held)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("a device the server had forgotten is not treated as new")
	}
	if !Knows(who, held.Endpoint) {
		t.Error("re-registering did not restore the device")
	}
	if got := Devices(who); len(got) != 1 {
		t.Errorf("%d devices, want the one that came back", len(got))
	}
}

// Notifications can be turned off, and the card offers it.
//
// /push/unsubscribe existed from the first version with nothing calling it.
// The card had "Turn on for this device" and "Send a test" and no way back, so
// the only way to stop notifications was to revoke the permission in browser
// settings — which does not tell this instance, leaving a row it goes on
// sending to until a push service happens to answer 410.
func TestNotificationsCanBeTurnedOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := Subscribe("acct", device("https://push.example/one")); err != nil {
		t.Fatal(err)
	}
	if !Subscribed("acct") {
		t.Fatal("the device was not recorded")
	}

	Unsubscribe("acct", "https://push.example/one")
	if Subscribed("acct") {
		t.Error("the device is still on the list after being turned off")
	}
}

// And the card renders the control, so the endpoint is reachable by a person
// rather than only by curl.
func TestTheCardOffersAWayToTurnItOff(t *testing.T) {
	if !strings.Contains(cardJS, "/notify/unsubscribe") {
		t.Error("the notifications card never calls /notify/unsubscribe, so it can " +
			"be turned on and not off")
	}
	if !strings.Contains(cardJS, "pushManager.getSubscription") ||
		!strings.Contains(cardJS, "unsubscribe()") {
		t.Error("turning it off does not unsubscribe the browser, so the next page " +
			"load re-registers the device it just removed")
	}
}

// What became of the last notification is recorded, because "I turned it on
// and never received one" was otherwise unanswerable: a row in the store says
// a device was registered, not that anything reached it.
func TestTheLastResultIsRemembered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := Subscribe("acct", device("https://push.example/one")); err != nil {
		t.Fatal(err)
	}
	if sent, failed, _ := LastResult("acct"); !sent.IsZero() || !failed.IsZero() {
		t.Fatal("a device nothing has been sent to reports a result")
	}

	record("acct", "https://push.example/one", "the push service refused it (403)")
	sent, failed, reason := LastResult("acct")
	if !sent.IsZero() {
		t.Error("a refusal was recorded as a delivery")
	}
	if failed.IsZero() || reason == "" {
		t.Error("a refusal left no trace, so the card still says everything is fine")
	}

	record("acct", "https://push.example/one", "")
	if sent, _, _ := LastResult("acct"); sent.IsZero() {
		t.Error("a successful send was not recorded")
	}
}

// A test that cannot fail is not a test.
//
// /push/test called Send, which hands each device to a goroutine and returns
// nothing, and then answered ok. Pressing "Send a test" therefore said the same
// thing whether the push service took the notification, timed out, or refused
// it outright — and the button appeared to do nothing at all, because the page
// only spoke up on a failure it was never told about.
func TestATestSaysWhatActuallyHappened(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var got int
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer refuse.Close()

	if _, err := Subscribe("refused", device(refuse.URL+"/sub")); err != nil {
		t.Fatal(err)
	}
	err := SendNow("refused", Notification{Title: "Test notification"})
	if err == nil {
		t.Fatal("a push service that refused the notification was reported as a success")
	}
	if got == 0 {
		t.Fatal("nothing was actually sent, so the test button proves nothing")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("the reason does not say what the push service said: %v", err)
	}
	if _, failed, reason := LastResult("refused"); failed.IsZero() || reason == "" {
		t.Error("the refusal was not recorded, so the card still reads as fine")
	}

	accept := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("the notification went out unsigned")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer accept.Close()

	if _, err := Subscribe("taken", device(accept.URL+"/sub")); err != nil {
		t.Fatal(err)
	}
	if err := SendNow("taken", Notification{Title: "Test notification"}); err != nil {
		t.Fatalf("a push service that accepted the notification was reported as a failure: %v", err)
	}
	if sent, _, _ := LastResult("taken"); sent.IsZero() {
		t.Error("the delivery was not recorded")
	}
}

// And with nothing subscribed it says so rather than reporting a send.
func TestATestWithNoDeviceSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	err := SendNow("nobody", Notification{Title: "Test notification"})
	if err == nil {
		t.Fatal("a test with no device registered was reported as sent")
	}
}
