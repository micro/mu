package events

// The calendar somebody already uses.
//
// events stores what Mu was asked to schedule, which is a real calendar and a
// small one: nobody moves their working week into a new tool because a new tool
// asked. So the Spec's promise — "what is scheduled, and when you are free" —
// was true only about the events Mu itself created, and "when are you free" is
// exactly the question where a partial answer is worse than none. Offering a
// Thursday that is actually full is not a smaller version of being useful.
//
// These hooks let main.go attach an outside calendar without this package
// importing a client for it. Same pattern as OnFire and OnCreate: events stays
// the thing that owns scheduling, and knows nothing about who else keeps one.
//
// Unset hooks are the normal case — a self-hosted Mu with no Google credentials
// is not degraded, it just has one calendar instead of two.

import "time"

// External is one entry on a calendar Mu does not own. Read-only by
// construction: there is no id here to cancel by, because Mu cannot cancel it.
type External struct {
	Title    string
	Start    time.Time
	End      time.Time
	Location string
	AllDay   bool
	// Source names where it came from, for a UI that must never imply Mu
	// scheduled something it did not.
	Source string
}

// Length is how long the entry occupies, with the same half-hour default the
// rest of this package uses for something with no stated end.
func (e External) Length() time.Duration {
	if d := e.End.Sub(e.Start); d > 0 {
		return d
	}
	return defaultDuration
}

var (
	// ExternalBusy returns booked periods from an attached calendar. Set by
	// main.go when Google is configured.
	ExternalBusy func(owner string, from, to time.Time) []Slot

	// ExternalEntries returns what is scheduled on an attached calendar.
	ExternalEntries func(owner string, from, to time.Time) []External

	// ExternalConnected reports whether this owner has attached one. Kept
	// separate from the two above so the UI can tell "nothing booked" apart
	// from "nothing connected" — the difference between a free week and an
	// unanswered question.
	ExternalConnected func(owner string) bool

	// ExternalAccount names which account is attached, for showing somebody
	// which calendar they connected.
	ExternalAccount func(owner string) string

	// ExternalName is what to call it in front of a person.
	ExternalName = "Google Calendar"
)

// externalBusy is the guarded call: a hook that is unset, or an owner who never
// connected anything, contributes nothing rather than failing.
func externalBusy(owner string, from, to time.Time) []Slot {
	if ExternalBusy == nil || owner == "" {
		return nil
	}
	return ExternalBusy(owner, from, to)
}

// externalEntries is the guarded call for what is scheduled.
func externalEntries(owner string, from, to time.Time) []External {
	if ExternalEntries == nil || owner == "" {
		return nil
	}
	return ExternalEntries(owner, from, to)
}

// HasExternal reports whether this owner has an outside calendar attached.
func HasExternal(owner string) bool {
	return ExternalConnected != nil && owner != "" && ExternalConnected(owner)
}

// CanConnectExternal reports whether attaching one is offered at all on this
// instance — false when no client is wired, so no UI dangles a button that
// cannot work.
func CanConnectExternal() bool { return ExternalConnected != nil }
