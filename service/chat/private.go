package chat

// A room nobody can walk into by guessing its name.
//
// getOrCreateRoom makes a room for any id it is handed. That is correct and is
// the feature for a public room — a room for a news article comes into being
// because somebody opened it, and nobody had to create it first. It is exactly
// wrong for a conversation between two people, and the only thing standing
// between /chat?id=<two names> and somebody else's private conversation today
// is that nothing links to it.
//
// # A namespace, not a flag on a room
//
// The obvious fix is a Private bool on Room, checked on join. It does not hold,
// because the room does not exist yet: the attack is to *create* it. Ask for an
// id nobody has used and getOrCreateRoom hands you a fresh room with that name,
// and when the two people it belongs to open theirs, they join yours.
//
// So privacy is a property of the *name*, decided before any room exists. An id
// under a private prefix is never created by a request. It comes into being when
// somebody with the right to do so calls Open, which records who is in it; a
// request for one either finds you on that list or is told there is no such
// room.
//
// # Told there is no such room, not told you may not enter
//
// Refusing by name confirms the name. "You are not a member of dm_asim_henrik"
// tells a stranger that asim and henrik are talking, which is most of what
// there was to learn. Not found is the honest answer to somebody with no
// standing: for them, there is not.

import (
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// privatePrefix marks the ids that are never public.
//
// One prefix rather than a list, so the rule is checkable by reading the id.
// getOrCreateRoom already splits on the first underscore to decide a room's
// type — see the switch there — so this is the same convention doing one more
// job rather than a second scheme beside it.
const privatePrefix = "dm_"

// members is roomID → the accounts allowed in it.
//
// On disk, because a room that forgets who is in it after a restart is a room
// that locks its own members out. The transcripts are already persisted per
// room; this is the smaller half that says who may read them.
var (
	memberMu sync.RWMutex
	members  = map[string][]string{}
)

// loadPrivate reads the membership record. Called once at Load.
func loadPrivate() {
	memberMu.Lock()
	defer memberMu.Unlock()
	data.LoadJSON("chat_private.json", &members) //nolint:errcheck
}

// savePrivate writes it. Caller holds memberMu.
func savePrivate() {
	data.SaveJSON("chat_private.json", members) //nolint:errcheck
}

// Private reports whether an id names a room that is never public.
//
// A question about the id alone, answerable without touching any state, which
// is the property that makes it safe: the check runs before anything has been
// created, which is where the hole was.
func Private(roomID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(roomID)), privatePrefix)
}

// Listable reports whether a room may be named to somebody who is not in it.
//
// The other half of Private, and the half that was missing. Private answers "may
// this person go in"; nothing answered "may this room be mentioned", so every
// place that assembled a list of what is happening — the channel row across the
// top of a room, Happening now, the JSON catalogue — took every room that was
// not a topic and named it. A conversation between two people is exactly that,
// so the moment they said anything, "@asim and @henrik" appeared as a tab on
// everybody else's screen. Not-found at the door is no use when the page before
// it announces the room.
//
// A predicate rather than a check at each call site, because there were three
// call sites and one of them was written before private rooms existed.
func Listable(roomID string) bool {
	return !Private(roomID)
}

// Member reports whether an account may enter a room.
//
// True for every account when the room is public — that is what public means,
// and saying it here rather than at the call sites keeps one answer to "may I
// go in".
func Member(roomID, account string) bool {
	if !Private(roomID) {
		return true
	}
	account = strings.TrimSpace(account)
	if account == "" {
		return false
	}
	memberMu.RLock()
	defer memberMu.RUnlock()
	for _, m := range members[roomID] {
		if m == account {
			return true
		}
	}
	return false
}

// Open creates a private room with a fixed membership, or adds to one.
//
// The only way a private room comes into existence. A caller that has decided
// two people may talk — the profile's Chat button, an invite — calls this, and
// only then does the id mean anything.
//
// Idempotent, and additive: opening the same pair room twice is the second
// person arriving, not a second room. Adding somebody to an existing one is an
// invite, which is the same operation.
func Open(roomID string, accounts ...string) {
	if !Private(roomID) {
		return
	}
	memberMu.Lock()
	defer memberMu.Unlock()
	have := map[string]bool{}
	for _, m := range members[roomID] {
		have[m] = true
	}
	for _, a := range accounts {
		a = strings.TrimSpace(a)
		if a != "" && !have[a] {
			members[roomID] = append(members[roomID], a)
			have[a] = true
		}
	}
	savePrivate()
}

// Members is who is in a private room, for a caller that may see it.
//
// Empty for a public room rather than every account on the instance: a public
// room has no membership, and returning a list would invite a caller to treat
// "not in the list" as "may not enter", which is the opposite of what public
// means.
func Members(roomID string) []string {
	memberMu.RLock()
	defer memberMu.RUnlock()
	out := append([]string(nil), members[roomID]...)
	sort.Strings(out)
	return out
}

// Mine is the private rooms an account is in.
//
// The other half of Members: that answers "who is in this room", this answers
// "which rooms am I in", and without it a conversation you are a member of is
// reachable only from the profile of the person you had it with. A private room
// is unlisted by construction — nothing links to it and its name is no use to a
// stranger — so the membership record is the only place the answer lives.
//
// Sorted by id, so the caller decides the order it wants. /chat sorts by when
// the room last moved, which needs the transcript rather than this list.
func Mine(account string) []string {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil
	}
	memberMu.RLock()
	defer memberMu.RUnlock()
	var out []string
	for id, who := range members {
		for _, m := range who {
			if m == account {
				out = append(out, id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// PairRoom is the id of the room two people share.
//
// Sorted, so the room asim opens with henrik is the room henrik opens with
// asim. Deterministic rather than stored, because two people have exactly one
// conversation between them and a lookup table would be a second place for that
// fact to be wrong.
//
// Guessable by construction, which is the whole reason for the membership check
// above: knowing the name of a private room gets you nothing.
func PairRoom(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" || a == b {
		return ""
	}
	if a > b {
		a, b = b, a
	}
	return privatePrefix + a + "_" + b
}

// pairTitle names a room after the person you are not.
//
// A conversation between people has no item behind it to look up, so its
// subject is who is in it. Rendered for whoever is reading it would be better —
// "@henrik" to asim and "@asim" to henrik — and Room.Title is one string shared
// by everybody in the room, so it names both. The alternative is a title
// computed per reader, which is a bigger change than a room heading is worth.
func pairTitle(roomID string) string {
	who := Members(roomID)
	if len(who) == 0 {
		// Nobody has opened it. It should not be renderable at all — Handler
		// refuses a private room to a non-member — so this is the belt on that
		// brace rather than a state anybody reaches.
		return "Private"
	}
	for i, m := range who {
		who[i] = "@" + m
	}
	return strings.Join(who, " and ")
}

// has reports whether an account has any connection to this room.
//
// Caller holds room.mutex. Used to tell a person arriving from a person opening
// a second tab: without it a new window announces you to a room you are already
// standing in, and closing one announces that you left while you are still
// talking.
func (room *Room) has(account string) bool {
	for _, c := range room.Clients {
		if c != nil && c.UserID == account {
			return true
		}
	}
	return false
}

// arrival says that somebody came or went.
//
// A message in the room rather than a separate event type, because that is what
// it is: a line in the transcript, in order, that everybody present sees and
// somebody arriving later reads in context. A second channel for it would mean
// the client rendering two kinds of thing into one column and getting the order
// right by hand.
//
// Not in the lobby or a topic room. Those are public and busy by design, and a
// line every time anybody looks in is the thing that makes a public room
// unreadable — IRC solved this by letting people turn joins off, which is an
// admission that they are noise. In a room with two people in it, somebody
// arriving is the most useful thing that can be said.
//
// # Nobody said it
//
// It went out as the agent: UserID micro, IsLLM true. The client renders any
// IsLLM line with micro's byline, so the first thing in a conversation between
// two people was "micro: @asim joined" — which reads as the agent announcing
// itself into a private room, and was reported as exactly that.
//
// So it carries no author and says so. See RoomMessage.System.
func (room *Room) arrival(account, what string) {
	if account == "" || account == agentName || !Private(room.ID) {
		return
	}
	select {
	case room.Broadcast <- RoomMessage{
		Content:   "@" + account + " " + what,
		Timestamp: time.Now(),
		System:    true,
	}:
	default:
	}
}
