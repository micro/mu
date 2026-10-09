// Package registration owns shared password signup rules for both hosts.
package registration

import (
	"errors"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/x402/billing"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// Allowed returns true if the IP is allowed to sign up.
// It also records the attempt against the bucket on success.
// Configurable via SIGNUP_MAX_PER_IP (default 3) and SIGNUP_WINDOW_HOURS (default 24).
func Allowed(ip string) bool {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return true // never rate-limit localhost (self-hosted, dev)
	}
	maxPerIP := app.EnvInt("SIGNUP_MAX_PER_IP", 3)
	window := time.Duration(app.EnvInt("SIGNUP_WINDOW_HOURS", 24)) * time.Hour

	signupMu.Lock()
	defer signupMu.Unlock()

	now := time.Now()
	b, ok := signupAttempts[ip]
	if !ok || now.After(b.resetAt) {
		b = &signupBucket{count: 0, resetAt: now.Add(window)}
		signupAttempts[ip] = b
	}
	if b.count >= maxPerIP {
		return false
	}
	b.count++

	// Opportunistic GC to avoid unbounded growth.
	if len(signupAttempts) > 10000 {
		for k, v := range signupAttempts {
			if now.After(v.resetAt) {
				delete(signupAttempts, k)
			}
		}
	}
	return true
}

// Signup rate limiting per IP — defends against bulk account creation.
// Configurable via SIGNUP_MAX_PER_IP and SIGNUP_WINDOW_HOURS env vars.
var (
	signupMu       sync.Mutex
	signupAttempts = map[string]*signupBucket{}
)

type signupBucket struct {
	count   int
	resetAt time.Time
}

func Create(r *http.Request) (*auth.Session, error) {
	if err := r.ParseForm(); err != nil {
		return nil, errors.New("Invalid form")
	}
	invite := r.FormValue("invite")
	if auth.InviteOnly() && invite == "" {
		return nil, errors.New("An invite code is required")
	}
	if invite != "" {
		if err := auth.ValidateInvite(invite); err != nil {
			return nil, err
		}
	}
	if err := app.VerifyCaptchaRequest(r); err != nil {
		return nil, err
	}
	if !Allowed(app.ClientIP(r)) {
		return nil, errors.New("Too many sign-ups from your network. Please try again later.")
	}
	id, secret := r.FormValue("id"), r.FormValue("secret")
	if !regexp.MustCompile("^[a-z][a-z0-9_]{3,23}$").MatchString(id) {
		return nil, errors.New("Username must start with a letter and contain 4–24 lowercase letters, numbers or underscores")
	}
	if reason := auth.ValidateUsername(id); reason != "" {
		return nil, errors.New(reason)
	}
	if len(secret) < 6 {
		return nil, errors.New("Password must be at least 6 characters")
	}
	claimed := false
	if invite != "" {
		if existing := auth.UnclaimedFor(auth.InviteEmail(invite)); existing != nil {
			if err := auth.Claim(existing.ID, id, secret, billing.SignupCredits); err != nil {
				return nil, err
			}
			claimed = true
		}
	}
	if !claimed {
		if err := auth.Create(&auth.Account{ID: id, Secret: secret, SecretSet: true, SignupCredits: billing.SignupCredits, Name: id, Created: time.Now()}); err != nil {
			return nil, err
		}
	}
	if invite != "" {
		auth.ConsumeInvite(invite, id)
	}
	session, err := auth.Login(id, secret)
	if err != nil {
		return nil, errors.New("Account created but login failed. Please sign in.")
	}
	billing.RetrySignup(session.Account)
	return session, nil
}
