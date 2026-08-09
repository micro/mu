package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"mu/internal/data"

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
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Secret          string    `json:"secret"`
	Created         time.Time `json:"created"`
	Admin           bool      `json:"admin"`
	Language        string    `json:"language"`
	Widgets         []string  `json:"widgets,omitempty"`         // App IDs to show as home widgets
	Pinned          []string  `json:"pinned,omitempty"`          // Service names pinned to the sidebar, in the order shown
	Approved        bool      `json:"approved,omitempty"`        // Admin-approved, bypasses new account restrictions
	Email           string    `json:"email,omitempty"`
	EmailVerified   bool      `json:"email_verified,omitempty"`
	EmailVerifiedAt time.Time `json:"email_verified_at,omitempty"`
	Banned          bool      `json:"banned,omitempty"` // Silently hidden from everyone except themselves
}

// legacyCardIDs maps retired card ids to their current name. Accounts saved
// before a rename still hold the old id, and the compatibility promise says an
// upgrade never loses a preference — so stored ids are canonicalised on read
// rather than migrated on disk.
var legacyCardIDs = map[string]string{
	"reminder": "prayer",
	"islam":    "prayer",
}

// canonicalCardID resolves a stored card id to its current name.
func canonicalCardID(id string) string {
	if cur, ok := legacyCardIDs[id]; ok {
		return cur
	}
	return id
}

// settingsFlag / setSettingsFlag record that a one-time migration has run.
func settingsFlag(name string) bool {
	var flags map[string]bool
	if err := data.LoadJSON("migrations.json", &flags); err != nil {
		return false
	}
	return flags[name]
}

func setSettingsFlag(name string) {
	var flags map[string]bool
	_ = data.LoadJSON("migrations.json", &flags)
	if flags == nil {
		flags = map[string]bool{}
	}
	flags[name] = true
	_ = data.SaveJSON("migrations.json", flags)
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
}

func init() {
	b, _ := data.LoadFile("accounts.json")
	json.Unmarshal(b, &accounts)
	b, _ = data.LoadFile("sessions.json")
	json.Unmarshal(b, &sessions)
	b, _ = data.LoadFile("tokens.json")
	json.Unmarshal(b, &tokens)
}

func Create(acc *Account) error {
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
	// admin and /admin/env is unreachable. The operator named in the ADMIN env
	// var (comma-separated ids/usernames/emails) is made an admin; if ADMIN is
	// unset, the very first account on a fresh instance becomes admin.
	if shouldBootstrapAdmin(acc, len(accounts) == 0) {
		acc.Admin = true
	}

	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	return nil
}

// shouldBootstrapAdmin reports whether a newly created account should be granted
// admin. ADMIN (or MU_ADMIN) explicitly lists admins; when neither is set the
// first account on an empty instance is bootstrapped so the operator can reach
// /admin/env. An existing admin is never demoted (this only runs at creation).
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

func Delete(acc *Account) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[acc.ID]; !ok {
		return errors.New("account does not exist")
	}

	delete(accounts, acc.ID)
	data.SaveJSON("accounts.json", accounts)

	return nil
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

func GetAllAccounts() []*Account {
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

// GetAccountByEmail finds an account by email (case-insensitive). Used for
// OAuth sign-in, where the email is the stable identity across providers.
func GetAccountByEmail(email string) (*Account, error) {
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

// GetAccountByName finds an account by username (case-insensitive)
func GetAccountByName(name string) (*Account, error) {
	mutex.Lock()
	defer mutex.Unlock()

	nameLower := strings.ToLower(name)
	for _, acc := range accounts {
		if strings.ToLower(acc.Name) == nameLower || strings.ToLower(acc.ID) == nameLower {
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

	// Run all registered cleanup hooks outside the lock.
	go func() {
		for _, hook := range AccountDeleteHooks {
			hook(id)
		}
	}()

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

func GenerateToken() string {
	id := uuid.New().String()
	return base64.StdEncoding.EncodeToString([]byte(id))
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

// IsOnline checks if a user is online (seen within last 3 minutes)
func IsOnline(username string) bool {
	presenceMutex.RLock()
	defer presenceMutex.RUnlock()

	lastSeen, exists := userPresence[username]
	if !exists {
		return false
	}

	return time.Since(lastSeen) < 3*time.Minute
}

// GetOnlineUsers returns a list of currently online usernames
func GetOnlineUsers() []string {
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

// HasCredit is set by main.go and reports whether an account has a positive
// credit balance. Kept as a hook because the wallet imports auth, not the
// other way round.
var HasCredit func(accountID string) bool

// trusted reports whether an account has shown it is a person rather than a
// signup script. Three things count, and any one of them is enough:
//
//   - an admin said so (Admin, Approved),
//   - a verified email address,
//   - money in the wallet.
//
// The 24-hour wait is not a fourth signal, it is what we fall back on when we
// have none of these. Waiting proves nothing about a bot — it costs a script
// nothing and costs a new user their whole first session — so anything that
// does carry signal has to be able to skip it.
//
// Takes a copy, not the live account, because it calls out to HasCredit and
// must not do that under auth's mutex: the wallet reads accounts, so holding
// the lock across the call would deadlock the first time someone checked a
// balance. Callers snapshot under the lock and evaluate after releasing it.
func trusted(acc Account) bool {
	if acc.Admin || acc.Approved || acc.EmailVerified {
		return true
	}
	return HasCredit != nil && HasCredit(acc.ID)
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
	if time.Since(acc.Created) < 24*time.Hour {
		remaining := (24*time.Hour - time.Since(acc.Created)).Round(time.Minute)
		return fmt.Sprintf("Verify your email address on /account, or add credit on /wallet, and you can post straight away. Otherwise a new account waits 24 hours — %s remaining.", remaining)
	}
	if VerificationRequired != nil && VerificationRequired() {
		return "Verify your email address on /account before posting."
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

// GetOnlineCount returns the number of online users
func GetOnlineCount() int {
	return len(GetOnlineUsers())
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
	}

	tokens[tokenID] = token
	data.SaveJSON("tokens.json", tokens)

	// Return the unhashed token only once (user must save it)
	return token, rawToken, nil
}

// ValidatePAT validates a Personal Access Token and returns the associated account ID
func ValidatePAT(rawToken string) (string, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Normalize: strip trailing base64 padding so tokens work with or without '='
	rawToken = strings.TrimRight(rawToken, "=")

	// Check all tokens to find a match (try without padding, then with)
	for _, token := range tokens {
		// Try raw token (no padding) first, then with padding for older tokens
		err := bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(rawToken))
		if err != nil {
			// Retry with padding in case the hash was generated with padded token
			padded := rawToken
			if m := len(padded) % 4; m != 0 {
				padded += strings.Repeat("=", 4-m)
			}
			err = bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(padded))
		}
		if err == nil {
			// Check if expired
			if !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt) {
				return "", errors.New("token expired")
			}

			// Update last used time
			token.LastUsed = time.Now()
			data.SaveJSON("tokens.json", tokens)

			return token.Account, nil
		}
	}

	return "", errors.New("invalid token")
}

// ValidatePATToken is ValidatePAT returning the token's id rather than its
// account, so a caller that needs the token record — to read its scope — can
// find it without a second comparison pass over every bcrypt hash.
func ValidatePATToken(rawToken string) (string, error) {
	rawToken = strings.TrimRight(rawToken, "=")

	mutex.Lock()
	defer mutex.Unlock()
	for _, token := range tokens {
		err := bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(rawToken))
		if err != nil {
			padded := rawToken
			if m := len(padded) % 4; m != 0 {
				padded += strings.Repeat("=", 4-m)
			}
			err = bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(padded))
		}
		if err == nil {
			if !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt) {
				return "", errors.New("token expired")
			}
			return token.ID, nil
		}
	}
	return "", errors.New("invalid token")
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
	data.SaveJSON("tokens.json", tokens)

	return nil
}

// GetTokenByID retrieves a token by ID (for display purposes)
func GetTokenByID(tokenID string) (*Token, error) {
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
