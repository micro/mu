package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
	"mu/internal/event"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var mutex sync.Mutex
var accounts = map[string]*Account{}
var sessions = map[string]*Session{}
var tokens = map[string]*Token{} // PAT tokens: tokenID -> Token

// User presence tracking
var presenceMutex sync.RWMutex
var userPresence = map[string]time.Time{} // username -> last seen time

type Account struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Secret   string    `json:"secret"`
	Created  time.Time `json:"created"`
	Admin    bool      `json:"admin"`
	Language string    `json:"language"`
	// Place is where this account is, in words — "London", "Lisbon,
	// Portugal" — with Lat and Lon where they are known and Zone the IANA
	// timezone.
	//
	// The single most useful fact an agent can have about somebody, and until
	// now it existed nowhere on this server. The weather card resolved
	// coordinates in the browser and kept them in localStorage, so asking the
	// weather agent "do I need a coat today" got "which city are you in?" —
	// from an instance whose home screen was showing that person's local
	// forecast at the time. Everything else inherited the hole: places could
	// not answer "near me", transit had no stop, prayer had no qibla, and a
	// scheduled run at 7am had no location at all because there was no browser
	// in the room.
	//
	// Coordinates are rounded to two decimal places before they are stored —
	// see account.SetPlace. That is about a kilometre, which is right for a
	// forecast, a prayer time and what is nearby, and is not somebody's
	// address.
	Place           string    `json:"place,omitempty"`
	Lat             float64   `json:"lat,omitempty"`
	Lon             float64   `json:"lon,omitempty"`
	Zone            string    `json:"zone,omitempty"`
	Pinned          []string  `json:"pinned,omitempty"`   // Service names pinned to the sidebar, in the order shown
	Approved        bool      `json:"approved,omitempty"` // Admin-approved, bypasses new account restrictions
	Email           string    `json:"email,omitempty"`
	EmailVerified   bool      `json:"email_verified,omitempty"`
	EmailVerifiedAt time.Time `json:"email_verified_at,omitempty"`
	// Addresses are other email addresses this account has proved it can read,
	// beyond the one above. See address.go: Email is the address the account
	// signs in and recovers with, and there is exactly one; these are the rest.
	Addresses []string `json:"addresses,omitempty"`
	Banned    bool     `json:"banned,omitempty"` // Silently hidden from everyone except themselves
	// Agent marks an account that is a program rather than a person: the
	// instance's own Micro, and anything else acting on its own behalf. An
	// admin may be an agent; not knowing which is how a password reset ends up
	// being mailed to a program.
	Agent bool `json:"agent,omitempty"`
	// Unclaimed marks an account created from an inbound email that nobody has
	// signed up for. It has no secret, so it cannot sign in; it exists so a
	// stranger writing to agent@ gets an answer and a conversation that is still
	// there when they do sign up. See unclaimed.go.
	Unclaimed bool `json:"unclaimed,omitempty"`
	// Turns is how many free exchanges an unclaimed account has used. Reset to
	// zero when it is claimed, at which point credits govern it like any other.
	Turns int `json:"turns,omitempty"`
	// InvitedAt is when the sign-up invitation was mailed, so it is mailed once.
	InvitedAt time.Time `json:"invited_at,omitempty"`
	// SecretSet marks a password its owner chose, as opposed to the random one
	// a Google signup is created with and never told. See password.go: without
	// it, "does this account have a password" cannot be answered, because every
	// account has a hash and only some of them have a password.
	SecretSet bool `json:"secret_set,omitempty"`
	// Customer is who Stripe thinks this account is: cus_….
	//
	// The only handle on a subscription once it exists. Without it there is no
	// way to open a billing portal and therefore no way to cancel, which is
	// what this product shipped with — a monthly charge whose only
	// customer-side exit was a failed card or a chargeback. Recorded by the
	// webhook, and looked up by email for anyone who paid before it was.
	Customer string `json:"customer,omitempty"`
}

// legacyCardIDs maps retired card ids to their current name. Accounts saved
// before a rename still hold the old id, and the compatibility promise says an
// upgrade never loses a preference — so stored ids are canonicalised on read
// rather than migrated on disk.
var legacyCardIDs = map[string]string{
	"reminder": "prayer",
	"islam":    "prayer",
}

type Session struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Token   string    `json:"token"`
	Account string    `json:"account"`
	Created time.Time `json:"created"`
}

// Token represents a Personal Access Token (PAT) for API automation
type Token struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`    // User-friendly name for the token
	Token       string    `json:"token"`   // The actual token value (hashed in storage)
	Account     string    `json:"account"` // Account ID this token belongs to
	Created     time.Time `json:"created"`
	LastUsed    time.Time `json:"last_used"`
	ExpiresAt   time.Time `json:"expires_at"`  // Optional expiration
	Permissions []string  `json:"permissions"` // e.g., "read", "write", "admin"

	// Lookup is sha256 of the raw token, hex, and it is how a token is found.
	//
	// Validation used to bcrypt-compare the presented token against every token
	// on the instance, twice each — once unpadded, once padded — while holding
	// the package mutex that accounts and sessions also use. bcrypt at cost 10
	// is about 60ms by design, so a hundred tokens is six seconds of CPU per
	// call with every other request on the instance blocked behind it. That is
	// not a slow admin page, it is one agent calling /mcp stalling the site.
	//
	// A hash rather than a slow KDF, because these are not passwords: a token
	// is 32 bytes from crypto/rand, so there is no dictionary to attack and
	// nothing for a work factor to buy. It is what GitHub and Stripe do with
	// theirs. The bcrypt hash stays in Token for tokens issued before this
	// existed; they are found by the old scan once and filled in here.
	Lookup string `json:"lookup,omitempty"`
}

// tokenBy is the index: sha256 of a raw token to the token. Guarded by mutex.
var tokenBy = map[string]*Token{}

// tokenKey is how a raw token is looked up.
func tokenKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(raw, "=")))
	return hex.EncodeToString(sum[:])
}

// indexToken files a token under its lookup. Caller holds mutex.
func indexToken(t *Token) {
	if t != nil && t.Lookup != "" {
		tokenBy[t.Lookup] = t
	}
}

func init() {
	b, _ := data.LoadFile("accounts.json")
	json.Unmarshal(b, &accounts)
	b, _ = data.LoadFile("sessions.json")
	json.Unmarshal(b, &sessions)
	b, _ = data.LoadFile("tokens.json")
	json.Unmarshal(b, &tokens)
	for _, t := range tokens {
		indexToken(t)
	}
}

// Create makes an account, if the username is one this instance allows.
//
// The check is here rather than at the signup handlers because this and Claim
// are the only two functions that put an id into the accounts map, and a rule
// enforced at the callers is a rule enforced at the callers who remembered.
// Two of them did; internal/setup and Claim did not, and micro.mu has an
// account called 3834 to show for it.
func Create(acc *Account) error {
	if reason := ValidateUsername(acc.ID); reason != "" {
		return errors.New(reason)
	}

	mutex.Lock()
	defer mutex.Unlock()

	_, exists := accounts[acc.ID]
	if exists {
		return errors.New("Account already exists")
	}

	// hash the secret
	hash, err := bcrypt.GenerateFromPassword([]byte(acc.Secret), 10)
	if err != nil {
		return err
	}

	acc.Secret = string(hash)

	// Admin bootstrap for self-hosting: without this a fresh instance has no
	// admin and /admin/config is unreachable. The operator named in the ADMIN env
	// var (comma-separated ids/usernames/emails) is made an admin; if ADMIN is
	// unset, the very first account on a fresh instance becomes admin.
	first := len(accounts) == 0
	if shouldBootstrapAdmin(acc, first) {
		acc.Admin = true
	}

	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	// Said, not sent. Whether anybody wants to know is not this package's
	// question — see event.AccountCreated. Published after the save, so a
	// subscriber that goes looking for the account finds it.
	event.Publish(event.Event{Type: event.AccountCreated, Data: map[string]interface{}{
		"account": acc.ID,
		"name":    acc.Name,
		"first":   strconv.FormatBool(first),
	}})

	return nil
}

// shouldBootstrapAdmin reports whether a newly created account should be granted
// admin. ADMIN (or MU_ADMIN) explicitly lists admins; when neither is set the
// first account on an empty instance is bootstrapped so the operator can reach
// /admin/config. An existing admin is never demoted (this only runs at creation).
func shouldBootstrapAdmin(acc *Account, isFirst bool) bool {
	list := os.Getenv("ADMIN")
	if list == "" {
		list = os.Getenv("MU_ADMIN")
	}
	if list == "" {
		return isFirst // no explicit config — bootstrap the first account
	}
	for _, want := range strings.Split(list, ",") {
		want = strings.ToLower(strings.TrimSpace(want))
		if want == "" {
			continue
		}
		if want == strings.ToLower(acc.ID) || want == strings.ToLower(acc.Name) ||
			(acc.Email != "" && want == strings.ToLower(acc.Email)) {
			return true
		}
	}
	return false // ADMIN set but this account isn't on the list
}

func GetAccount(id string) (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}

	return acc, nil
}

func UpdateAccount(acc *Account) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[acc.ID]; !ok {
		return errors.New("account does not exist")
	}

	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	return nil
}

func AllAccounts() []*Account {
	mutex.Lock()
	defer mutex.Unlock()

	list := make([]*Account, 0, len(accounts))
	for _, acc := range accounts {
		list = append(list, acc)
	}
	return list
}

// AdminExists reports whether any account has admin rights. Used to gate the
// first-run setup flow: while no admin exists the instance is unconfigured.
func AdminExists() bool {
	mutex.Lock()
	defer mutex.Unlock()
	for _, acc := range accounts {
		if acc.Admin {
			return true
		}
	}
	return false
}

// AccountByEmail finds an account by email (case-insensitive). Used for
// OAuth sign-in, where the email is the stable identity across providers.
func AccountByEmail(email string) (*Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("email required")
	}
	mutex.Lock()
	defer mutex.Unlock()
	for _, acc := range accounts {
		if strings.ToLower(acc.Email) == email {
			return acc, nil
		}
	}
	return nil, errors.New("account not found")
}

// AccountByUsername finds an account by its username, case-insensitively.
//
// The username is the ID: unique, enforced unique at signup, validated by
// ValidateUsername, and the thing the product shows everywhere — @handle, the
// mail local part, author_id. Name is a *display* name. It is free text, it is
// not unique, it is not validated, and at Google sign-in it is whatever the
// Google profile says.
//
// This replaces GetAccountByName, which matched either and returned the first
// hit from a map — so "pay asim" could credit the account whose username is
// asim, or a different account that had simply typed "asim" into its display
// name, depending on Go's randomised iteration order that call. Money moved on
// a coin flip.
//
// Matching a display name was the error underneath the coin flip. A display
// name is not an identifier and must never resolve one: anyone can set theirs
// to a popular username, and if that could receive a transfer it would be a way
// to harvest other people's money rather than a bug.
func AccountByUsername(username string) (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()

	want := strings.ToLower(strings.TrimSpace(username))
	if want == "" {
		return nil, errors.New("account not found")
	}
	for _, acc := range accounts {
		if strings.ToLower(acc.ID) == want {
			return acc, nil
		}
	}
	return nil, errors.New("account not found")
}

// AccountDeleteHooks are called when an account is deleted. Each
// building block registers a hook to clean up its own data. This
// avoids auth importing every other package.
var AccountDeleteHooks []func(accountID string)

func DeleteAccount(id string) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[id]; !ok {
		return errors.New("account does not exist")
	}

	delete(accounts, id)

	// Delete sessions for this account.
	for sid, sess := range sessions {
		if sess.Account == id {
			delete(sessions, sid)
		}
	}

	// Delete PATs for this account.
	for tid, tok := range tokens {
		if tok.Account == id {
			delete(tokens, tid)
		}
	}

	data.SaveJSON("accounts.json", accounts)
	data.SaveJSON("sessions.json", sessions)
	data.SaveJSON("tokens.json", tokens)

	// And the SSH keys, which are the same kind of thing as a token: a
	// credential that says "this is them". One left behind is a way into
	// nothing at all, right up until somebody signs up with the same name.
	DeleteSSHKeysFor(id)

	// Run all registered cleanup hooks outside the lock.
	go func() {
		for _, hook := range AccountDeleteHooks {
			hook(id)
		}
	}()

	return nil
}

// CheckSecret verifies an account's password without signing anybody in.
//
// Login does this too, and mints a session as well — which is wrong for asking
// somebody to prove who they are before doing something dangerous with the
// session they already have. The point of re-authenticating is that the cookie
// is not enough on its own; issuing a second one in the middle of it is exactly
// backwards.
//
// An account signed up through Google or a passkey has no password anybody
// knows, so this can only ever say no to them. That is not a bug in this
// function, but a caller must say so in words rather than reporting a wrong
// password.
func CheckSecret(id, secret string) error {
	mutex.Lock()
	acc, ok := accounts[id]
	mutex.Unlock()
	if !ok {
		return errors.New("account does not exist")
	}
	if strings.TrimSpace(acc.Secret) == "" {
		return errors.New("this account has no password set")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.Secret), []byte(secret)); err != nil {
		return errors.New("invalid account secret")
	}
	return nil
}

func Login(id, secret string) (*Session, error) {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}

	err := bcrypt.CompareHashAndPassword([]byte(acc.Secret), []byte(secret))
	if err != nil {
		return nil, errors.New("invalid account secret")
	}

	guid := uuid.New().String()

	sess := &Session{
		ID:      guid,
		Type:    "account",
		Token:   base64.StdEncoding.EncodeToString([]byte(guid)),
		Account: acc.ID,
		Created: time.Now(),
	}

	// store the session
	sessions[guid] = sess
	data.SaveJSON("sessions.json", sessions)

	return sess, nil
}

// CreateSession creates a new session for the given account ID without password validation.
// Used for passkey authentication where identity is verified via WebAuthn.
func CreateSession(id string) (*Session, error) {
	mutex.Lock()
	defer mutex.Unlock()

	_, ok := accounts[id]
	if !ok {
		return nil, errors.New("account does not exist")
	}

	guid := uuid.New().String()

	sess := &Session{
		ID:      guid,
		Type:    "account",
		Token:   base64.StdEncoding.EncodeToString([]byte(guid)),
		Account: id,
		Created: time.Now(),
	}

	sessions[guid] = sess
	data.SaveJSON("sessions.json", sessions)

	return sess, nil
}

func Logout(tk string) error {
	sess, err := ParseToken(tk)
	if err != nil {
		return err
	}

	mutex.Lock()
	delete(sessions, sess.ID)
	data.SaveJSON("sessions.json", sessions)
	mutex.Unlock()

	return nil
}

func GetSession(r *http.Request) (*Session, error) {
	// Try cookie first
	c, err := r.Cookie("session")
	if err == nil && c != nil {
		sess, err := ParseToken(c.Value)
		if err == nil {
			// Validate that the account still exists
			mutex.Lock()
			_, accountExists := accounts[sess.Account]
			if !accountExists {
				// Account was deleted, invalidate the session
				delete(sessions, sess.ID)
			}
			mutex.Unlock()

			if !accountExists {
				return nil, errors.New("account no longer exists")
			}

			return sess, nil
		}
	}

	// Try Authorization header (PAT, session token, or Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Support both "Bearer <token>" and just "<token>"
		token := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		// Try as PAT first
		accountID, err := ValidatePAT(token)
		if err == nil {
			return &Session{
				Type:    "token",
				Account: accountID,
			}, nil
		}

		// Try as session token (returned by login/signup MCP tools)
		sess, err := ParseToken(token)
		if err == nil {
			mutex.Lock()
			_, accountExists := accounts[sess.Account]
			if !accountExists {
				delete(sessions, sess.ID)
			}
			mutex.Unlock()
			if accountExists {
				return sess, nil
			}
		}
	}

	// Try X-Micro-Token header (legacy)
	tokenHeader := r.Header.Get("X-Micro-Token")
	if tokenHeader != "" {
		accountID, err := ValidatePAT(tokenHeader)
		if err == nil {
			// Create a pseudo-session for PAT
			return &Session{
				Type:    "token",
				Account: accountID,
			}, nil
		}
	}

	return nil, errors.New("session not found")
}

// RequireSession returns the session and account, or an error if not authenticated
// This is a convenience function that combines GetSession and GetAccount
func RequireSession(r *http.Request) (*Session, *Account, error) {
	sess, err := GetSession(r)
	if err != nil {
		return nil, nil, errors.New("authentication required")
	}

	acc, err := GetAccount(sess.Account)
	if err != nil {
		return nil, nil, errors.New("account not found")
	}

	// Using the instance is being on it.
	//
	// UpdatePresence had exactly one caller: a JSON endpoint the browser polls
	// for an online count. So somebody could read every page on this server for
	// an hour and never be present, because presence was a property of one poll
	// rather than of using the place — and Home's Here strip, which is supposed
	// to say who is about, could not see the person reading it.
	//
	// Here because this is the one function that answers "which account is this
	// request", and every page, API call and tool call goes through it or
	// through TrySession, which is this. A map write behind its own mutex, on a
	// request that has already done a session lookup.
	UpdatePresence(acc.ID)

	return sess, acc, nil
}

// TrySession returns the session and account if authenticated, or nil values if not
// Use this for optional auth checks where you want to show different content for guests vs users
func TrySession(r *http.Request) (*Session, *Account) {
	sess, acc, err := RequireSession(r)
	if err != nil {
		return nil, nil
	}
	return sess, acc
}

// RequireAdmin returns the session and account if the user is an admin, or an error
func RequireAdmin(r *http.Request) (*Session, *Account, error) {
	sess, acc, err := RequireSession(r)
	if err != nil {
		return nil, nil, err
	}

	if !acc.Admin {
		return nil, nil, errors.New("admin access required")
	}

	return sess, acc, nil
}

func ParseToken(tk string) (*Session, error) {
	dec, err := base64.StdEncoding.DecodeString(tk)
	if err != nil {
		return nil, errors.New("invalid session")
	}

	id, err := uuid.Parse(string(dec))
	if err != nil {
		return nil, errors.New("invalid session")
	}

	mutex.Lock()
	sess, ok := sessions[id.String()]
	mutex.Unlock()

	if !ok {
		return nil, errors.New("session not found")
	}

	return sess, nil
}

func ValidateToken(tk string) error {
	if len(tk) == 0 {
		return errors.New("invalid token")
	}

	// Try session token first
	sess, err := ParseToken(tk)
	if err == nil {
		if sess.Type != "account" {
			return errors.New("invalid session")
		}
		return nil
	}

	// Try PAT token
	_, err = ValidatePAT(tk)
	if err == nil {
		return nil
	}

	return errors.New("invalid token")
}

// UpdatePresence updates the last seen time for a user
func UpdatePresence(username string) {
	presenceMutex.Lock()
	defer presenceMutex.Unlock()
	userPresence[username] = time.Now()
}

// OnlineUsers returns a list of currently online usernames
func OnlineUsers() []string {
	presenceMutex.RLock()
	defer presenceMutex.RUnlock()

	var online []string
	now := time.Now()

	for username, lastSeen := range userPresence {
		if now.Sub(lastSeen) < 3*time.Minute {
			online = append(online, username)
		}
	}

	return online
}

// VerificationRequired is set by main.go and reports whether email
// verification is currently required to post on this instance. When it
// returns false (e.g. mail isn't configured) CanPost falls back to the
// older "any account can post" rule. Defaults to false (no verification)
// so self-hosters without mail aren't accidentally locked out.
var VerificationRequired func() bool

// HasPaid is set by main.go and reports whether an account has ever put money
// in. Kept as a hook because the wallet imports auth, not the other way round.
//
// It was HasCredit and it asked whether the balance was positive, which is a
// different question and stopped being the same one the day new accounts were
// given a hundred credits to start with. A grant is not a signal — see
// account.Paid, which is what this points at.
var HasPaid func(accountID string) bool

// trusted reports whether an account has shown it is a person rather than a
// signup script. Three things count, and any one of them is enough:
//
//   - an admin said so (Admin, Approved),
//   - a verified email address,
//   - money the account put in.
//
// Put in, not held. What makes money a signal is that producing it costs
// something a script cannot spend at scale — a card that clears is a person a
// chargeback can reach. Credits we handed out at signup cost the holder
// nothing, so a balance made of them says only that somebody signed up, and
// reading it as trust promoted every new account past the 24-hour wait, the
// new-account post cap, the agent cap and the gate on mail leaving here.
//
// The 24-hour wait is not a fourth signal, it is what we fall back on when we
// have none of these. Waiting proves nothing about a bot — it costs a script
// nothing and costs a new user their whole first session — so anything that
// does carry signal has to be able to skip it.
//
// Takes a copy, not the live account, because it calls out to HasPaid and
// must not do that under auth's mutex: the wallet reads accounts, so holding
// the lock across the call would deadlock the first time someone checked a
// balance. Callers snapshot under the lock and evaluate after releasing it.
func trusted(acc Account) bool {
	if acc.Admin || acc.Approved || acc.EmailVerified {
		return true
	}
	return HasPaid != nil && HasPaid(acc.ID)
}

// Trusted is the same question asked by account id.
//
// Exported because posting is not the only thing that spends something shared.
// Mail leaving this instance goes out under a domain that also carries password
// resets, and what an unaccountable account sends is charged to the
// deliverability of the mail that has to arrive. That wants the same answer
// this already gives, not a second notion of accountability with its own bugs —
// see service/mail/outbound.go.
func Trusted(accountID string) bool {
	acc, exists := snapshot(accountID)
	return exists && trusted(acc)
}

// snapshot returns a copy of an account, or false if there is no such account.
// The copy is what the trust rules below run against, so they can call out to
// the wallet without holding the lock.
func snapshot(accountID string) (Account, bool) {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return Account{}, false
	}
	return *acc, true
}

// CanPost checks if an account is allowed to create content.
// Rules:
//   - A trusted account (see above) can always post.
//   - Otherwise it waits out its first 24 hours.
//   - After that, if this instance requires verification, it still needs a
//     verified address — which trusted() already covers, so reaching here
//     unverified on such an instance means no.
func CanPost(accountID string) bool {
	acc, exists := snapshot(accountID)
	if !exists {
		return false
	}

	if trusted(acc) {
		return true
	}

	// Must be at least 24 hours old.
	if time.Since(acc.Created) < 24*time.Hour {
		return false
	}

	return VerificationRequired == nil || !VerificationRequired()
}

// PostBlockReason returns a human-readable reason an account cannot post,
// or an empty string if it can. Used by handlers to render helpful errors.
func PostBlockReason(accountID string) string {
	acc, exists := snapshot(accountID)
	if !exists {
		return "Account not found"
	}
	if trusted(acc) {
		return ""
	}
	// Lead with the way out. The wait is the fallback, not the instruction —
	// a reason that only says "come back tomorrow" reads as a dead end when
	// two doors are open right now.
	//
	// Named the way a person would name them — "in your Account", "to your
	// Wallet" — rather than as paths. A route written into a sentence reads as
	// a link, so it gets clicked, and in plain text nothing happens; and even
	// where it is turned into one, /account is how the page is addressed rather
	// than what it is called. The words are what the renderer links, so this
	// stays a sentence and not markup.
	if time.Since(acc.Created) < 24*time.Hour {
		remaining := (24*time.Hour - time.Since(acc.Created)).Round(time.Minute)
		return fmt.Sprintf("Verify your email address in your Account, or add credit to your Balance, and you can post straight away. Otherwise a new account waits 24 hours — %s remaining.", remaining)
	}
	if VerificationRequired != nil && VerificationRequired() {
		return "Verify your email address in your Account before posting."
	}
	return ""
}

// IsNewAccount reports whether we still know nothing about this account.
//
// The blog hides posts by a new account from its lists, which is the right
// instinct — a spam run should not reach the front page — but it has to mean
// the same thing as CanPost or the product contradicts itself: the post is
// accepted, charged, indexed, the author is redirected to the list, and the
// post is not on it. Nothing says why, because from the writer's side nothing
// went wrong.
//
// So "new" is the same question as "untrusted", asked once. An account that has
// verified an address or put money in its wallet is not new to us, whatever the
// clock says.
func IsNewAccount(accountID string) bool {
	acc, exists := snapshot(accountID)
	if !exists {
		return false
	}
	if trusted(acc) {
		return false
	}
	return time.Since(acc.Created) < 24*time.Hour
}

// IsBanned returns true if the account is banned. Content from banned
// users is silently hidden from everyone except the user themselves —
// they don't know they're muted.
func IsBanned(accountID string) bool {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return false
	}
	return acc.Banned
}

// BanAccount silently mutes a user. Their content is hidden from
// all other users, but they can still browse and post (to themselves).
// Admins can never be banned — this is a hard safety guard.
func BanAccount(accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return errors.New("account not found")
	}
	if acc.Admin {
		return errors.New("cannot ban an admin account")
	}
	acc.Banned = true
	data.SaveJSON("accounts.json", accounts)
	return nil
}

// UnbanAccount lifts a ban.
func UnbanAccount(accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()
	acc, exists := accounts[accountID]
	if !exists {
		return errors.New("account not found")
	}
	acc.Banned = false
	data.SaveJSON("accounts.json", accounts)
	return nil
}

// ApproveAccount marks an account as approved, bypassing new account restrictions
func ApproveAccount(accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	acc, exists := accounts[accountID]
	if !exists {
		return errors.New("account not found")
	}
	acc.Approved = true
	data.SaveJSON("accounts.json", accounts)
	return nil
}

// OnlineCount returns the number of online users
func OnlineCount() int {
	return len(OnlineUsers())
}

// ============================================
// Personal Access Token (PAT) Management
// ============================================

// CreateToken creates a new Personal Access Token for an account
func CreateToken(accountID, name string, permissions []string, expiresAt time.Time) (*Token, string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Verify account exists
	_, exists := accounts[accountID]
	if !exists {
		return nil, "", errors.New("account does not exist")
	}

	// Generate a cryptographically secure token
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return nil, "", err
	}
	rawToken := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// Hash the token for storage
	hash, err := bcrypt.GenerateFromPassword([]byte(rawToken), 10)
	if err != nil {
		return nil, "", err
	}

	tokenID := uuid.New().String()
	token := &Token{
		ID:          tokenID,
		Name:        name,
		Token:       string(hash),
		Account:     accountID,
		Created:     time.Now(),
		LastUsed:    time.Time{},
		ExpiresAt:   expiresAt,
		Permissions: permissions,
		Lookup:      tokenKey(rawToken),
	}

	tokens[tokenID] = token
	indexToken(token)
	data.SaveJSON("tokens.json", tokens)

	// Return the unhashed token only once (user must save it)
	return token, rawToken, nil
}

// ValidatePAT validates a Personal Access Token and returns the account it
// belongs to.
func ValidatePAT(rawToken string) (string, error) {
	t, err := tokenFor(rawToken)
	if err != nil {
		return "", err
	}
	return t.Account, nil
}

// ValidatePATToken is ValidatePAT returning the token's id rather than its
// account, so a caller that needs the token record — to read its scope — can
// find it without a second pass.
func ValidatePATToken(rawToken string) (string, error) {
	t, err := tokenFor(rawToken)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// tokenFor resolves a presented token, by index where it can and by comparison
// where it must.
//
// The index is one map lookup. The fallback exists for tokens issued before the
// index did: those are compared against their bcrypt hash, and — this is the
// part that matters — the comparing happens with the mutex released, because it
// is the slow thing and everything else in this package waits on that lock. A
// token found the slow way is indexed, so it is only ever slow once.
func tokenFor(rawToken string) (*Token, error) {
	rawToken = strings.TrimRight(rawToken, "=")
	if rawToken == "" {
		return nil, errors.New("invalid token")
	}

	mutex.Lock()
	t := tokenBy[tokenKey(rawToken)]
	var legacy []*Token
	if t == nil {
		for _, candidate := range tokens {
			if candidate.Lookup == "" {
				legacy = append(legacy, candidate)
			}
		}
	}
	mutex.Unlock()

	if t == nil {
		// Outside the lock. This is the path that used to hold it.
		padded := rawToken
		if m := len(padded) % 4; m != 0 {
			padded += strings.Repeat("=", 4-m)
		}
		for _, candidate := range legacy {
			if bcrypt.CompareHashAndPassword([]byte(candidate.Token), []byte(rawToken)) != nil &&
				bcrypt.CompareHashAndPassword([]byte(candidate.Token), []byte(padded)) != nil {
				continue
			}
			t = candidate
			mutex.Lock()
			t.Lookup = tokenKey(rawToken)
			indexToken(t)
			mutex.Unlock()
			break
		}
	}
	if t == nil {
		return nil, errors.New("invalid token")
	}
	if !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return nil, errors.New("token expired")
	}

	touchToken(t)
	return t, nil
}

// touchToken notes that a token was used, and writes that down rarely.
//
// Every successful validation wrote the whole token file to disk, under the
// package mutex, so an agent polling an endpoint rewrote every token on the
// instance on every call. The field is a diagnostic — "last called 3 Jan 14:02"
// on the connect page — and a minute of resolution is more than it needs.
func touchToken(t *Token) {
	now := time.Now()
	mutex.Lock()
	defer mutex.Unlock()
	if now.Sub(t.LastUsed) < time.Minute {
		t.LastUsed = now
		return
	}
	t.LastUsed = now
	data.SaveJSON("tokens.json", tokens) //nolint:errcheck
}

// ListTokens returns all PAT tokens for an account (with hashed values)
func ListTokens(accountID string) []*Token {
	mutex.Lock()
	defer mutex.Unlock()

	var result []*Token
	for _, token := range tokens {
		if token.Account == accountID {
			result = append(result, token)
		}
	}
	return result
}

// DeleteToken removes a PAT token
func DeleteToken(tokenID, accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	token, exists := tokens[tokenID]
	if !exists {
		return errors.New("token does not exist")
	}

	// Verify the token belongs to the account
	if token.Account != accountID {
		return errors.New("unauthorized")
	}

	delete(tokens, tokenID)
	delete(tokenBy, token.Lookup)
	data.SaveJSON("tokens.json", tokens)

	return nil
}

// TokenByID retrieves a token by ID (for display purposes)
func TokenByID(tokenID string) (*Token, error) {
	mutex.Lock()
	defer mutex.Unlock()

	token, exists := tokens[tokenID]
	if !exists {
		return nil, errors.New("token does not exist")
	}

	return token, nil
}

// HasPermission checks if a token has a specific permission
func (t *Token) HasPermission(perm string) bool {
	for _, p := range t.Permissions {
		if p == perm || p == "all" {
			return true
		}
	}
	return false
}

// PinnedServices is the services this account keeps in its sidebar, in order.
//
// Empty is the default and the common case. The sidebar teaches three levels —
// agents, tools, services — and a reader who has pinned nothing sees exactly
// those, which is what somebody arriving from a landing page that says tools
// for agents should see.
//
// It stops being the right thing the moment you use one of the services. The
// nav went from nineteen alphabetical entries to none, and reaching for Video
// meant going to the catalogue and hunting; the answer to both is a list that
// is yours rather than the instance's opinion.
func (a *Account) PinnedServices() []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.Pinned))
	seen := map[string]bool{}
	for _, name := range a.Pinned {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// SetPinned records the selection and its order.
func (a *Account) SetPinned(names []string) {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	a.Pinned = out
}

// Pin and Unpin say which they mean.
//
// The only control used to be TogglePin, so the request carried "flip this"
// rather than "make it pinned". Two tabs showing the same page then disagreed:
// pin from one, pin from the other, and the second flip unpins what the first
// just pinned — the reader pressed pin twice and ended with it off. Naming the
// outcome makes the request idempotent, which is what a control that can be
// pressed from two places has to be.
func (a *Account) Pin(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}
	for _, p := range a.PinnedServices() {
		if p == name {
			return
		}
	}
	a.SetPinned(append(a.PinnedServices(), name))
}

func (a *Account) Unpin(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}
	cur := a.PinnedServices()
	out := make([]string, 0, len(cur))
	for _, p := range cur {
		if p != name {
			out = append(out, p)
		}
	}
	a.SetPinned(out)
}

// TogglePin adds a service if absent, removes it if present, and reports
// whether it is pinned afterwards.
func (a *Account) TogglePin(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for i, p := range a.PinnedServices() {
		if p == name {
			cur := a.PinnedServices()
			a.SetPinned(append(cur[:i:i], cur[i+1:]...))
			return false
		}
	}
	a.SetPinned(append(a.PinnedServices(), name))
	return true
}
