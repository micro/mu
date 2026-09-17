// Package onboarding retains a first message until its sender proves possession
// of the channel. It never calls a model or chooses an existing account.
package onboarding

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"mu/internal/abuse"
	"mu/internal/data"
)

type Request struct {
	Channel   string
	Address   string
	Text      string
	Subject   string
	MessageID string
	Account   string
	Expires   time.Time
	State     string
	Reply     string
}

var mu sync.Mutex

func key(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func path(id string) string { return "onboarding/" + id + ".json" }

func remove(name string) error {
	if err := data.DeleteFile("onboarding/" + name); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := data.DeleteFile("onboarding/" + name + ".prev"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Begin bounds verification replies, not browsing or login. Only callers that
// authenticated the inbound channel may call it. The raw token is not stored.
func Begin(r Request) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if r.Channel != "mail" && r.Channel != "sms" && r.Channel != "whatsapp" {
		return "", errors.New("unknown channel")
	}
	if r.Address == "" || r.Text == "" || len(r.Text) > 16000 {
		return "", errors.New("invalid first message")
	}
	keys, err := data.ListKeys("onboarding")
	if err != nil {
		return "", err
	}
	active := 0
	for _, name := range keys {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var old Request
		if err := data.LoadJSON("onboarding/"+name, &old); err != nil {
			return "", err
		}
		if time.Now().After(old.Expires) {
			if err := remove(name); err != nil {
				return "", err
			}
			continue
		}
		active++
		if old.Channel == r.Channel && old.Address == r.Address {
			return "", nil
		}
	}
	if active >= 1000 {
		return "", errors.New("verification capacity reached")
	}
	for _, limit := range []struct {
		key string
		max int
	}{{"onboarding:sender:" + r.Channel + ":" + r.Address, 3}, {"onboarding:instance", 100}} {
		wait, err := abuse.Take(limit.key, limit.max, 24*time.Hour)
		if err != nil {
			return "", err
		}
		if wait > 0 {
			return "", errors.New("verification reply allowance reached")
		}
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b[:])
	r.Account = "member" + token[:16]
	r.Expires = time.Now().Add(30 * time.Minute)
	r.State = "pending"
	return token, data.SaveJSON(path(key(token)), r)
}

func read(token string) (*Request, error) {
	if len(token) != 64 {
		return nil, errors.New("This link is invalid or has expired.")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, errors.New("This link is invalid or has expired.")
	}
	var r Request
	if err := data.LoadJSON(path(key(token)), &r); err != nil || time.Now().After(r.Expires) {
		return nil, errors.New("This link is invalid or has expired.")
	}
	return &r, nil
}

func Read(token string) (*Request, error) { mu.Lock(); defer mu.Unlock(); return read(token) }

// Confirm consumes a proof once. The callback must only create the private
// account and channel binding; model execution happens after the durable queue.
func Confirm(token string, create func(*Request) error) (*Request, error) {
	mu.Lock()
	defer mu.Unlock()
	r, err := read(token)
	if err != nil {
		return nil, err
	}
	if r.State != "pending" {
		return nil, errors.New("This link has already been used.")
	}
	// A failed or interrupted account creation is never replayed automatically.
	r.State = "confirming"
	if err = data.SaveJSON(path(key(token)), r); err != nil {
		return nil, err
	}
	if err = create(r); err != nil {
		return nil, err
	}
	r.State = "queued"
	r.Expires = time.Now().Add(24 * time.Hour)
	return r, data.SaveJSON(path(key(token)), r)
}

// Next marks a job working before any external action. After a restart an
// interrupted job is not replayed, because a tool action may have completed.
func Next() (string, *Request, error) {
	mu.Lock()
	defer mu.Unlock()
	keys, err := data.ListKeys("onboarding")
	if err != nil {
		return "", nil, err
	}
	for _, name := range keys {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var r Request
		if err := data.LoadJSON("onboarding/"+name, &r); err != nil {
			return "", nil, err
		}
		if time.Now().After(r.Expires) {
			if err := remove(name); err != nil {
				return "", nil, err
			}
			continue
		}
		if r.State != "queued" {
			continue
		}
		r.State = "working"
		if err := data.SaveJSON("onboarding/"+name, &r); err != nil {
			return "", nil, err
		}
		return name, &r, nil
	}
	return "", nil, nil
}

func Complete(name string, r *Request) error {
	mu.Lock()
	defer mu.Unlock()
	r.State = "done"
	r.Text = ""
	r.Subject = ""
	r.Reply = ""
	if err := data.SaveJSON("onboarding/"+name, r); err != nil {
		return err
	}
	if err := data.DeleteFile("onboarding/" + name + ".prev"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Recover records interrupted execution without repeating possible tool actions.
func Recover() error {
	mu.Lock()
	defer mu.Unlock()
	keys, err := data.ListKeys("onboarding")
	if err != nil {
		return err
	}
	for _, name := range keys {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var r Request
		if err := data.LoadJSON("onboarding/"+name, &r); err != nil {
			return err
		}
		if r.State == "working" || r.State == "confirming" {
			r.State = "interrupted"
			if err := data.SaveJSON("onboarding/"+name, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func Failed(name string, r *Request) error {
	mu.Lock()
	defer mu.Unlock()
	r.State = "failed"
	return data.SaveJSON("onboarding/"+name, r)
}

func Forget(account string) {
	mu.Lock()
	defer mu.Unlock()
	keys, err := data.ListKeys("onboarding")
	if err != nil {
		return
	}
	for _, name := range keys {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var r Request
		if data.LoadJSON("onboarding/"+name, &r) == nil && r.Account == account {
			_ = remove(name)
		}
	}
}
