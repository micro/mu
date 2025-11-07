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

type Account struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Secret  string    `json:"secret"`
	Created time.Time `json:"created"`
}

type Session struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Token   string    `json:"token"`
	Account string    `json:"account"`
	Created time.Time `json:"created"`
}

func init() {
	b, _ := data.Load("accounts.json")
	json.Unmarshal(b, &accounts)
	b, _ = data.Load("sessions.json")
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

// AccountExists checks if an account with the given ID exists
func AccountExists(id string) bool {
	mutex.Lock()
	defer mutex.Unlock()
	_, exists := accounts[id]
	return exists
}
