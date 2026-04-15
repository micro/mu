// Simple HMAC-protected math captcha used on the signup form.
// No state, no third-party service. The challenge embeds an HMAC of the
// expected answer plus a per-load nonce, so bots have to actually parse
// the question to submit the form.
package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// captchaSecret is generated lazily on first use. We don't persist it;
// regenerating across restarts only invalidates in-flight signup forms.
var (
	captchaSecretOnce sync.Once
	captchaSecret     []byte
)

func getCaptchaSecret() []byte {
	captchaSecretOnce.Do(func() {
		if v := os.Getenv("CAPTCHA_SECRET"); v != "" {
			captchaSecret = []byte(v)
			return
		}
		b := make([]byte, 32)
		rand.Read(b)
		captchaSecret = b
	})
	return captchaSecret
}

// CaptchaChallenge returns a question, nonce, timestamp, and signature
// suitable for embedding in a form. The signature commits to (nonce,
// timestamp, expected answer) so the answer cannot be brute-forced
// without also forging the HMAC.
type CaptchaChallenge struct {
	Question  string // human-readable, e.g. "What is 7 + 4?"
	Nonce     string
	Timestamp string
	Signature string
}

// NewCaptchaChallenge builds a fresh challenge. Numbers are kept small
// and non-zero so the answer is unambiguous.
func NewCaptchaChallenge() CaptchaChallenge {
	a := randInt(2, 12)
	b := randInt(2, 12)
	answer := a + b
	question := fmt.Sprintf("What is %d + %d?", a, b)

	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := captchaSign(nonce, ts, fmt.Sprintf("%d", answer))

	return CaptchaChallenge{
		Question:  question,
		Nonce:     nonce,
		Timestamp: ts,
		Signature: sig,
	}
}

// VerifyCaptcha returns nil on success.
func VerifyCaptcha(answer, nonce, ts, sig string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return fmt.Errorf("captcha required")
	}
	// Reject stale challenges (>15 minutes old).
	if t, err := parseUnix(ts); err != nil || time.Since(t) > 15*time.Minute {
		return fmt.Errorf("captcha expired — please reload the form")
	}
	expected := captchaSign(nonce, ts, answer)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return fmt.Errorf("captcha answer is wrong")
	}
	return nil
}

// CaptchaHTML renders the form fragment for a fresh challenge.
func CaptchaHTML(c CaptchaChallenge) string {
	return fmt.Sprintf(`<div class="form-group">
  <label for="captcha">%s</label>
  <input type="text" id="captcha" name="captcha" inputmode="numeric" pattern="[0-9]*" autocomplete="off" required>
  <input type="hidden" name="captcha_nonce" value="%s">
  <input type="hidden" name="captcha_ts" value="%s">
  <input type="hidden" name="captcha_sig" value="%s">
</div>`, c.Question, c.Nonce, c.Timestamp, c.Signature)
}

// VerifyCaptchaRequest extracts and verifies captcha fields from a form.
func VerifyCaptchaRequest(r *http.Request) error {
	return VerifyCaptcha(
		r.FormValue("captcha"),
		r.FormValue("captcha_nonce"),
		r.FormValue("captcha_ts"),
		r.FormValue("captcha_sig"),
	)
}

func captchaSign(nonce, ts, answer string) string {
	mac := hmac.New(sha256.New, getCaptchaSecret())
	mac.Write([]byte(nonce + "|" + ts + "|" + answer))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randInt(min, max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return int(n.Int64()) + min
}

func parseUnix(s string) (time.Time, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0), nil
}
