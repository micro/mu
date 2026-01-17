package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"mu/data"

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
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Secret               string    `json:"secret"`
	Created              time.Time `json:"created"`
	Admin                bool      `json:"admin"`
	Member               bool      `json:"member"`
	Language             string    `json:"language"`
	Widgets              []string  `json:"widgets,omitempty"` // App IDs to show as home widgets
	StripeCustomerID     string    `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string    `json:"stripe_subscription_id,omitempty"`
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

	accounts[acc.ID] = acc
	data.SaveJSON("accounts.json", accounts)

	return nil
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

func DeleteAccount(id string) error {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[id]; !ok {
		return errors.New("account does not exist")
	}

	delete(accounts, id)

	// Also delete any sessions for this account
	for sid, sess := range sessions {
		if sess.Account == id {
			delete(sessions, sid)
		}
	}

	data.SaveJSON("accounts.json", accounts)
	data.SaveJSON("sessions.json", sessions)

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

	// Try Authorization header (PAT or Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Support both "Bearer <token>" and just "<token>"
		token := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		accountID, err := ValidatePAT(token)
		if err == nil {
			// Create a pseudo-session for PAT
			return &Session{
				Type:    "token",
				Account: accountID,
			}, nil
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

// RequireMember returns the session and account if the user is a member or admin, or an error
func RequireMember(r *http.Request) (*Session, *Account, error) {
	sess, acc, err := RequireSession(r)
	if err != nil {
		return nil, nil, err
	}

	if !acc.Member && !acc.Admin {
		return nil, nil, errors.New("member access required")
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

// CanPost checks if an account is old enough to post (30 minutes)
func CanPost(accountID string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	acc, exists := accounts[accountID]
	if !exists {
		return false
	}

	// Admins and members can always post
	if acc.Admin || acc.Member {
		return true
	}

	// Account must be at least 30 minutes old
	return time.Since(acc.Created) >= 30*time.Minute
}

// IsNewAccount checks if account is less than 24 hours old
func IsNewAccount(accountID string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	acc, exists := accounts[accountID]
	if !exists {
		return false
	}

	// Admins and members are never considered "new"
	if acc.Admin || acc.Member {
		return false
	}

	return time.Since(acc.Created) < 24*time.Hour
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
	rawToken := base64.URLEncoding.EncodeToString(tokenBytes)

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

	// Check all tokens to find a match
	for _, token := range tokens {
		// Check if token matches (compare hash)
		err := bcrypt.CompareHashAndPassword([]byte(token.Token), []byte(rawToken))
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
