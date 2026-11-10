package mucp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mu/internal/api"
	"strings"
	"sync"
	"time"
)

var pageKey = sync.OnceValue(func() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
})

type cursor struct {
	Scope   string
	Offset  int
	Expires int64
}

// Page uses live, deterministically ordered lists. Tokens bind account, operation and filter.
func Page[T any](items []T, size int32, token, scope string) ([]T, string, error) {
	if size == 0 {
		size = 25
	}
	if size < 1 || size > 100 {
		return nil, "", api.Fail(400, "invalid_arguments", "Page size must be 1–100")
	}
	sum := sha256.Sum256([]byte(scope))
	scope = fmt.Sprintf("%x", sum)
	offset := 0
	if token != "" {
		parts := strings.Split(token, ".")
		if len(parts) != 2 || len(token) > 2048 {
			return nil, "", api.Fail(400, "invalid_arguments", "Invalid page token")
		}
		b, e := base64.RawURLEncoding.DecodeString(parts[0])
		sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
		mac := hmac.New(sha256.New, pageKey())
		mac.Write(b)
		var c cursor
		if e != nil || e2 != nil || !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(b, &c) != nil || c.Scope != scope || c.Expires < time.Now().Unix() || c.Offset < 0 {
			return nil, "", api.Fail(400, "invalid_arguments", "Invalid or expired page token")
		}
		offset = c.Offset
	}
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + int(size)
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		b, _ := json.Marshal(cursor{scope, end, time.Now().Add(15 * time.Minute).Unix()})
		mac := hmac.New(sha256.New, pageKey())
		mac.Write(b)
		next = base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	return items[offset:end], next, nil
}
