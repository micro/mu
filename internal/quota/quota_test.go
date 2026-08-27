package quota

import "testing"

// Where to top up is a whole address, when the instance knows its own.
//
// The refusal a tool gives is read by a program on another machine as often as
// by a person in a browser, and "/wallet/topup" is only a destination if you
// already know what it is relative to. An agent handed that has to guess an
// origin, or give up.
func TestTheTopupAddressIsOneAProgramCanFollow(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")
	if got, want := TopupURL(), "https://micro.mu/wallet/topup"; got != want {
		t.Errorf("TopupURL = %q, want %q", got, want)
	}

	// An instance that genuinely does not know its own address says the path
	// rather than inventing localhost — see origin.Self.
	t.Setenv("MU_DOMAIN", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("APP_URL", "")
	if got, want := TopupURL(), "/wallet/topup"; got != want {
		t.Errorf("TopupURL with no configured domain = %q, want %q", got, want)
	}
}
