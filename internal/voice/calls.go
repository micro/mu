package voice

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"sync"
	"time"

	"mu/internal/settings"
)

func CallsConfigured() bool {
	return settings.Get("TWILIO_VOICE_ENABLED") == "true" && settings.Get("TWILIO_VOICE_FROM") != ""
}

type callCode struct {
	code     string
	until    time.Time
	attempts int
}

var codes = struct {
	sync.Mutex
	values map[string]callCode
}{values: map[string]callCode{}}

// NewCode replaces the account's previous code. Codes deliberately expire on restart.
func NewCode(owner string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	codes.Lock()
	defer codes.Unlock()
	for id, c := range codes.values {
		if time.Now().After(c.until) {
			delete(codes.values, id)
		}
	}
	codes.values[owner] = callCode{code: code, until: time.Now().Add(5 * time.Minute)}
	return code, nil
}

// ConsumeCode limits guesses and binds the code to the verified number's owner.
func ConsumeCode(owner, code string) bool {
	codes.Lock()
	defer codes.Unlock()
	c, ok := codes.values[owner]
	if !ok {
		return false
	}
	if time.Now().After(c.until) || c.attempts >= 5 {
		delete(codes.values, owner)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(c.code), []byte(code)) == 1 {
		delete(codes.values, owner)
		return true
	}
	c.attempts++
	codes.values[owner] = c
	return false
}
