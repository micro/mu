package shell

import (
	"context"
	_ "embed"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/internal/auth"
	"mu/internal/container"
	"mu/internal/quota"
)

// Pinned xterm.js 6.0.0 and addon-fit 0.11.0; MIT licenses accompany the bundle.
//
//go:embed terminal.js
var terminalJS []byte

func TerminalScriptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(terminalJS)
}

func terminalOrigin(r *http.Request) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && strings.EqualFold(u.Host, r.Host)
}

var webTerminals = struct {
	sync.Mutex
	accounts map[string]bool
}{accounts: make(map[string]bool)}

func claimTerminal(id string) bool {
	webTerminals.Lock()
	defer webTerminals.Unlock()
	if webTerminals.accounts[id] || len(webTerminals.accounts) >= machineBudget() {
		return false
	}
	webTerminals.accounts[id] = true
	return true
}
func releaseTerminal(id string) {
	webTerminals.Lock()
	delete(webTerminals.accounts, id)
	webTerminals.Unlock()
}

type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	CSRF string `json:"csrf"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

func (m terminalMessage) validSize() bool {
	return m.Rows > 0 && m.Rows <= 500 && m.Cols > 0 && m.Cols <= 1000
}

type terminalOutput struct {
	mu      sync.Mutex
	conn    *websocket.Conn
	cancel  context.CancelFunc
	account string
}

func (o *terminalOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := o.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		o.cancel()
		return 0, err
	}
	touched(machineFor(o.account))
	return len(p), nil
}
func (o *terminalOutput) status(message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = o.conn.WriteJSON(map[string]string{"type": "status", "message": message})
}

// The browser opens the same per-account PTY as SSH. Authentication precedes
// upgrade; CSRF precedes container creation, credentials and quota reservation.
func terminalHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Sign in to open a terminal", http.StatusUnauthorized)
		return
	}
	// Ignore header tokens: this interactive door requires a full browser session.
	r = r.Clone(r.Context())
	r.Header.Del("Authorization")
	r.Header.Del("X-Micro-Token")
	sess, err := auth.ParseToken(cookie.Value)
	if err != nil || sess.Type != "account" {
		http.Error(w, "Sign in to open a terminal", http.StatusUnauthorized)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc.Banned || !auth.Trusted(acc.ID) {
		http.Error(w, "Verify your account to open a terminal", http.StatusForbidden)
		return
	}
	if !terminalOrigin(r) {
		http.Error(w, "Invalid origin", http.StatusForbidden)
		return
	}
	if !Configured() || shared() {
		http.Error(w, "Terminal unavailable", http.StatusServiceUnavailable)
		return
	}
	if !claimTerminal(acc.ID) {
		http.Error(w, "A terminal is already open or all machines are busy", http.StatusConflict)
		return
	}
	defer releaseTerminal(acc.ID)
	upgrader := websocket.Upgrader{CheckOrigin: terminalOrigin, HandshakeTimeout: 10 * time.Second}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(64 * 1024)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var first terminalMessage
	if conn.ReadJSON(&first) != nil || first.Type != "open" || !first.validSize() {
		return
	}
	if !terminalCSRF(r, first.CSRF) {
		return
	}
	conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(75 * time.Second)) })
	ctx, cancel := context.WithTimeout(r.Context(), sessionLimit)
	defer cancel()
	go func() {
		tick := time.NewTicker(25 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case <-tick.C:
				if !terminalCSRF(r, first.CSRF) || conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
					cancel()
					return
				}
			}
		}
	}()
	output := &terminalOutput{conn: conn, cancel: cancel, account: acc.ID}
	input, keys := io.Pipe()
	defer input.Close()
	defer keys.Close()
	var sh *container.Session
	var token, tokenID string
	err = quota.Run(acc.ID, quota.OpShellRun, func() error {
		boot, done := context.WithTimeout(ctx, time.Minute)
		err := ready(boot, acc.ID)
		done()
		if err != nil {
			return err
		}
		token, tokenID = sessionToken(acc.ID)
		env := sessionEnv(token)
		if env == nil {
			env = make(map[string]string)
		}
		env["TERM"] = "xterm-256color"
		sh, err = container.Shell(ctx, container.Run{Name: machineFor(acc.ID), Dir: home(acc.ID), User: runAs(acc.ID), Env: env}, input, output)
		return err
	})
	defer revoke(acc.ID, tokenID)
	if err != nil {
		output.status(err.Error())
		return
	}
	sh.Resize(first.Rows, first.Cols)
	output.status("Connected")
	// Timeout/disconnect releases both pipes and the websocket, unblocking readers.
	go func() { <-ctx.Done(); conn.Close(); keys.Close() }()
	go func() {
		defer cancel()
		for {
			var m terminalMessage
			if conn.ReadJSON(&m) != nil {
				return
			}
			switch m.Type {
			case "input":
				touched(machineFor(acc.ID))
				if _, err := keys.Write([]byte(m.Data)); err != nil {
					return
				}
			case "resize":
				if !m.validSize() {
					return
				}
				sh.Resize(m.Rows, m.Cols)
			default:
				return
			}
		}
	}()
	_ = sh.Wait()
	output.status("Disconnected")
}

func terminalCSRF(r *http.Request, token string) bool {
	sess, acc, err := auth.RequireSession(r)
	if token == "" || err != nil || sess.Type != "account" || acc.Banned || !auth.Trusted(acc.ID) {
		return false
	}
	r = r.Clone(r.Context())
	r.Header.Set("X-CSRF-Token", token)
	return auth.StrictCSRF(r)
}
