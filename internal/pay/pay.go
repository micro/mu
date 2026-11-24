// Package pay owns durable paid-session state. Agent composes access;
// billing supplies Stripe checkout and the existing account credit ledger.
package pay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
	"mu/internal/settings"
	pb "mu/proto/assistant/v1"
)

type Policy = pb.PayPolicy
type Session = pb.PaidSession

type Record struct {
	Session       *Session `json:"session"`
	Owner         string   `json:"owner"`
	StripeID      string   `json:"stripe_id"`
	StripeURL     string   `json:"stripe_url"`
	ReturnURL     string   `json:"return_url"`
	NotifyURL     string   `json:"notify_url"`
	NotifySecret  string   `json:"notify_secret"`
	DeliveryState string   `json:"delivery_state,omitempty"`
	Delivered     bool     `json:"delivered"`
	NextDelivery  int64    `json:"next_delivery"`
	Attempts      int      `json:"attempts"`
	Expires       int64    `json:"expires"`
}
type Provider interface {
	Checkout(context.Context, Record) (string, string, int64, error)
	Credit(context.Context, Record) error
}

// Backend is installed by server composition; nil disables Mu Pay.
var Backend Provider
var mu sync.Mutex
var checkoutMu sync.Mutex

const file = "pay/sessions.json"

var referencePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var ErrNotFound = errors.New("paid session not found")
var ErrInactive = errors.New("paid session is not active")

func Validate(p *Policy) error {
	if p == nil || !p.Enabled {
		return nil
	}
	if p.Price < 50 || p.Price > 100000 || p.Minutes < 1 || p.Minutes > 1440 {
		return errors.New("Mu Pay requires a price of 50–100000 minor units and duration of 1–1440 minutes")
	}
	if p.Currency != "gbp" && p.Currency != "usd" {
		return errors.New("Mu Pay supports GBP and USD")
	}
	if p.ReturnUrl == "" {
		return errors.New("Mu Pay requires an app return URL")
	}
	for _, raw := range []string{p.ReturnUrl, p.NotifyUrl} {
		if raw == "" && raw == p.NotifyUrl {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
			return errors.New("Mu Pay URLs must be HTTPS URLs without credentials or fragments")
		}
	}
	return nil
}
func creditGrant(p *Policy, n int32) (int32, error) {
	if p.Currency == "usd" {
		return p.Price * n, nil
	}
	// A fixed product rate, not a live GBP/USD conversion: £3 grants 300 credits.
	raw := settings.Get("MU_PAY_GBP_CREDITS_PER_POUND")
	if raw == "" {
		raw = "100"
	}
	rate, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || rate < 1 || rate > 10000 {
		return 0, errors.New("MU_PAY_GBP_CREDITS_PER_POUND must be an integer from 1 to 10000")
	}
	credits := int32((int64(p.Price) * rate / 100) * int64(n))
	if credits < 1 {
		return 0, errors.New("configured price grants less than one credit")
	}
	return credits, nil
}
func load() (map[string]Record, error) {
	records := map[string]Record{}
	raw, err := data.LoadFile(file)
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &records); err != nil || records == nil {
		return nil, errors.New("could not read Mu Pay sessions")
	}
	for id, r := range records {
		if r.Session == nil || r.Session.Id != id || r.Owner == "" {
			return nil, errors.New("invalid Mu Pay session record")
		}
	}
	return records, nil
}
func clone(r Record) Record {
	b, _ := json.Marshal(r)
	var c Record
	_ = json.Unmarshal(b, &c)
	return c
}
func view(r Record) *Session {
	s := clone(r).Session
	now := time.Now().Unix()
	if (s.Status == "active" && now >= s.EndsAt) || (s.Status == "pending" && now >= r.Expires) {
		s.Status = "expired"
	}
	s.NotificationStatus = "pending"
	switch {
	case r.NotifyURL == "":
		s.NotificationStatus = "disabled"
	case r.Delivered:
		s.NotificationStatus = "delivered"
	case r.DeliveryState == "sending" || r.DeliveryState == "uncertain":
		s.NotificationStatus = "uncertain"
	case r.DeliveryState == "failed":
		s.NotificationStatus = "retrying"
	}
	return s
}
func Checkout(ctx context.Context, owner, agent, base string, p *Policy, secret string, q *pb.CheckoutRequest) (*Session, error) {
	if Backend == nil {
		return nil, errors.New("Mu Pay is not configured")
	}
	if owner == "" || agent == "" || q == nil || !referencePattern.MatchString(q.Reference) || q.Participants < 1 || q.Participants > 100 {
		return nil, errors.New("provide an opaque reference and 1–100 participants")
	}
	checkoutMu.Lock()
	defer checkoutMu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return nil, err
	}
	var r Record
	for _, existing := range records {
		if existing.Owner == owner && existing.Session.Agent == agent && existing.Session.Reference == q.Reference {
			if existing.Session.Participants != q.Participants {
				return nil, errors.New("booking reference already has a different participant count")
			}
			r = existing
			break
		}
	}
	if r.Session == nil {
		if p == nil || !p.Enabled {
			return nil, errors.New("paid sessions are not enabled for this agent")
		}
		if err := Validate(p); err != nil {
			return nil, err
		}
		if p.NotifyUrl != "" && secret == "" {
			return nil, errors.New("notification signing secret is missing")
		}
		credits, err := creditGrant(p, q.Participants)
		if err != nil {
			return nil, err
		}
		// Bound abandoned checkout records per owner; keep receipts for recovery.
		pending := 0
		for _, v := range records {
			if v.Owner == owner && view(v).Status == "pending" {
				pending++
			}
		}
		if pending >= 100 {
			return nil, errors.New("too many pending checkouts")
		}
		var random [24]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, err
		}
		id := hex.EncodeToString(random[:])
		now := time.Now().Unix()
		returnURL, _ := url.Parse(p.ReturnUrl)
		query := returnURL.Query()
		query.Set("session", id)
		returnURL.RawQuery = query.Encode()
		r = Record{Owner: owner, ReturnURL: returnURL.String(), NotifyURL: p.NotifyUrl, NotifySecret: secret, Expires: now + 23*3600,
			Session: &Session{Id: id, Agent: agent, Reference: q.Reference, Participants: q.Participants, Amount: p.Price * q.Participants, Currency: p.Currency, Credits: credits, Minutes: p.Minutes, Status: "pending", CreatedAt: now, CheckoutUrl: strings.TrimRight(base, "/") + "/pay/" + id}}
		records[id] = r
		// Persist before contacting Stripe so retries use the same key and amounts.
		if err := data.SaveJSON(file, records); err != nil {
			return nil, err
		}
	}
	if view(r).Status == "expired" {
		return nil, errors.New("checkout expired; use a new booking reference")
	}
	if r.StripeID == "" {
		// Do not block agent admission or Stripe webhooks during network I/O.
		mu.Unlock()
		id, link, expires, checkoutErr := Backend.Checkout(ctx, clone(r))
		mu.Lock()
		if checkoutErr != nil {
			return nil, checkoutErr
		}
		records, err = load()
		if err != nil {
			return nil, err
		}
		// A webhook may already have settled while checkout was in flight.
		current, exists := records[r.Session.Id]
		if !exists {
			return nil, ErrNotFound
		}
		r = current
		if r.StripeID != "" && r.StripeID != id {
			return nil, errors.New("Stripe checkout changed")
		}
		r.StripeID, r.StripeURL, r.Expires = id, link, expires
		records[r.Session.Id] = r
		if err := data.SaveJSON(file, records); err != nil {
			return nil, err
		}
	}
	return view(r), nil
}
func Read(owner, agent, id string) (*Session, error) {
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return nil, err
	}
	r, ok := records[id]
	if !ok || r.Owner != owner || r.Session.Agent != agent {
		return nil, ErrNotFound
	}
	return view(r), nil
}
func Start(owner, agent, id string) (*Session, error) {
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return nil, err
	}
	r, ok := records[id]
	if !ok || r.Owner != owner || r.Session.Agent != agent {
		return nil, ErrNotFound
	}
	if view(r).Status == "active" || view(r).Status == "expired" {
		return view(r), nil
	}
	if r.Session.Status != "paid" {
		return nil, ErrInactive
	}
	now := time.Now().Unix()
	r.Session.StartedAt = now
	r.Session.EndsAt = now + int64(r.Session.Minutes)*60
	r.Session.Status = "active"
	records[id] = r
	if err := data.SaveJSON(file, records); err != nil {
		return nil, err
	}
	return view(r), nil
}

// Settlement is constructed only from a verified Stripe event.
type Settlement struct {
	ID, StripeID, Owner, Agent, Currency, Status string
	Amount                                       int32
}

func Settle(ctx context.Context, p Settlement) error {
	if p.Status != "paid" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return err
	}
	r, ok := records[p.ID]
	if !ok {
		return ErrNotFound
	}
	if p.StripeID == "" || r.Owner != p.Owner || r.Session.Agent != p.Agent || r.Session.Amount != p.Amount || r.Session.Currency != p.Currency ||
		(r.StripeID != "" && r.StripeID != p.StripeID) {
		return errors.New("payment does not match booked session")
	}
	if r.Session.PaidAt != 0 {
		return nil
	}
	if Backend == nil {
		return errors.New("Mu Pay unavailable")
	}
	r.StripeID = p.StripeID
	// CreditOnce is durable. If saving the receipt fails, webhook replay repeats
	// this step without granting credits twice, then completes the receipt.
	if err := Backend.Credit(ctx, clone(r)); err != nil {
		return err
	}
	r.Session.PaidAt = time.Now().Unix()
	r.Session.Status = "paid"
	records[p.ID] = r
	return data.SaveJSON(file, records)
}

// Check binds one paid session to one conversation. A paid thread may never be
// used as an ordinary call by omitting its session identifier.
func Check(owner, agent, id, thread string) error {
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return err
	}
	if id == "" {
		if thread != "" {
			for _, r := range records {
				if r.Owner == owner && r.Session.Thread == thread {
					return ErrInactive
				}
			}
		}
		return nil
	}
	r, ok := records[id]
	if !ok || r.Owner != owner || (agent != "" && r.Session.Agent != agent) {
		return ErrNotFound
	}
	if view(r).Status != "active" {
		return ErrInactive
	}
	if thread != "" && r.Session.Thread != "" && r.Session.Thread != thread {
		return errors.New("session belongs to another thread")
	}
	return nil
}
func Bind(owner, agent, id, thread string) error {
	mu.Lock()
	defer mu.Unlock()
	records, err := load()
	if err != nil {
		return err
	}
	r, ok := records[id]
	if !ok || r.Owner != owner || r.Session.Agent != agent {
		return ErrNotFound
	}
	if view(r).Status != "active" {
		return ErrInactive
	}
	if r.Session.Thread != "" && r.Session.Thread != thread {
		return errors.New("session already has a thread")
	}
	r.Session.Thread = thread
	records[id] = r
	return data.SaveJSON(file, records)
}
