package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type session struct {
	Server  string    `json:"server"`
	Token   string    `json:"token"`
	Client  string    `json:"client,omitempty"`
	Refresh string    `json:"refresh,omitempty"`
	Expires time.Time `json:"expires,omitempty"`
}

func sessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "mu-cli", "session.json"), err
}
func readSession(path string) (session, error) {
	var s session
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}
func saveSession(path string, s session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// All writers use this lock because refresh tokens rotate on the server.
// Never steal a lock from another process: a crash requires explicit recovery.
func lockSession(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		f, err := os.OpenFile(path+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			f.Close()
			return func() { os.Remove(path + ".lock") }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("session is locked; if no mu-cli process is running, remove %s.lock", path)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func credentials(ctx context.Context, path, server string, h *http.Client) (string, error) {
	unlock, err := lockSession(ctx, path)
	if err != nil {
		return "", err
	}
	defer unlock()
	s, err := readSession(path)
	if err != nil {
		return "", fmt.Errorf("run mu-cli login or set MU_TOKEN")
	}
	if s.Server != server {
		return "", fmt.Errorf("saved sign-in belongs to another server; sign in to this server or set MU_TOKEN")
	}
	if s.Token != "" && (s.Expires.IsZero() || time.Until(s.Expires) > time.Minute) {
		return s.Token, nil
	}
	if s.Refresh == "" || s.Client == "" {
		return "", fmt.Errorf("session expired; run mu-cli login")
	}
	next, err := exchange(ctx, h, server, url.Values{"grant_type": {"refresh_token"}, "client_id": {s.Client}, "refresh_token": {s.Refresh}})
	if err != nil {
		return "", fmt.Errorf("session renewal failed; sign in again: %w", err)
	}
	if next.Refresh == "" {
		return "", fmt.Errorf("runtime returned no rotated refresh token")
	}
	next.Client = s.Client
	if err = saveSession(path, next); err != nil {
		return "", err
	}
	return next.Token, nil
}
func oauthRequest(ctx context.Context, h *http.Client, endpoint, kind string, b []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", kind)
	res, err := h.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return fmt.Errorf("sign-in response too large")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("sign-in HTTP %d", res.StatusCode)
	}
	return json.Unmarshal(data, out)
}
func exchange(ctx context.Context, h *http.Client, server string, form url.Values) (session, error) {
	var v struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Type    string `json:"token_type"`
		Expires int64  `json:"expires_in"`
	}
	err := oauthRequest(ctx, h, server+"/oauth/token", "application/x-www-form-urlencoded", []byte(form.Encode()), &v)
	if err != nil {
		return session{}, err
	}
	if v.Type != "Bearer" || v.Access == "" || len(v.Access) > 16384 || v.Expires <= 0 || v.Expires > 86400 {
		return session{}, fmt.Errorf("invalid sign-in response")
	}
	return session{Server: server, Token: v.Access, Refresh: v.Refresh, Expires: time.Now().Add(time.Duration(v.Expires) * time.Second)}, nil
}
func openBrowser(address string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", address)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		cmd = exec.Command("xdg-open", address)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func browserLogin(ctx context.Context, h *http.Client, server string, output io.Writer, open func(string) error) (session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return session{}, err
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	payload, _ := json.Marshal(map[string]any{"client_name": "Mu CLI", "redirect_uris": []string{redirect}})
	var registered struct {
		ID string `json:"client_id"`
	}
	if err = oauthRequest(ctx, h, server+"/oauth/register", "application/json", payload, &registered); err != nil {
		return session{}, err
	}
	if registered.ID == "" {
		return session{}, fmt.Errorf("runtime did not register client")
	}
	state, verifier := randomID(), randomID()
	sum := sha256.Sum256([]byte(verifier))
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	var once sync.Once
	callback := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || r.Method != "GET" || r.Host != listener.Addr().String() || r.URL.Path != "/callback" || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 || (len(q["code"]) != 1 && len(q["error"]) != 1) || (q.Has("code") && q.Has("error")) {
			http.Error(w, "Invalid callback", 400)
			return
		}
		accepted := false
		once.Do(func() {
			accepted = true
			if q.Get("error") != "" {
				done <- result{err: fmt.Errorf("sign-in was not approved")}
			} else if q.Get("code") == "" {
				done <- result{err: fmt.Errorf("empty authorization code")}
			} else {
				done <- result{code: q.Get("code")}
			}
		})
		if !accepted {
			http.Error(w, "Callback already used", 400)
			return
		}
		fmt.Fprintln(w, "You can return to Mu CLI.")
	})}
	defer callback.Close()
	go callback.Serve(listener)
	query := url.Values{"response_type": {"code"}, "client_id": {registered.ID}, "redirect_uri": {redirect}, "scope": {"read write api:agent api:home api:inbox api:services"}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}}
	address := server + "/oauth/authorize?" + query.Encode()
	fmt.Fprintln(output, "Open this URL to sign in:\n"+address)
	_ = open(address)
	select {
	case <-ctx.Done():
		return session{}, ctx.Err()
	case result := <-done:
		if result.err != nil {
			return session{}, result.err
		}
		s, err := exchange(ctx, h, server, url.Values{"grant_type": {"authorization_code"}, "client_id": {registered.ID}, "redirect_uri": {redirect}, "code": {result.code}, "code_verifier": {verifier}})
		s.Client = registered.ID
		return s, err
	}
}
