package mail

// Registering to receive mail at an address.
//
// This service used to know what an agent was. There was a `var InboundAgent
// func(InboundMail)` here, filled in from the server's wiring, and a rule
// called shouldWakeAgent — a mail server with a special case for one feature of
// the product on top of it. The next thing that wanted to react to mail would
// have added a second variable and a second special case beside it.
//
// So the shape is a registration instead: something says it handles mail at an
// address, and this dispatches to it without knowing or caring what it is.
//
//	mail.Inbound("agent", func(m mail.InboundMail) { … })
//	mail.Inbound(mail.Tagged, func(m mail.InboundMail) { … })
//
// What stays here is the part that is genuinely mail's business: whether the
// person who wrote in is entitled to be listened to at all. That guard is not
// the handler's to make — it needs the SPF and DKIM results, which exist only
// inside the SMTP session, and it is the same question whatever registers.

import (
	"strings"
	"sync"

	"mu/internal/app"
)

// Tagged is the address every account has a private version of: you+anything@.
// Registering for it is registering for every user's tagged mail, which is what
// an agent roster wants — the handler works out which agent a tag names.
const Tagged = "+"

// InboundHandler reacts to a message that has already been stored.
//
// It is called on its own goroutine after delivery, so taking a while, failing,
// or panicking cannot cost the sender their mail or hold the SMTP session open.
type InboundHandler func(InboundMail)

var (
	inboundMu       sync.RWMutex
	inboundHandlers = map[string][]InboundHandler{}
	deliverHandlers []InboundHandler
)

// Inbound registers a handler for mail arriving at a local address.
//
// local is the part before the @ — "agent", "support" — or Tagged for any
// user's plus-addressed mail. Registering the same address twice adds a second
// handler rather than replacing the first; nothing here owns an address.
func Inbound(local string, h InboundHandler) {
	if h == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(local))
	if key == "" {
		return
	}
	inboundMu.Lock()
	inboundHandlers[key] = append(inboundHandlers[key], h)
	inboundMu.Unlock()
}

// Delivered registers a handler for every message that lands in a local inbox,
// whatever address it arrived at and whoever sent it.
//
// The difference from Inbound is the whole reason both exist. Inbound asks who
// may *wake* something — a stranger holding your agent's address must not be
// able to drive it and spend your credits, so that dispatch is gated on SPF or
// DKIM and on the sender being somebody this account knows. Delivered asks what
// *arrived*, which is a question of fact and has no such gate: mail from
// somebody you have never met is still mail you were sent.
//
// Conflating them is how the inbox came to hold only the conversations you had
// started. Nothing but agent-addressed mail was ever handed on, so nothing but
// agent-addressed mail was ever written to the record, and /inbox — a page whose
// whole claim is that things turn up in it — showed an empty list to an account
// with a full mailbox.
//
// Spam is the one exclusion, and it is made here rather than by each handler:
// it was refused at the door in every sense that matters, and a record full of
// it is not a record of anything.
func Delivered(h InboundHandler) {
	if h == nil {
		return
	}
	inboundMu.Lock()
	deliverHandlers = append(deliverHandlers, h)
	inboundMu.Unlock()
}

// deliveredHandlers returns what is registered for every delivery.
func deliveredHandlers() []InboundHandler {
	inboundMu.RLock()
	defer inboundMu.RUnlock()
	return append([]InboundHandler(nil), deliverHandlers...)
}

// handlersFor returns what is registered for a message's address.
func handlersFor(m InboundMail) []InboundHandler {
	key := Tagged
	if m.Shared {
		key = AgentMailbox
	}
	inboundMu.RLock()
	defer inboundMu.RUnlock()
	// Copied, because a handler may register another and the lock is not held
	// while they run.
	return append([]InboundHandler(nil), inboundHandlers[key]...)
}

// anyRegistered reports whether anything at all is listening.
//
// Used by the guard so an instance with no handlers does not do the work of
// deciding whether a stranger may wake something that does not exist.
func anyRegistered() bool {
	inboundMu.RLock()
	defer inboundMu.RUnlock()
	return len(inboundHandlers) > 0
}

// deliverInbound hands a stored message to whatever registered for its address.
//
// Called after the message is saved, so the mail is in the inbox whether or not
// anything picks it up, and a handler that fails never loses it.
func deliverInbound(m InboundMail, r wakeRequest) {
	// What arrived, before and regardless of who may wake what. See Delivered.
	if !r.IsSpam {
		dispatch(deliveredHandlers(), m)
	}
	if !mayDispatch(r) {
		return
	}
	dispatch(handlersFor(m), m)
}

// dispatch runs handlers, each on its own goroutine and none able to bring the
// server down with it.
func dispatch(handlers []InboundHandler, m InboundMail) {
	for _, h := range handlers {
		go func(h InboundHandler) {
			defer func() {
				// A handler is somebody else's code. It must not be able to
				// take the mail server down with it.
				if rec := recover(); rec != nil {
					app.Log("mail", "an inbound handler panicked: %v", rec)
				}
			}()
			h(m)
		}(h)
	}
}
