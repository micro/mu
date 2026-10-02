// Package privacy minimizes text sent to models. Mappings live only for a run;
// this is pseudonymization, not encryption or a guarantee of anonymity.
package privacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type key struct{}

type Session struct {
	mu      sync.Mutex
	prefix  string
	values  map[string]string
	secrets []string
	next    int
}

// With starts a fresh mapping even if ctx belongs to another run.
func With(ctx context.Context) context.Context {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(err)
	}
	return context.WithValue(ctx, key{}, &Session{prefix: "__MU_" + hex.EncodeToString(nonce[:]) + "_", values: map[string]string{}})
}

func From(ctx context.Context) *Session { s, _ := ctx.Value(key{}).(*Session); return s }

func (s *Session) Remember(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(value) != "" {
		if s.redact(value) == value {
			s.alias(value)
		}
	}
}

// Forget prevents a known credential from acquiring a reversible mapping.
func (s *Session) Forget(value string) {
	if value == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets = append(s.secrets, value)
	for original := range s.values {
		if strings.Contains(original, value) {
			delete(s.values, original)
		}
	}
}

func (s *Session) alias(value string) string {
	if token, ok := s.values[value]; ok {
		return token
	}
	s.next++
	token := fmt.Sprintf("%s%d__", s.prefix, s.next)
	s.values[value] = token
	return token
}

var email = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}`)
var googleLink = regexp.MustCompile(`https://(?:mail|calendar|docs|drive)\.google\.com/[^\s<>"')\]]+`)
var credentials = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z ]*PRIVATE KEY)-----.*?-----END (?:[A-Z ]*PRIVATE KEY)-----`),
	regexp.MustCompile(`\b(?:ya29\.[a-zA-Z0-9_-]+|1//[a-zA-Z0-9_-]+|sk-[a-zA-Z0-9_-]{16,}|gh[pousr]_[a-zA-Z0-9]{20,}|github_pat_[a-zA-Z0-9_]+)\b`),
	regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/-]+=*`),
	regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\b`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|api[_ -]?key|access[_ -]?token|refresh[_ -]?token|client[_ -]?secret)["']?\s*[:=]\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`),
}

func secretKey(k string) bool {
	k = strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(k))
	switch k {
	case "password", "passwd", "secret", "clientsecret", "apikey", "accesstoken", "refreshtoken", "authorization", "privatekey":
		return true
	}
	return false
}

func (s *Session) redact(v string) string {
	for _, secret := range s.secrets {
		v = strings.ReplaceAll(v, secret, "[REDACTED]")
	}
	for _, pattern := range credentials {
		v = pattern.ReplaceAllString(v, "[REDACTED]")
	}
	return v
}

func (s *Session) text(v string) string {
	v = s.redact(v)
	v = googleLink.ReplaceAllStringFunc(v, s.alias)
	// Replace known names before addresses, longest first so overlapping names
	// cannot expose a suffix. Word boundaries avoid changing ordinary substrings.
	values := make([]string, 0, len(s.values))
	for value := range s.values {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		var out strings.Builder
		rest := v
		for {
			i := strings.Index(rest, value)
			if i < 0 {
				out.WriteString(rest)
				break
			}
			before, _ := utf8.DecodeLastRuneInString(rest[:i])
			after, _ := utf8.DecodeRuneInString(rest[i+len(value):])
			out.WriteString(rest[:i])
			if !word(before) && !word(after) {
				out.WriteString(s.values[value])
			} else {
				out.WriteString(value)
			}
			rest = rest[i+len(value):]
		}
		v = out.String()
	}
	return email.ReplaceAllStringFunc(v, s.alias)
}

func (s *Session) Protect(v string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(v)
}

func (s *Session) Restore(v string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	// One pass: original text containing another token is not recursively expanded.
	pairs := make([]string, 0, len(s.values)*2)
	for value, token := range s.values {
		pairs = append(pairs, token, value)
	}
	return strings.NewReplacer(pairs...).Replace(v)
}

// Value copies JSON-shaped content, preserving numeric types and field names.
// JSON inside text (common for tool results) is treated as structured data too.
func (s *Session) Value(v any, restore bool) any {
	switch x := v.(type) {
	case string:
		var data any
		if (strings.HasPrefix(strings.TrimSpace(x), "{") || strings.HasPrefix(strings.TrimSpace(x), "[")) && decode(x, &data) == nil {
			b, _ := json.Marshal(s.Value(data, restore))
			return string(b)
		}
		if restore {
			return s.Restore(x)
		}
		return s.Protect(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			if !restore && secretKey(k) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = s.Value(value, restore)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = s.Value(value, restore)
		}
		return out
	default:
		if v != nil {
			kind := reflect.TypeOf(v).Kind()
			if kind == reflect.Struct || kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array || kind == reflect.Pointer {
				b, err := json.Marshal(v)
				var data any
				if err == nil && decode(string(b), &data) == nil {
					return s.Value(data, restore)
				}
				// Unserializable content must not bypass filtering.
				return "[REDACTED]"
			}
		}
		return v
	}
}

func word(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
func decode(text string, value *any) error {
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

// Mail removes standard reply tails/signatures and learns display names from
// address headers before masking subjects and bodies. It only touches the
// model's copy, never the stored mail or the service API response.
func (s *Session) Mail(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"from", "to", "cc", "bcc", "reply_to"} {
			if header, ok := x[k].(string); ok {
				if addresses, err := mail.ParseAddressList(header); err == nil {
					for _, a := range addresses {
						s.Remember(a.Name)
						s.Remember(a.Address)
					}
				}
			}
		}
		out := make(map[string]any, len(x))
		for k, value := range x {
			if k == "snippet" {
				continue
			} // Read the selected body, not a duplicate preview.
			if str, ok := value.(string); ok {
				switch k {
				case "text", "body":
					value = trimMail(str)
				case "id", "thread_id", "url", "next_page":
					s.Remember(str)
				}
			}
			out[k] = s.Mail(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = s.Mail(value)
		}
		return out
	default:
		return v
	}
}

func trimMail(v string) string {
	lines := strings.Split(strings.ReplaceAll(v, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if line == "-- " || trimmed == "-----Original Message-----" || (strings.HasPrefix(trimmed, "On ") && strings.HasSuffix(trimmed, " wrote:")) {
			break
		}
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Calendar keeps useful titles and times while hiding links and exact locations.
// Availability itself comes from the Free tool, which contains no event details.
func (s *Session) Calendar(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if k == "url" || k == "location" {
				if str, ok := value.(string); ok {
					s.Remember(str)
				}
			}
			s.Calendar(value)
		}
	case []any:
		for _, value := range x {
			s.Calendar(value)
		}
	}
	return v
}

// Drive keeps document content useful while hiding persistent file identifiers.
func (s *Session) Drive(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if k == "id" || k == "webViewLink" || k == "nextPageToken" {
				if str, ok := value.(string); ok {
					s.Remember(str)
				}
			}
			s.Drive(value)
		}
	case []any:
		for _, value := range x {
			s.Drive(value)
		}
	}
	return v
}
