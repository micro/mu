package chat

import (
	"context"
	"encoding/base64"
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

	"golang.org/x/crypto/bcrypt"
)

// XMPP server implementation for chat federation
// Similar to mail/SMTP, this provides decentralized chat capability
// Implements core XMPP (RFC 6120, 6121, 6122)

const (
	xmppNamespace       = "jabber:client"
	xmppStreamNamespace = "http://etherx.jabber.org/streams"
	xmppSASLNamespace   = "urn:ietf:params:xml:ns:xmpp-sasl"
	xmppBindNamespace   = "urn:ietf:params:xml:ns:xmpp-bind"
)

// XMPPServer represents the XMPP server
type XMPPServer struct {
	Domain   string
	Port     string
	listener net.Listener
	sessions map[string]*XMPPSession
	mutex    sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// XMPPSession represents a client connection
type XMPPSession struct {
	conn       net.Conn
	jid        string // Full JID (user@domain/resource)
	username   string
	resource   string
	domain     string
	authorized bool
	encoder    *xml.Encoder
	decoder    *xml.Decoder
	mutex      sync.Mutex
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
	XMLName    xml.Name `xml:"stream:features"`
	Mechanisms []string `xml:"mechanisms>mechanism,omitempty"`
	Bind       *struct{} `xml:"bind,omitempty"`
	Session    *struct{} `xml:"session,omitempty"`
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
func NewXMPPServer(domain, port string) *XMPPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &XMPPServer{
		Domain:   domain,
		Port:     port,
		sessions: make(map[string]*XMPPSession),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins listening for XMPP connections
func (s *XMPPServer) Start() error {
	addr := ":" + s.Port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start XMPP server: %v", err)
	}

	s.listener = listener
	app.Log("xmpp", "XMPP server listening on %s (domain: %s)", addr, s.Domain)

	// Accept connections
	go s.acceptConnections()

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
	features := StreamFeatures{
		Mechanisms: []string{"PLAIN"},
		Bind:       &struct{}{},
		Session:    &struct{}{},
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
		if acc, err := auth.GetAccount(session.username); err == nil {
			auth.UpdatePresence(acc.ID)
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
		s.mutex.RLock()
		session, exists := s.sessions[recipientJID]
		s.mutex.RUnlock()

		if exists {
			session.mutex.Lock()
			defer session.mutex.Unlock()

			if err := session.encoder.Encode(msg); err != nil {
				app.Log("xmpp", "Failed to deliver message: %v", err)
			} else {
				app.Log("xmpp", "Message delivered to %s", recipientJID)
			}
		} else {
			// Store offline message (would integrate with mail system)
			app.Log("xmpp", "User %s offline, message would be stored", recipientJID)
		}
	} else {
		// Remote delivery via S2S (Server-to-Server)
		// For now, log that we'd relay it
		app.Log("xmpp", "Would relay message to remote domain: %s", domain)
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
	return fmt.Sprintf("%d", time.Now().UnixNano())
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

	// Create and start server
	xmppServer = NewXMPPServer(domain, port)

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
		xmppServer.mutex.RUnlock()

		status["enabled"] = true
		status["domain"] = xmppServer.Domain
		status["port"] = xmppServer.Port
		status["sessions"] = sessionCount
	}

	return status
}
