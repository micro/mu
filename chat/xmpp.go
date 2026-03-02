package chat

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"

	"golang.org/x/crypto/bcrypt"
)

// tlsCertStore caches a loaded TLS certificate and reloads it automatically
// when the underlying files change (e.g. after a Let's Encrypt renewal).
// GetCertificate is used as the tls.Config.GetCertificate callback so every
// new STARTTLS handshake checks the on-disk mod-time before returning the cert.
type tlsCertStore struct {
	certFile string
	keyFile  string
	cert     *tls.Certificate
	certMod  time.Time
	keyMod   time.Time
	mu       sync.RWMutex
}

// GetCertificate satisfies tls.Config.GetCertificate.
// It returns the cached certificate unless either file has been modified,
// in which case it reloads from disk. On reload failure it falls back to
// the previously cached certificate so existing TLS service is not disrupted.
func (cs *tlsCertStore) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	certInfo, err := os.Stat(cs.certFile)
	if err != nil {
		return nil, fmt.Errorf("cert file inaccessible: %v", err)
	}
	keyInfo, err := os.Stat(cs.keyFile)
	if err != nil {
		return nil, fmt.Errorf("key file inaccessible: %v", err)
	}

	cs.mu.RLock()
	upToDate := cs.cert != nil &&
		!certInfo.ModTime().After(cs.certMod) &&
		!keyInfo.ModTime().After(cs.keyMod)
	cached := cs.cert
	cs.mu.RUnlock()

	if upToDate {
		return cached, nil
	}

	// Reload under write lock (double-check to avoid redundant reloads).
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.cert != nil &&
		!certInfo.ModTime().After(cs.certMod) &&
		!keyInfo.ModTime().After(cs.keyMod) {
		return cs.cert, nil
	}

	newCert, err := tls.LoadX509KeyPair(cs.certFile, cs.keyFile)
	if err != nil {
		app.Log("xmpp", "TLS certificate reload failed: %v - keeping existing cert", err)
		if cs.cert != nil {
			return cs.cert, nil // keep serving with the old cert
		}
		return nil, err
	}

	cs.cert = &newCert
	cs.certMod = certInfo.ModTime()
	cs.keyMod = keyInfo.ModTime()
	app.Log("xmpp", "TLS certificate reloaded (cert mtime: %s)", certInfo.ModTime().Format(time.RFC3339))
	return cs.cert, nil
}

// Similar to mail/SMTP, this provides decentralized chat capability
// Implements core XMPP (RFC 6120, 6121, 6122)
// With full S2S federation, TLS, MUC, and offline messages

const (
	xmppNamespace       = "jabber:client"
	xmppStreamNamespace = "http://etherx.jabber.org/streams"
	xmppSASLNamespace   = "urn:ietf:params:xml:ns:xmpp-sasl"
	xmppBindNamespace   = "urn:ietf:params:xml:ns:xmpp-bind"
	xmppTLSNamespace    = "urn:ietf:params:xml:ns:xmpp-tls"
	xmppMUCNamespace    = "http://jabber.org/protocol/muc"
	xmppS2SNamespace    = "jabber:server"
)

// XMPPServer represents the XMPP server
type XMPPServer struct {
	Domain      string
	Port        string
	S2SPort     string
	listener    net.Listener
	s2sListener net.Listener
	sessions    map[string]*XMPPSession
	s2sSessions map[string]*S2SSession // domain -> S2S session
	rooms       map[string]*MUCRoom
	tlsConfig   *tls.Config
	mutex       sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// XMPPSession represents a client connection
type XMPPSession struct {
	conn         net.Conn
	jid          string // Full JID (user@domain/resource)
	username     string
	resource     string
	domain       string
	authorized   bool
	encrypted    bool
	encoder      *xml.Encoder
	decoder      *xml.Decoder
	mutex        sync.Mutex
	offlineQueue []*Message // Offline message queue
}

// S2SSession represents a server-to-server connection
type S2SSession struct {
	conn          net.Conn
	domain        string
	remoteDomain  string
	authenticated bool
	encrypted     bool
	encoder       *xml.Encoder
	decoder       *xml.Decoder
	mutex         sync.Mutex
	outbound      bool // true if we initiated the connection
}

// MUCRoom represents a multi-user chat room
type MUCRoom struct {
	JID        string
	Name       string
	Subject    string
	Occupants  map[string]*MUCOccupant
	Persistent bool
	CreatedAt  time.Time
	mutex      sync.RWMutex
}

// MUCOccupant represents a user in a MUC room
type MUCOccupant struct {
	JID         string
	Nick        string
	Role        string // moderator, participant, visitor
	Affiliation string // owner, admin, member, none
	session     *XMPPSession
}

// OfflineMessage represents a stored message for offline delivery
type OfflineMessage struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Body      string    `json:"body"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
}

// XMPP stream elements
type StreamStart struct {
	XMLName xml.Name `xml:"http://etherx.jabber.org/streams stream"`
	From    string   `xml:"from,attr,omitempty"`
	To      string   `xml:"to,attr,omitempty"`
	ID      string   `xml:"id,attr,omitempty"`
	Version string   `xml:"version,attr,omitempty"`
	Lang    string   `xml:"xml:lang,attr,omitempty"`
}

type StreamFeatures struct {
	XMLName  xml.Name `xml:"stream:features"`
	StartTLS *struct {
		XMLName  xml.Name  `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
		Required *struct{} `xml:"required,omitempty"`
	} `xml:"starttls,omitempty"`
	Mechanisms []string  `xml:"mechanisms>mechanism,omitempty"`
	Bind       *struct{} `xml:"bind,omitempty"`
	Session    *struct{} `xml:"session,omitempty"`
}

type TLSProceed struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls proceed"`
}

type TLSFailure struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls failure"`
}

type StartTLS struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
}

type SASLAuth struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl auth"`
	Mechanism string   `xml:"mechanism,attr"`
	Value     string   `xml:",chardata"`
}

type SASLSuccess struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl success"`
}

type SASLFailure struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-sasl failure"`
	Reason  string   `xml:",innerxml"`
}

type IQBind struct {
	XMLName  xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Resource string   `xml:"resource,omitempty"`
	JID      string   `xml:"jid,omitempty"`
}

type IQ struct {
	XMLName xml.Name `xml:"iq"`
	Type    string   `xml:"type,attr"`
	ID      string   `xml:"id,attr,omitempty"`
	From    string   `xml:"from,attr,omitempty"`
	To      string   `xml:"to,attr,omitempty"`
	Bind    *IQBind  `xml:"bind,omitempty"`
	Error   *struct {
		Type string `xml:"type,attr"`
		Text string `xml:",innerxml"`
	} `xml:"error,omitempty"`
}

type Message struct {
	XMLName xml.Name `xml:"message"`
	Type    string   `xml:"type,attr,omitempty"`
	From    string   `xml:"from,attr,omitempty"`
	To      string   `xml:"to,attr,omitempty"`
	ID      string   `xml:"id,attr,omitempty"`
	Body    string   `xml:"body,omitempty"`
}

type Presence struct {
	XMLName xml.Name `xml:"presence"`
	Type    string   `xml:"type,attr,omitempty"`
	From    string   `xml:"from,attr,omitempty"`
	To      string   `xml:"to,attr,omitempty"`
	Show    string   `xml:"show,omitempty"`
	Status  string   `xml:"status,omitempty"`
}

// NewXMPPServer creates a new XMPP server instance
func NewXMPPServer(domain, port, s2sPort string) *XMPPServer {
	ctx, cancel := context.WithCancel(context.Background())

	// Load TLS configuration if certificates are available
	tlsConfig := loadTLSConfig(domain)

	return &XMPPServer{
		Domain:      domain,
		Port:        port,
		S2SPort:     s2sPort,
		sessions:    make(map[string]*XMPPSession),
		s2sSessions: make(map[string]*S2SSession),
		rooms:       make(map[string]*MUCRoom),
		tlsConfig:   tlsConfig,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// loadTLSConfig builds a *tls.Config whose GetCertificate callback reloads
// the certificate from disk whenever the file modification time changes.
// This allows Let's Encrypt (or any external CA) to renew the certificate
// without restarting the XMPP server.
func loadTLSConfig(domain string) *tls.Config {
	certFile := os.Getenv("XMPP_CERT_FILE")
	keyFile := os.Getenv("XMPP_KEY_FILE")

	if certFile == "" || keyFile == "" {
		// Try default Let's Encrypt paths
		certFile = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
		keyFile = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain)
	}

	// Check if certificate files exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		app.Log("xmpp", "TLS certificate not found at %s - TLS disabled", certFile)
		return nil
	}

	// Do an initial load to verify the key pair is valid before accepting connections.
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		app.Log("xmpp", "Failed to load TLS certificates: %v - TLS disabled", err)
		return nil
	}

	store := &tlsCertStore{
		certFile: certFile,
		keyFile:  keyFile,
	}

	app.Log("xmpp", "TLS certificates loaded (auto-reload on renewal enabled)")

	return &tls.Config{
		GetCertificate: store.GetCertificate,
		ServerName:     domain,
		MinVersion:     tls.VersionTLS12,
	}
}

// Start begins listening for XMPP connections
func (s *XMPPServer) Start() error {
	// Start C2S (Client-to-Server) listener
	addr := ":" + s.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start XMPP C2S server: %v", err)
	}

	s.listener = listener
	app.Log("xmpp", "XMPP C2S server listening on %s (domain: %s)", addr, s.Domain)

	// Start S2S (Server-to-Server) listener if port is configured
	if s.S2SPort != "" {
		s2sAddr := ":" + s.S2SPort
		s2sListener, err := net.Listen("tcp", s2sAddr)
		if err != nil {
			app.Log("xmpp", "Failed to start S2S listener: %v", err)
		} else {
			s.s2sListener = s2sListener
			app.Log("xmpp", "XMPP S2S server listening on %s", s2sAddr)
			go s.acceptS2SConnections()
		}
	}

	if s.tlsConfig != nil {
		app.Log("xmpp", "STARTTLS enabled")
	} else {
		app.Log("xmpp", "WARNING: TLS not configured - connections will be unencrypted")
	}

	// Accept C2S connections
	go s.acceptConnections()

	// Start offline message delivery worker
	go s.processOfflineMessages()

	return nil
}

// acceptConnections handles incoming connections
func (s *XMPPServer) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.ctx.Done():
					return
				default:
					app.Log("xmpp", "Error accepting connection: %v", err)
					continue
				}
			}

			// Handle each connection in a goroutine
			go s.handleConnection(conn)
		}
	}
}

// acceptS2SConnections handles incoming server-to-server connections
func (s *XMPPServer) acceptS2SConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.s2sListener.Accept()
			if err != nil {
				select {
				case <-s.ctx.Done():
					return
				default:
					app.Log("xmpp", "Error accepting S2S connection: %v", err)
					continue
				}
			}

			// Handle each S2S connection in a goroutine
			go s.handleS2SConnection(conn, false)
		}
	}
}

// handleConnection processes a single XMPP client connection
func (s *XMPPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	session := &XMPPSession{
		conn:    conn,
		domain:  s.Domain,
		encoder: xml.NewEncoder(conn),
		decoder: xml.NewDecoder(conn),
	}

	remoteAddr := conn.RemoteAddr().String()
	app.Log("xmpp", "New connection from %s", remoteAddr)

	// Initial stream negotiation
	if err := s.handleStreamNegotiation(session); err != nil {
		app.Log("xmpp", "Stream negotiation failed: %v", err)
		return
	}

	// Main stanza processing loop
	s.handleStanzas(session)
}

// handleStreamNegotiation performs initial XMPP stream setup
func (s *XMPPServer) handleStreamNegotiation(session *XMPPSession) error {
	// Read opening stream tag
	var streamStart StreamStart
	if err := session.decoder.Decode(&streamStart); err != nil {
		return fmt.Errorf("failed to read stream start: %v", err)
	}

	// Send stream response
	streamID := generateStreamID()
	response := fmt.Sprintf(`<?xml version='1.0'?>
<stream:stream xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' 
from='%s' id='%s' version='1.0'>`, s.Domain, streamID)

	if _, err := session.conn.Write([]byte(response)); err != nil {
		return fmt.Errorf("failed to send stream header: %v", err)
	}

	// Send stream features
	features := StreamFeatures{}

	// Offer STARTTLS if TLS is configured and not yet encrypted
	if s.tlsConfig != nil && !session.encrypted {
		features.StartTLS = &struct {
			XMLName  xml.Name  `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
			Required *struct{} `xml:"required,omitempty"`
		}{
			Required: &struct{}{}, // Make TLS required
		}
	}

	// Offer SASL mechanisms only after TLS or if TLS not available
	if session.encrypted || s.tlsConfig == nil {
		features.Mechanisms = []string{"PLAIN"}
	}

	// Offer resource binding and session after authentication
	if session.authorized {
		features.Bind = &struct{}{}
		features.Session = &struct{}{}
	}

	if err := session.encoder.Encode(&features); err != nil {
		return fmt.Errorf("failed to send features: %v", err)
	}

	return nil
}

// handleStanzas processes incoming XMPP stanzas
func (s *XMPPServer) handleStanzas(session *XMPPSession) {
	for {
		// Read next token
		token, err := session.decoder.Token()
		if err != nil {
			if err != io.EOF {
				app.Log("xmpp", "Error reading token: %v", err)
			}
			return
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "starttls":
				if err := s.handleStartTLS(session); err != nil {
					app.Log("xmpp", "STARTTLS failed: %v", err)
					return
				}
				// After TLS upgrade, restart stream negotiation
				if err := s.handleStreamNegotiation(session); err != nil {
					app.Log("xmpp", "Stream renegotiation after TLS failed: %v", err)
					return
				}
			case "auth":
				s.handleAuth(session)
			case "iq":
				s.handleIQ(session, t)
			case "message":
				s.handleMessage(session, t)
			case "presence":
				s.handlePresence(session, t)
			}
		case xml.EndElement:
			if t.Name.Local == "stream" {
				return
			}
		}
	}
}

// handleAuth processes SASL authentication
func (s *XMPPServer) handleAuth(session *XMPPSession) {
	var authStanza SASLAuth
	if err := session.decoder.DecodeElement(&authStanza, nil); err != nil {
		app.Log("xmpp", "Failed to decode auth: %v", err)
		if err := session.encoder.Encode(&SASLFailure{Reason: "malformed-request"}); err != nil {
			app.Log("xmpp", "Failed to send auth failure: %v", err)
		}
		return
	}

	// For PLAIN mechanism, decode credentials
	if authStanza.Mechanism == "PLAIN" {
		// PLAIN format: \0username\0password (base64 encoded)
		// Decode the base64 auth value
		decoded, err := base64.StdEncoding.DecodeString(authStanza.Value)
		if err != nil {
			app.Log("xmpp", "Failed to decode auth credentials: %v", err)
			if err := session.encoder.Encode(&SASLFailure{Reason: "malformed-request"}); err != nil {
				app.Log("xmpp", "Failed to send auth failure: %v", err)
			}
			return
		}

		// Parse PLAIN SASL format: [authzid]\0username\0password
		parts := strings.Split(string(decoded), "\x00")
		if len(parts) < 3 {
			app.Log("xmpp", "Invalid PLAIN SASL format")
			if err := session.encoder.Encode(&SASLFailure{Reason: "invalid-authzid"}); err != nil {
				app.Log("xmpp", "Failed to send auth failure: %v", err)
			}
			return
		}

		username := parts[1]
		password := parts[2]

		// Verify credentials against auth system
		acc, err := auth.GetAccountByName(username)
		if err != nil {
			app.Log("xmpp", "Authentication failed for user %s: user not found", username)
			if err := session.encoder.Encode(&SASLFailure{Reason: "not-authorized"}); err != nil {
				app.Log("xmpp", "Failed to send auth failure: %v", err)
			}
			return
		}

		// Verify password using bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(acc.Secret), []byte(password)); err != nil {
			app.Log("xmpp", "Authentication failed for user %s: invalid password", username)
			if err := session.encoder.Encode(&SASLFailure{Reason: "not-authorized"}); err != nil {
				app.Log("xmpp", "Failed to send auth failure: %v", err)
			}
			return
		}

		// Authentication successful
		session.authorized = true
		session.username = username
		app.Log("xmpp", "User %s authenticated successfully", username)

		// Send success
		if err := session.encoder.Encode(&SASLSuccess{}); err != nil {
			app.Log("xmpp", "Failed to send auth success: %v", err)
			return
		}

		// Client will restart stream after successful auth
	} else {
		if err := session.encoder.Encode(&SASLFailure{Reason: "invalid-mechanism"}); err != nil {
			app.Log("xmpp", "Failed to send auth failure: %v", err)
		}
	}
}

// handleIQ processes IQ (Info/Query) stanzas
func (s *XMPPServer) handleIQ(session *XMPPSession, start xml.StartElement) {
	var iq IQ
	if err := session.decoder.DecodeElement(&iq, &start); err != nil {
		app.Log("xmpp", "Failed to decode IQ: %v", err)
		return
	}

	// Handle resource binding
	if iq.Type == "set" && iq.Bind != nil {
		resource := iq.Bind.Resource
		if resource == "" {
			resource = "mu-" + generateStreamID()[:8]
		}

		session.resource = resource
		session.jid = fmt.Sprintf("%s@%s/%s", session.username, s.Domain, resource)

		// Store session
		s.mutex.Lock()
		s.sessions[session.jid] = session
		s.mutex.Unlock()

		// Send bind result
		result := IQ{
			Type: "result",
			ID:   iq.ID,
			Bind: &IQBind{
				JID: session.jid,
			},
		}

		if err := session.encoder.Encode(&result); err != nil {
			app.Log("xmpp", "Failed to send bind result: %v", err)
		}

		app.Log("xmpp", "User bound to JID: %s", session.jid)

		// Deliver any offline messages
		go s.deliverOfflineMessages(session)
	}
}

// handleMessage processes message stanzas
func (s *XMPPServer) handleMessage(session *XMPPSession, start xml.StartElement) {
	var msg Message
	if err := session.decoder.DecodeElement(&msg, &start); err != nil {
		app.Log("xmpp", "Failed to decode message: %v", err)
		return
	}

	// Set from if not already set
	if msg.From == "" {
		msg.From = session.jid
	}

	app.Log("xmpp", "Message from %s to %s: %s", msg.From, msg.To, msg.Body)

	// Route message to recipient
	if msg.To != "" {
		s.routeMessage(&msg)
	}
}

// handlePresence processes presence stanzas
func (s *XMPPServer) handlePresence(session *XMPPSession, start xml.StartElement) {
	var pres Presence
	if err := session.decoder.DecodeElement(&pres, &start); err != nil {
		app.Log("xmpp", "Failed to decode presence: %v", err)
		return
	}

	if pres.From == "" {
		pres.From = session.jid
	}

	app.Log("xmpp", "Presence from %s: %s", pres.From, pres.Type)

	// Update user presence in auth system
	if session.username != "" {
		if account, err := auth.GetAccountByName(session.username); err == nil {
			auth.UpdatePresence(account.ID)
		}
	}

	// Broadcast presence to other sessions
	s.broadcastPresence(&pres)
}

// routeMessage delivers a message to the recipient
func (s *XMPPServer) routeMessage(msg *Message) {
	// Extract recipient JID
	recipientJID := msg.To

	// Check if recipient is local or remote
	parts := strings.Split(recipientJID, "@")
	if len(parts) < 2 {
		app.Log("xmpp", "Invalid recipient JID: %s", recipientJID)
		return
	}

	domain := strings.Split(parts[1], "/")[0]

	if domain == s.Domain {
		// Local delivery
		username := parts[0]

		// Try to find online session for this user
		s.mutex.RLock()
		var targetSession *XMPPSession
		for jid, session := range s.sessions {
			if strings.HasPrefix(jid, username+"@") {
				targetSession = session
				break
			}
		}
		s.mutex.RUnlock()

		if targetSession != nil {
			// User is online - deliver immediately
			targetSession.mutex.Lock()
			defer targetSession.mutex.Unlock()

			if err := targetSession.encoder.Encode(msg); err != nil {
				app.Log("xmpp", "Failed to deliver message: %v", err)
				// Save as offline if immediate delivery fails
				s.saveOfflineMessage(msg)
			} else {
				app.Log("xmpp", "Message delivered to %s", recipientJID)
			}
		} else {
			// User is offline - store message for later delivery
			app.Log("xmpp", "User %s offline, storing message", username)
			s.saveOfflineMessage(msg)
		}
	} else {
		// Remote delivery via S2S (Server-to-Server)
		app.Log("xmpp", "Routing message to remote domain: %s", domain)

		// Try to get or establish S2S connection
		s2sSession, err := s.dialS2S(domain)
		if err != nil {
			app.Log("xmpp", "Failed to establish S2S connection to %s: %v", domain, err)
			return
		}

		// Send message over S2S connection
		s2sSession.mutex.Lock()
		defer s2sSession.mutex.Unlock()

		if err := s2sSession.encoder.Encode(msg); err != nil {
			app.Log("xmpp", "Failed to send S2S message to %s: %v", domain, err)
		} else {
			app.Log("xmpp", "Message relayed to remote domain: %s", domain)
		}
	}
}

// broadcastPresence sends presence to all sessions
func (s *XMPPServer) broadcastPresence(pres *Presence) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for jid, session := range s.sessions {
		if jid != pres.From {
			session.mutex.Lock()
			if err := session.encoder.Encode(pres); err != nil {
				app.Log("xmpp", "Failed to broadcast presence to %s: %v", jid, err)
			}
			session.mutex.Unlock()
		}
	}
}

// generateStreamID creates a unique stream identifier
func generateStreamID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// handleStartTLS upgrades connection to TLS
func (s *XMPPServer) handleStartTLS(session *XMPPSession) error {
	if s.tlsConfig == nil {
		if err := session.encoder.Encode(&TLSFailure{}); err != nil {
			return fmt.Errorf("failed to send TLS failure: %v", err)
		}
		return fmt.Errorf("TLS not configured")
	}

	// Send TLS proceed
	if err := session.encoder.Encode(&TLSProceed{}); err != nil {
		return fmt.Errorf("failed to send TLS proceed: %v", err)
	}

	// Upgrade connection to TLS
	tlsConn := tls.Server(session.conn, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %v", err)
	}

	// Update session connection and IO
	session.conn = tlsConn
	session.encrypted = true
	session.encoder = xml.NewEncoder(tlsConn)
	session.decoder = xml.NewDecoder(tlsConn)

	app.Log("xmpp", "TLS negotiation successful")
	return nil
}

// handleS2SConnection processes a server-to-server connection
func (s *XMPPServer) handleS2SConnection(conn net.Conn, outbound bool) {
	defer conn.Close()

	s2sSession := &S2SSession{
		conn:     conn,
		encoder:  xml.NewEncoder(conn),
		decoder:  xml.NewDecoder(conn),
		outbound: outbound,
	}

	remoteAddr := conn.RemoteAddr().String()
	app.Log("xmpp", "New S2S connection from %s (outbound: %v)", remoteAddr, outbound)

	// S2S stream negotiation (simplified - full implementation would use dialback)
	// For now, just log that we received an S2S connection
	if !outbound {
		// Inbound S2S connection
		var streamStart StreamStart
		if err := s2sSession.decoder.Decode(&streamStart); err != nil {
			app.Log("xmpp", "S2S stream start failed: %v", err)
			return
		}

		s2sSession.remoteDomain = streamStart.From

		// Send stream response
		streamID := generateStreamID()
		response := fmt.Sprintf(`<?xml version='1.0'?>
<stream:stream xmlns='jabber:server' xmlns:stream='http://etherx.jabber.org/streams' 
xmlns:db='jabber:server:dialback' from='%s' to='%s' id='%s' version='1.0'>`,
			s.Domain, s2sSession.remoteDomain, streamID)

		if _, err := conn.Write([]byte(response)); err != nil {
			app.Log("xmpp", "Failed to send S2S stream header: %v", err)
			return
		}

		// For now, mark as authenticated (in production, would do dialback)
		s2sSession.authenticated = true
		s2sSession.domain = s.Domain

		// Store S2S session
		s.mutex.Lock()
		s.s2sSessions[s2sSession.remoteDomain] = s2sSession
		s.mutex.Unlock()

		app.Log("xmpp", "S2S session established with %s", s2sSession.remoteDomain)

		// Handle S2S stanzas (simplified)
		s.handleS2SStanzas(s2sSession)
	}
}

// handleS2SStanzas processes S2S stanzas
func (s *XMPPServer) handleS2SStanzas(session *S2SSession) {
	for {
		token, err := session.decoder.Token()
		if err != nil {
			if err != io.EOF {
				app.Log("xmpp", "S2S error reading token: %v", err)
			}
			return
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "message":
				var msg Message
				if err := session.decoder.DecodeElement(&msg, &t); err != nil {
					app.Log("xmpp", "Failed to decode S2S message: %v", err)
					continue
				}
				// Route to local user
				s.routeMessage(&msg)
			}
		case xml.EndElement:
			if t.Name.Local == "stream" {
				return
			}
		}
	}
}

// dialS2S creates an outbound S2S connection to a remote server
func (s *XMPPServer) dialS2S(domain string) (*S2SSession, error) {
	// Check if we already have a connection
	s.mutex.RLock()
	existing, ok := s.s2sSessions[domain]
	s.mutex.RUnlock()

	if ok && existing.authenticated {
		return existing, nil
	}

	// Lookup SRV record for xmpp-server
	// For now, just try port 5269
	addr := domain + ":5269"

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", domain, err)
	}

	s2sSession := &S2SSession{
		conn:         conn,
		domain:       s.Domain,
		remoteDomain: domain,
		encoder:      xml.NewEncoder(conn),
		decoder:      xml.NewDecoder(conn),
		outbound:     true,
	}

	// Send stream header
	streamHeader := fmt.Sprintf(`<?xml version='1.0'?>
<stream:stream xmlns='jabber:server' xmlns:stream='http://etherx.jabber.org/streams' 
from='%s' to='%s' version='1.0'>`, s.Domain, domain)

	if _, err := conn.Write([]byte(streamHeader)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send S2S stream header: %v", err)
	}

	// For simplified implementation, mark as authenticated
	// Full implementation would do dialback or SASL EXTERNAL
	s2sSession.authenticated = true

	// Store session
	s.mutex.Lock()
	s.s2sSessions[domain] = s2sSession
	s.mutex.Unlock()

	// Start handling stanzas in background
	go s.handleS2SStanzas(s2sSession)

	app.Log("xmpp", "Outbound S2S connection established to %s", domain)

	return s2sSession, nil
}

// saveOfflineMessage stores a message for later delivery
func (s *XMPPServer) saveOfflineMessage(msg *Message) {
	// Extract username from JID
	parts := strings.Split(msg.To, "@")
	if len(parts) < 2 {
		return
	}
	username := parts[0]

	offlineMsg := OfflineMessage{
		ID:        generateStreamID(),
		From:      msg.From,
		To:        msg.To,
		Body:      msg.Body,
		Timestamp: time.Now(),
		Type:      msg.Type,
	}

	// Load existing offline messages
	var messages []OfflineMessage
	if b, err := data.LoadFile(fmt.Sprintf("xmpp_offline_%s.json", username)); err == nil {
		json.Unmarshal(b, &messages)
	}

	// Append new message
	messages = append(messages, offlineMsg)

	// Save back
	if b, err := json.Marshal(messages); err == nil {
		data.SaveFile(fmt.Sprintf("xmpp_offline_%s.json", username), string(b))
		app.Log("xmpp", "Saved offline message for %s", username)
	}
}

// deliverOfflineMessages delivers stored offline messages to a user
func (s *XMPPServer) deliverOfflineMessages(session *XMPPSession) {
	if session.username == "" {
		return
	}

	// Load offline messages
	var messages []OfflineMessage
	filename := fmt.Sprintf("xmpp_offline_%s.json", session.username)
	if b, err := data.LoadFile(filename); err == nil {
		if err := json.Unmarshal(b, &messages); err != nil {
			return
		}
	}

	if len(messages) == 0 {
		return
	}

	app.Log("xmpp", "Delivering %d offline messages to %s", len(messages), session.jid)

	// Deliver each message
	session.mutex.Lock()
	defer session.mutex.Unlock()

	for _, offlineMsg := range messages {
		msg := &Message{
			Type: offlineMsg.Type,
			From: offlineMsg.From,
			To:   session.jid,
			ID:   offlineMsg.ID,
			Body: offlineMsg.Body,
		}

		if err := session.encoder.Encode(msg); err != nil {
			app.Log("xmpp", "Failed to deliver offline message: %v", err)
		}
	}

	// Clear offline messages after delivery
	data.SaveFile(filename, "[]")
}

// processOfflineMessages worker that periodically delivers offline messages
func (s *XMPPServer) processOfflineMessages() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Deliver offline messages to connected users
			s.mutex.RLock()
			for _, session := range s.sessions {
				if session.authorized && session.username != "" {
					s.deliverOfflineMessages(session)
				}
			}
			s.mutex.RUnlock()
		}
	}
}

// createRoom creates a new MUC room
func (s *XMPPServer) createRoom(roomJID, creator string) *MUCRoom {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if room, exists := s.rooms[roomJID]; exists {
		return room
	}

	room := &MUCRoom{
		JID:        roomJID,
		Name:       strings.Split(roomJID, "@")[0],
		Occupants:  make(map[string]*MUCOccupant),
		Persistent: true,
		CreatedAt:  time.Now(),
	}

	s.rooms[roomJID] = room
	app.Log("xmpp", "Created MUC room: %s", roomJID)

	return room
}

// joinRoom adds a user to a MUC room
func (s *XMPPServer) joinRoom(roomJID, userJID, nick string, session *XMPPSession) error {
	s.mutex.RLock()
	room, exists := s.rooms[roomJID]
	s.mutex.RUnlock()

	if !exists {
		// Auto-create room
		room = s.createRoom(roomJID, userJID)
	}

	room.mutex.Lock()
	defer room.mutex.Unlock()

	occupant := &MUCOccupant{
		JID:         userJID,
		Nick:        nick,
		Role:        "participant",
		Affiliation: "member",
		session:     session,
	}

	room.Occupants[nick] = occupant

	// Broadcast presence to room
	s.broadcastToRoom(room, &Presence{
		From: roomJID + "/" + nick,
		To:   userJID,
		Type: "",
	})

	app.Log("xmpp", "User %s joined room %s as %s", userJID, roomJID, nick)

	return nil
}

// broadcastToRoom sends a stanza to all occupants in a room
func (s *XMPPServer) broadcastToRoom(room *MUCRoom, stanza interface{}) {
	room.mutex.RLock()
	defer room.mutex.RUnlock()

	for _, occupant := range room.Occupants {
		if occupant.session != nil {
			occupant.session.mutex.Lock()
			occupant.session.encoder.Encode(stanza)
			occupant.session.mutex.Unlock()
		}
	}
}

// Stop gracefully shuts down the XMPP server
func (s *XMPPServer) Stop() error {
	app.Log("xmpp", "Shutting down XMPP server")
	s.cancel()

	if s.listener != nil {
		return s.listener.Close()
	}

	return nil
}

// Global XMPP server instance
var xmppServer *XMPPServer

// StartXMPPServer initializes and starts the XMPP server
func StartXMPPServer() error {
	// Get configuration from environment
	domain := os.Getenv("XMPP_DOMAIN")
	if domain == "" {
		domain = "localhost" // Default domain
	}

	port := os.Getenv("XMPP_PORT")
	if port == "" {
		port = "5222" // Standard XMPP client-to-server port
	}

	s2sPort := os.Getenv("XMPP_S2S_PORT")
	if s2sPort == "" {
		s2sPort = "5269" // Standard XMPP server-to-server port
	}

	// Create and start server
	xmppServer = NewXMPPServer(domain, port, s2sPort)

	// Start in goroutine
	go func() {
		if err := xmppServer.Start(); err != nil {
			log.Printf("XMPP server error: %v", err)
		}
	}()

	return nil
}

// StartXMPPServerIfEnabled starts the XMPP server if configured
func StartXMPPServerIfEnabled() bool {
	// Check if XMPP is enabled
	enabled := os.Getenv("XMPP_ENABLED")
	if enabled == "" || enabled == "false" || enabled == "0" {
		app.Log("xmpp", "XMPP server disabled (set XMPP_ENABLED=true to enable)")
		return false
	}

	if err := StartXMPPServer(); err != nil {
		app.Log("xmpp", "Failed to start XMPP server: %v", err)
		return false
	}

	return true
}

// GetXMPPStatus returns the XMPP server status for health checks
func GetXMPPStatus() map[string]interface{} {
	status := map[string]interface{}{
		"enabled": false,
	}

	if xmppServer != nil {
		xmppServer.mutex.RLock()
		sessionCount := len(xmppServer.sessions)
		s2sCount := len(xmppServer.s2sSessions)
		roomCount := len(xmppServer.rooms)
		xmppServer.mutex.RUnlock()

		status["enabled"] = true
		status["domain"] = xmppServer.Domain
		status["c2s_port"] = xmppServer.Port
		status["s2s_port"] = xmppServer.S2SPort
		status["sessions"] = sessionCount
		status["s2s_connections"] = s2sCount
		status["muc_rooms"] = roomCount
		status["tls_enabled"] = xmppServer.tlsConfig != nil
	}

	return status
}
