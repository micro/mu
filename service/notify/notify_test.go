package notify

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"mu/internal/push"
)

func device(endpoint string) push.Subscription {
	return push.Subscription{
		Endpoint: endpoint,
		P256dh:   "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth:     "BTBZMqHH6r4Tts7J_aSIgg",
	}
}

// The one property everything else rests on: this service can reach you and
// nobody else.
//
// Enforced by the shape rather than by a check, which is why it is worth a test
// that reads the shape. Send takes the account from the context and the request
// has no recipient field, so there is no argument an agent could be talked into
// filling in with somebody else's name. A "to" here would turn a tool that
// notifies you into a tool for making a stranger's phone buzz, and the only
// bound on it would be whatever the model was persuaded of.
func TestThereIsNoWayToNotifySomebodyElse(t *testing.T) {
	fields := reflect.TypeOf(SendRequest{})
	for i := 0; i < fields.NumField(); i++ {
		name := strings.ToLower(fields.Field(i).Name)
		switch name {
		case "to", "account", "recipient", "owner", "user", "who":
			t.Fatalf("SendRequest has a %q field — an agent can now be asked to "+
				"notify somebody who did not ask to be notified", fields.Field(i).Name)
		}
	}
}

// Sending says whether it worked, because the caller asked on purpose.
//
// push.Send is fire-and-forget so that mail delivery does not wait on a push
// service. That is right for mail and wrong here: an agent told to tell you
// something needs to know it could not, or it reports success and the standing
// instruction silently does nothing forever.
func TestSendingReportsWhatHappened(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Send("nodevice", "Something happened", "", "", "test"); err == nil {
		t.Fatal("notifying an account with no device was reported as sent")
	} else if !strings.Contains(err.Error(), "no device") {
		t.Errorf("the reason does not say the account has no device: %v", err)
	}

	took := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer took.Close()
	if _, err := push.Subscribe("reachable", device(took.URL+"/sub")); err != nil {
		t.Fatal(err)
	}
	if err := Send("reachable", "Something happened", "The thing you asked about.", "/home", "test"); err != nil {
		t.Fatalf("a push service that accepted it was reported as a failure: %v", err)
	}

	// And it is written down, which is the whole reason this is a service and
	// not three hardcoded call sites. A notification is the only message this
	// product sends that otherwise leaves no copy anywhere.
	sent := History("reachable", 10)
	if len(sent) != 1 {
		t.Fatalf("history has %d entries, want 1 — nothing records what you were told", len(sent))
	}
	if sent[0].Title != "Something happened" || !sent[0].OK || sent[0].From != "test" {
		t.Errorf("the record does not describe what was sent: %+v", sent[0])
	}
}

// A title is the whole message on a watch, so there has to be one.
func TestANotificationNeedsATitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Send("someone", "   ", "a body nobody will see", "", "test"); err == nil {
		t.Fatal("a notification with no title was accepted")
	}
}

// Turning notifications off is not the same as erasing what you were told.
//
// push.Forget is the "turn it off" button and is also what the account deletion
// hook used to call. One function meaning both is how somebody pressing the
// first quietly gets the second — so they are two, and only DeleteAll takes the
// history.
func TestTurningItOffKeepsTheRecordAndDeletingTheAccountDoesNot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	took := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer took.Close()
	if _, err := push.Subscribe("leaver", device(took.URL+"/sub")); err != nil {
		t.Fatal(err)
	}
	if err := Send("leaver", "You were told this", "", "", "test"); err != nil {
		t.Fatal(err)
	}

	push.Forget("leaver")
	if len(Devices("leaver")) != 0 {
		t.Error("turning it off left a device on the list")
	}
	if len(History("leaver", 10)) == 0 {
		t.Error("turning notifications off erased the record of what you were " +
			"already told, which is not what that button says it does")
	}

	DeleteAll("leaver")
	if len(History("leaver", 10)) != 0 {
		t.Error("deleting the account left behind a record of what it was told")
	}
}
