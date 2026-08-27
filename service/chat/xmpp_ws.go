package chat

// XMPP in the browser, over a WebSocket. RFC 7395.
//
// # Why this exists
//
// So that the browser is a *carrier* of the chat protocol rather than a second
// protocol beside it. This package had two: a hand-written JSON protocol over a
// WebSocket with its own roster and its own file on disk, and an XMPP server
// with its own store. Two implementations of one idea, and every feature after
// that would have had to be written twice or exist in one of them — which is
// how the rooms ended up with no presence, no offline delivery and no record.
//
// Mail settled this a long time ago and nobody thought of it as settling
// anything: SMTP and IMAP are two carriers over one store, and nothing calls
// IMAP "a second mail protocol". This is the same arrangement one rung up.
//
// # What the RFC actually asks for
//
// Very little, which is why it is worth doing rather than approximating. One
// WebSocket message is exactly one stanza — so the framing that a socket gets
// from XML nesting comes from the message boundary instead — and the stream
// tags are replaced by a self-closing <open/> and <close/> in a namespace of
// their own. SASL, resource binding, rosters, presence and every stanza are
// unchanged, which is the whole reason a browser client and Conversations can
// be the same client.
//
// # Authentication
//
// SASL, inside the stream, with an access token. Deliberately not the session
// cookie: a cookie would make this endpoint something any page on the internet
// could open on a signed-in visitor's behalf, and the origin check is the only
// thing standing in the way. A token has to be handed over on purpose, which
// is also what makes the browser and a desktop client the same kind of client
// rather than a privileged one and an ordinary one.

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/app"
)

// wsUpgrader accepts the xmpp subprotocol, which is what a client asks for.
//
// A browser opens `new WebSocket(url, "xmpp")` and will refuse the connection
// if the server does not name the subprotocol back, so this is not decoration.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	Subprotocols:    []string{"xmpp"},
	// Any origin, because nothing here is reachable without a token: there is
	// no cookie to ride and no state that answers before SASL. That is the
	// property that makes it safe, not a judgement about who is calling.
	CheckOrigin: func(*http.Request) bool { return true },
}

// wsCarrier is a WebSocket that reads and writes stanzas.
type wsCarrier struct {
	conn *websocket.Conn
	// rest is what is left of the message being read. A stanza arrives whole
	// but the decoder asks for bytes, so a message is handed over in as many
	// reads as it wants and the next one is not touched until this is spent.
	rest []byte
}

func (w *wsCarrier) Read(p []byte) (int, error) {
	for len(w.rest) == 0 {
		_, msg, err := w.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		w.rest = msg
	}
	n := copy(p, w.rest)
	w.rest = w.rest[n:]
	return n, nil
}

func (w *wsCarrier) writeStanza(s string) error {
	return w.conn.WriteMessage(websocket.TextMessage, []byte(s))
}

func (w *wsCarrier) SetReadDeadline(t time.Time) error  { return w.conn.SetReadDeadline(t) }
func (w *wsCarrier) SetWriteDeadline(t time.Time) error { return w.conn.SetWriteDeadline(t) }
func (w *wsCarrier) Close() error                       { return w.conn.Close() }
func (*wsCarrier) framed() bool                         { return true }

// XMPPWebSocketHandler serves /xmpp-websocket.
//
// The address is the one clients look for: a browser given a domain asks
// /.well-known/host-meta for it, and the convention every server follows is
// this path. See WellKnownHostMeta.
func XMPPWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already answered the request by the time it fails.
		app.Log("chat", "xmpp websocket upgrade refused: %v", err)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			ip = strings.TrimSpace(first)
		} else {
			ip = fwd
		}
	}
	defer func() {
		if rec := recover(); rec != nil {
			app.Log("chat", "xmpp websocket session panicked: %v", rec)
		}
		conn.Close()
	}()
	serve(&wsCarrier{conn: conn}, ip)
}

// WellKnownHostMeta answers /.well-known/host-meta.json, which is how a browser
// client finds the WebSocket endpoint from the domain alone.
//
// The same job the SRV record does for a desktop client, in the one form a
// browser can perform: it cannot look up SRV, so XEP-0156 defines this instead.
// Without it a web client given "you@micro.mu" has nothing to try.
func WellKnownHostMeta(w http.ResponseWriter, r *http.Request) {
	base := "https://" + Domain()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write([]byte(`{"links":[{"rel":"urn:xmpp:alt-connections:websocket",` +
		`"href":"` + base + `/xmpp-websocket"}]}`))
}
