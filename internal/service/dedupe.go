package service

// One request, one charge.
//
// A live image generation debited 60 credits — four charges of 15 — and put one
// image in the gallery. One prompt, one picture, four times the money.
//
// The shape that produces this is a slow endpoint. Generating an image polls a
// provider for up to 150 seconds, which is longer than the read timeout of a
// good deal of the software between a caller and here: a proxy gives up waiting
// and re-sends, a client library retries on what looks like a dropped
// connection, an agent framework retries a tool call it never saw an answer to.
// Every one of those is a well-behaved participant doing the correct thing with
// a request it believes was lost. What arrives is the same call several times
// over, and a gateway that counts calls charges for all of them.
//
// It cannot be fixed by making the endpoint faster; it is slow because the
// provider is slow. It cannot be fixed by asking callers not to retry, because
// retrying is right — the alternative is a caller that loses work whenever a
// connection breaks. The only place it can be fixed is here, by being able to
// tell that two arrivals are one request.
//
// Two ways to know that, and they are not equally strong.
//
// **An explicit request id.** A caller that stamps one is saying "these are the
// same request" in as many words. That is authoritative, so it is honoured for
// as long as replayFor: a retry that arrives after the first has finished gets
// the first one's answer back, and is charged nothing, because it is not a
// second call.
//
// **The request itself, while an identical one is still running.** Without an
// id, two identical calls from the same account are ambiguous — somebody may
// simply want two images of a red square. But *overlapping* identical calls are
// not ambiguous in practice: a person asking twice asks twice in a row, having
// seen the first answer. Concurrency is the signature of a retry, because a
// retry is by definition sent before the first was known to have finished. So
// identical calls that overlap are collapsed into one, and sequential ones are
// left alone — which is the distinction that matters and the reason this is not
// simply a response cache.
//
// What is deliberately not here: deduplication of free calls. A free call
// arriving twice costs nobody anything, and collapsing it would be this file
// deciding it knows better than a caller who asked for something twice.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sync"
	"time"
)

// replayFor is how long a completed call is kept so an explicit retry of it can
// be answered. Long enough to cover a retry of the slowest thing here — image
// generation polls for up to 150 seconds, and a client that gives up on that
// waits its own timeout before trying again — and short enough that this is a
// table of things in the last few minutes rather than a cache.
const replayFor = 10 * time.Minute

// sweepAbove is when a claim pays for a sweep of expired entries. There is no
// timer: entries are cheap, the map is bounded by concurrent traffic, and a
// background goroutine that exists to tidy a map is a goroutine that has to be
// reasoned about at shutdown.
const sweepAbove = 256

// call is one request: in flight, or finished and kept for replay.
type call struct {
	done chan struct{} // closed when the handler has returned

	rsp interface{} // the response the handler filled in, for replay
	err error

	at   time.Time
	keep bool // stamped with a request id, so keep it after it finishes
}

var (
	callsMu sync.Mutex
	calls   = map[string]*call{}
)

// dedupeKey identifies this request, or "" when it cannot be identified and the
// call must simply run.
//
// An explicit request id wins: it is the caller telling us directly. Failing
// that, the request body is hashed — two calls with the same account, the same
// operation and the same arguments are the same request, for as long as one of
// them is still running.
//
// The account is in the key because two accounts asking the same thing are two
// requests, and collapsing them would charge one of them nothing and hand it
// the other's answer.
func dedupeKey(ctx context.Context, account, operation string, body interface{}) (key string, explicit bool) {
	if id := RequestFrom(ctx); id != "" {
		return account + "\x00" + operation + "\x00id:" + id, true
	}
	raw, err := json.Marshal(body)
	if err != nil {
		// A body that will not marshal cannot be fingerprinted, so this call
		// cannot be told apart from any other. It runs, and it is charged.
		return "", false
	}
	sum := sha256.Sum256(raw)
	return account + "\x00" + operation + "\x00" + hex.EncodeToString(sum[:16]), false
}

// claim registers this call as the one in flight for key.
//
// It returns the call already running under that key, if there is one, and
// whether this goroutine is the one that has to do the work. Only the owner
// runs the handler and only the owner is charged; everybody else waits on the
// first one's answer.
func claim(key string, explicit bool) (first *call, mine bool) {
	callsMu.Lock()
	defer callsMu.Unlock()

	if c, ok := calls[key]; ok {
		select {
		case <-c.done:
			// Finished. Only an explicit request id replays a finished call:
			// without one, a second identical request that arrives after the
			// first has completed is a second request.
			if c.keep && explicit && time.Since(c.at) < replayFor {
				return c, false
			}
			delete(calls, key)
		default:
			// Still running, so this is a retry of it whichever way it was keyed.
			return c, false
		}
	}

	if len(calls) > sweepAbove {
		sweepLocked()
	}
	c := &call{done: make(chan struct{}), at: time.Now(), keep: explicit}
	calls[key] = c
	return c, true
}

// finish records the outcome and releases everybody waiting on it.
//
// A call with no request id is forgotten as soon as it finishes: it existed
// only to collapse the retries that overlapped it, and keeping it would start
// answering a later, legitimate repeat with a stale result.
func finish(key string, c *call, rsp interface{}, err error) {
	callsMu.Lock()
	c.rsp, c.err, c.at = rsp, err, time.Now()
	if !c.keep || err != nil {
		// A failure is never replayed. The caller retried because it did not get
		// an answer, and handing back the error the first attempt got would turn
		// one transient provider failure into a permanent one for ten minutes.
		delete(calls, key)
	}
	callsMu.Unlock()
	close(c.done)
}

// sweepLocked drops entries nothing will ask for again. Called with the lock.
func sweepLocked() {
	for k, c := range calls {
		select {
		case <-c.done:
			if time.Since(c.at) > replayFor {
				delete(calls, k)
			}
		default:
		}
	}
}

// replay copies a finished call's response into this call's response struct.
//
// Both are the same endpoint's response type, so this is a plain assignment
// through reflection. It reports whether it could: if it could not — a door
// decoding into a map rather than the struct, say — the caller runs the handler
// normally rather than returning something wrong, and pays for it, which is the
// safe direction to fail in.
func replay(into, from interface{}) bool {
	if into == nil || from == nil {
		return false
	}
	dst, src := reflect.ValueOf(into), reflect.ValueOf(from)
	if dst.Kind() != reflect.Ptr || src.Kind() != reflect.Ptr || dst.IsNil() || src.IsNil() {
		return false
	}
	if dst.Type() != src.Type() || !dst.Elem().CanSet() {
		return false
	}
	dst.Elem().Set(src.Elem())
	return true
}

// ResetDedupe forgets every request in flight. For tests, which would otherwise
// see one case's calls collapsed into another's.
func ResetDedupe() {
	callsMu.Lock()
	calls = map[string]*call{}
	callsMu.Unlock()
}
