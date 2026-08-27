package chat

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// A browser can do the whole handshake over a WebSocket.
//
// Driven with the bytes a client actually sends, over a real socket, for the
// same reason the TCP test is: a server that compiles is not a server. What is
// being checked here is not that XMPP works — the other test covers that — but
// that it works *unchanged* on the other carrier, because that is the entire
// claim. If the browser needed one different stanza it would be a second
// protocol again.
func TestABrowserSpeaksTheSameXMPP(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "wsuser")

	c, srv := dialWS(t)
	defer srv.Close()
	defer c.Close()

	// RFC 7395: <open/> rather than <stream:stream>, and it is self-closing
	// because the message boundary already says where a stanza ends.
	c.say(t, `<open xmlns='urn:ietf:params:xml:ns:xmpp-framing' to='example.test' version='1.0'/>`)
	if got := c.hear(t); !strings.Contains(got, "<open ") {
		t.Fatalf("no <open/> back, so the framing is wrong: %q", clip(got))
	} else if strings.Contains(got, "<stream:stream") {
		t.Error("the socket's stream header was sent to a WebSocket client, " +
			"which is the one thing RFC 7395 replaces")
	}
	if got := c.hear(t); !strings.Contains(got, "PLAIN") {
		t.Fatalf("SASL was not offered: %q", clip(got))
	}

	// From here it is ordinary XMPP, which is the point.
	c.say(t, `<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>`+
		base64.StdEncoding.EncodeToString([]byte("\x00"+acc.ID+"\x00"+token))+`</auth>`)
	if got := c.hear(t); !strings.Contains(got, "<success") {
		t.Fatalf("sign-in refused with a valid token: %q", clip(got))
	}

	c.say(t, `<open xmlns='urn:ietf:params:xml:ns:xmpp-framing' to='example.test' version='1.0'/>`)
	c.hear(t) // the second <open/>
	if got := c.hear(t); !strings.Contains(got, "xmpp-bind") {
		t.Fatalf("bind was not offered after authentication: %q", clip(got))
	}

	c.say(t, `<iq type='set' id='b1'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'>`+
		`<resource>browser</resource></bind></iq>`)
	if got := c.hear(t); !strings.Contains(got, "wsuser@example.test/browser") {
		t.Fatalf("bind did not return the full JID: %q", clip(got))
	}
	if !Online("wsuser") {
		t.Error("a browser that has bound does not count as online")
	}
}

// One message is one stanza, which is the framing rule the whole subprotocol is.
//
// A server that wrote a stanza in two calls would send two messages, and a
// client parsing each message as a complete stanza would see two broken halves.
func TestOneMessageIsOneStanza(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "wsframing")

	c, srv := dialWS(t)
	defer srv.Close()
	defer c.Close()
	c.handshake(t, acc.ID, token)

	// A roster is the longest thing this server writes, built up in a strings
	// Builder — exactly the shape that would be written out piecemeal.
	c.say(t, `<iq type='get' id='r1'><query xmlns='jabber:iq:roster'/></iq>`)
	got := c.hear(t)
	if !strings.HasPrefix(strings.TrimSpace(got), "<iq") || !strings.HasSuffix(strings.TrimSpace(got), "</iq>") {
		t.Errorf("a message is not one whole stanza: %q", clip(got))
	}
}

// A browser given a domain can find the endpoint.
//
// It cannot look up the SRV record a desktop client uses, so XEP-0156 defines
// this instead. Without it a web client has the address and nowhere to send it.
func TestAWebClientCanDiscoverTheEndpoint(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	w := httptest.NewRecorder()
	WellKnownHostMeta(w, httptest.NewRequest(http.MethodGet, "/.well-known/host-meta.json", nil))

	body := w.Body.String()
	if !strings.Contains(body, "urn:xmpp:alt-connections:websocket") {
		t.Errorf("host-meta does not advertise a websocket connection: %s", body)
	}
	if !strings.Contains(body, "example.test/xmpp-websocket") {
		t.Errorf("host-meta does not point at this instance: %s", body)
	}
}

// ── harness ────────────────────────────────────────────────────────────

type wsClient struct{ *websocket.Conn }

func dialWS(t *testing.T) (*wsClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(XMPPWebSocketHandler))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	// The subprotocol, because a browser sends it and refuses a server that
	// does not name it back.
	d := websocket.Dialer{Subprotocols: []string{"xmpp"}}
	c, resp, err := d.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "xmpp" {
		t.Errorf("server agreed to subprotocol %q, want xmpp — a browser refuses "+
			"a connection that does not name it back", got)
	}
	return &wsClient{c}, srv
}

func (c *wsClient) say(t *testing.T, s string) {
	t.Helper()
	if err := c.WriteMessage(websocket.TextMessage, []byte(s)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func (c *wsClient) hear(t *testing.T) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, b, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

func (c *wsClient) handshake(t *testing.T, id, token string) {
	t.Helper()
	open := `<open xmlns='urn:ietf:params:xml:ns:xmpp-framing' to='example.test' version='1.0'/>`
	c.say(t, open)
	c.hear(t)
	c.hear(t)
	c.say(t, `<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>`+
		base64.StdEncoding.EncodeToString([]byte("\x00"+id+"\x00"+token))+`</auth>`)
	c.hear(t)
	c.say(t, open)
	c.hear(t)
	c.hear(t)
	c.say(t, `<iq type='set' id='b'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'/></iq>`)
	c.hear(t)
}
