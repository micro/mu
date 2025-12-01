package auth

import (
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

// User presence tracking
var presenceMutex sync.RWMutex
var userPresence = map[string]time.Time{} // username -> last seen time

type Account struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Secret   string    `json:"secret"`
	Created  time.Time `json:"created"`
	Admin    bool      `json:"admin"`
	Member   bool      `json:"member"`
	Language string    `json:"language"`
}

type Session struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Token   string    `json:"token"`
	Account string    `json:"account"`
	Created time.Time `json:"created"`
}

func init() {
	b, _ := data.LoadFile("accounts.json")
	json.Unmarshal(b, &accounts)
	b, _ = data.LoadFile("sessions.json")
	json.Unmarshal(b, &sessions)
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
	c, err := r.Cookie("session")
	if err != nil {
		return nil, err
	}

	if c == nil {
		return nil, errors.New("session not found")
	}

	sess, err := ParseToken(c.Value)
	if err != nil {
		return nil, err
	}

	return sess, nil
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

	sess, err := ParseToken(tk)
	if err != nil {
		return err
	}
	if sess.Type != "account" {
		return errors.New("invalid session")
	}
	return nil
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

// GetOnlineCount returns the number of online users
func GetOnlineCount() int {
	return len(GetOnlineUsers())
}
