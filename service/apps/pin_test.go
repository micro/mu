package apps

// Pinning an app to the top of home happens here, on the row of the app being
// pinned.
//
// It used to be a list of checkboxes on /context, inside the panel that chooses
// what an agent watches, under a heading reading "Apps — pin apps to the top of
// your home screen". A setting about Home, edited on Context, in a panel about
// neither, and there because it is also a list of checkboxes — a resemblance,
// not a reason. It also meant a second list of every public app's name, kept in
// step with this one by nobody.

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAnAppCanBePinnedFromItsOwnRow(t *testing.T) {
	r := httptest.NewRequest("GET", "/apps?tag=timer", nil)

	got := pinControl(r, "reader", "pomodoro", false)
	if !strings.Contains(got, `name="pin" value="pomodoro"`) {
		t.Errorf("no pin for an unpinned app:\n%s", got)
	}
	if !strings.Contains(got, `name="return" value="/apps?tag=timer"`) {
		t.Errorf("pinning would drop the reader on a different page:\n%s", got)
	}
	if !strings.Contains(got, `name="_csrf"`) {
		t.Errorf("a form post that writes the account needs the CSRF field:\n%s", got)
	}

	// Already pinned, the same control is the way back off.
	got = pinControl(r, "reader", "pomodoro", true)
	if !strings.Contains(got, `name="unpin" value="pomodoro"`) {
		t.Errorf("a pinned app has no way to be unpinned:\n%s", got)
	}
}

// Signed out there is no account to pin to, and a switch that cannot do
// anything is worse than no switch.
func TestPinningIsNotOfferedToAGuest(t *testing.T) {
	r := httptest.NewRequest("GET", "/apps", nil)
	if got := pinControl(r, "", "pomodoro", false); got != "" {
		t.Errorf("a guest was offered a pin: %s", got)
	}
}

// Pinning a service to the sidebar on /services is the same gesture with the
// same meaning, and it is a star you fill in. Two pins that look different are
// two features to a reader, so this is the same control — .pin-btn, which is
// in mu.css rather than either page's own style block for that reason.
func TestBothPinsAreTheSamePin(t *testing.T) {
	r := httptest.NewRequest("GET", "/apps", nil)

	got := pinControl(r, "reader", "pomodoro", false)
	if !strings.Contains(got, `class="pin-btn"`) {
		t.Errorf("the app pin rolled its own styling:\n%s", got)
	}
	if !strings.Contains(got, "<polygon") {
		t.Errorf("the app pin is not the star /services uses:\n%s", got)
	}
	if pinned := pinControl(r, "reader", "pomodoro", true); !strings.Contains(pinned, `class="pin-btn pinned"`) {
		t.Errorf("a pinned app does not render as filled in:\n%s", pinned)
	}

	css, err := os.ReadFile("../../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".pin-btn {") {
		t.Error("the shared pin styling is not shared — it is back in one page's style block, " +
			"where the other page cannot see it")
	}
}
