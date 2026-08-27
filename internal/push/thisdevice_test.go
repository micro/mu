package push

// "It should appear on this device" has to be a claim somebody checked.
//
// Reported: the button said Sent and nothing arrived. The button sent to the
// *account*, and an account can hold several subscriptions — an old browser, a
// laptop, the same phone from before it re-subscribed. SendNow returned as soon
// as one of them was accepted, so the answer meant "some device took it" while
// the page said "this device". A test that proves a different handset works is
// worse than no test: it sends you hunting for the fault on a device nothing
// was ever sent to.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// twoDevices registers a phone and a laptop and reports what each received.
func twoDevices(t *testing.T, account string) (phone, laptop string, phoneGot, laptopGot *int) {
	t.Helper()
	pn, ln := 0, 0

	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pn++
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(ps.Close)
	ls := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ln++
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(ls.Close)

	// Registered laptop-first, because the bug was "the first one that answers"
	// and a test that registers the target first passes on the broken code.
	for _, e := range []string{ls.URL + "/laptop", ps.URL + "/phone"} {
		if _, err := Subscribe(account, device(e)); err != nil {
			t.Fatal(err)
		}
	}
	return ps.URL + "/phone", ls.URL + "/laptop", &pn, &ln
}

// The device that asked is the device that gets it.
func TestATestGoesToTheDeviceThatAskedForIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	phone, _, phoneGot, laptopGot := twoDevices(t, "two")

	if err := SendToDevice("two", phone, Notification{Title: "Test notification"}); err != nil {
		t.Fatalf("sending to this device failed: %v", err)
	}
	if *phoneGot != 1 {
		t.Errorf("the device that asked received %d notifications", *phoneGot)
	}
	if *laptopGot != 0 {
		t.Errorf("a device that did not ask received %d — the answer says "+
			"\"this device\" and it went somewhere else", *laptopGot)
	}
}

// And an endpoint this account does not hold is refused rather than quietly
// broadcast, which would put the old bug back with an extra step.
func TestATestForAnUnknownDeviceIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, phoneGot, laptopGot := twoDevices(t, "two")

	err := SendToDevice("two", "https://push.example.test/somebody-else", Notification{Title: "x"})
	if err == nil {
		t.Fatal("an endpoint this account does not hold was accepted")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("the reason does not say what is wrong: %v", err)
	}
	if *phoneGot+*laptopGot != 0 {
		t.Error("an unknown endpoint fell back to sending to everything")
	}
}

// SendNow, when it is used, reaches every device rather than stopping at the
// first that answers. Sending to one and calling it done left the rest of an
// account's devices silent with nothing recording that they had been skipped.
func TestSendNowReachesEveryDevice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, phoneGot, laptopGot := twoDevices(t, "two")

	if err := SendNow("two", Notification{Title: "Test notification"}); err != nil {
		t.Fatalf("sending failed: %v", err)
	}
	if *phoneGot != 1 || *laptopGot != 1 {
		t.Errorf("stopped after the first device: phone %d, laptop %d",
			*phoneGot, *laptopGot)
	}
}

// One device refusing does not hide the one that worked, and does not report
// success for the one that did not.
func TestOneRefusalDoesNotSpeakForTheOthers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dead.Close()
	live := 0
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		live++
		w.WriteHeader(http.StatusCreated)
	}))
	defer alive.Close()

	for _, e := range []string{dead.URL + "/dead", alive.URL + "/alive"} {
		if _, err := Subscribe("mixed", device(e)); err != nil {
			t.Fatal(err)
		}
	}
	if err := SendNow("mixed", Notification{Title: "Test notification"}); err != nil {
		t.Errorf("one dead device made the whole send a failure: %v", err)
	}
	if live != 1 {
		t.Error("the working device was skipped because a dead one came first")
	}
	// And asking the dead one directly still says so.
	if err := SendToDevice("mixed", dead.URL+"/dead",
		Notification{Title: "Test notification"}); err == nil {
		t.Error("a device that refused was reported as having taken it")
	}
}

// And the handler passes it through.
//
// The unit above proves SendToDevice targets one subscription; this proves
// /push/test actually gives it one. Logic right and wiring wrong is the shape
// of every push bug so far — the button that answered ok unconditionally was
// the same fault one layer up.
func TestTheTestEndpointHonoursTheDeviceItWasGiven(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "wired"
	if _, err := auth.GetAccount(who); err != nil {
		if err := auth.Create(&auth.Account{ID: who, Name: who, Created: time.Now()}); err != nil {
			t.Fatalf("could not create %s: %v", who, err)
		}
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	phone, _, phoneGot, laptopGot := twoDevices(t, who)

	ask := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/push/test", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		rec := httptest.NewRecorder()
		SubscribeHandler(rec, r)
		return rec
	}

	rec := ask(`{"endpoint":"` + phone + `"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("the test was refused: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"here":true`) {
		t.Errorf("the answer does not claim this device: %s", rec.Body.String())
	}
	if *phoneGot != 1 || *laptopGot != 0 {
		t.Errorf("the endpoint was ignored: phone %d, laptop %d", *phoneGot, *laptopGot)
	}

	// A page cached before this change sends no endpoint. It still works, and
	// it says it went everywhere rather than claiming a device nobody checked.
	*phoneGot, *laptopGot = 0, 0
	rec = ask(``)
	if !strings.Contains(rec.Body.String(), `"here":false`) {
		t.Errorf("a bodyless test still claims this device: %s", rec.Body.String())
	}
	if *phoneGot != 1 || *laptopGot != 1 {
		t.Errorf("the fallback did not reach every device: phone %d, laptop %d",
			*phoneGot, *laptopGot)
	}
}
