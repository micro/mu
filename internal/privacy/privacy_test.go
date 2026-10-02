package privacy

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	gmai "go-micro.dev/v6/model"
)

func session() *Session { return From(With(context.Background())) }

func TestRoundTripAndIsolation(t *testing.T) {
	a, b := session(), session()
	a.Remember("Jane Doe")
	original := "Jane Doe, Jane Doe: jane+calendar@example.com"
	masked := a.Protect(original)
	if strings.Contains(masked, "Jane") || strings.Contains(masked, "example.com") {
		t.Fatal(masked)
	}
	if got := a.Restore(masked); got != original {
		t.Fatal(got)
	}
	if got := b.Restore(masked); got != masked {
		t.Fatal("another run restored private data")
	}
	if a.Protect(original) != masked {
		t.Fatal("mapping changed within run")
	}
	if b.Protect(original) == masked {
		t.Fatal("mapping reused across runs")
	}
	if a.Protect(masked) != masked {
		t.Fatal("double filtering damaged placeholders")
	}
	if a.Protect("Jane Does") != "Jane Does" {
		t.Fatal("name replaced within a word")
	}
}

func TestSecretsCannotBeRestored(t *testing.T) {
	s := session()
	s.Forget("opaque-owner-secret")
	for _, raw := range []string{
		"password=correct-horse", `{"refresh_token":"opaque-google-refresh"}`,
		"ya29.exampleAccessToken", "1//exampleRefreshToken", "Bearer private-token",
		"sk-123456789012345678901234", "opaque-owner-secret",
		"-----BEGIN PRIVATE KEY-----\nprivate key material\n-----END PRIVATE KEY-----",
	} {
		s.Remember(raw)
		got := s.Value(raw, false).(string)
		if got == raw || !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("not redacted: %q", got)
		}
		if s.Restore(got) != got {
			t.Fatalf("secret restored: %q", raw)
		}
	}
	value := map[string]any{"accessToken": "opaque-secret", "nested": map[string]string{"api_key": "another-secret"}}
	data, _ := json.Marshal(s.Value(value, false))
	if strings.Contains(string(data), "secret") {
		t.Fatal(string(data))
	}
}

func TestMailFilteringAndCalendar(t *testing.T) {
	s := session()
	raw := map[string]any{
		"from": "Jane Doe <jane@example.com>", "to": "Alex Smith <alex@example.com>",
		"subject": "Jane Doe meeting", "text": "Let's meet tomorrow.\r\n> old private quote\r\n-- \r\nPrivate signature",
		"id": "message123", "thread_id": "thread123", "url": "https://mail.google.com/mail/u/0/#all/thread123",
	}
	filtered := s.Value(s.Mail(raw), false)
	data, _ := json.Marshal(filtered)
	for _, forbidden := range []string{"Jane Doe", "Alex Smith", "example.com", "old private quote", "Private signature", "message123", "thread123"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("leaked %s: %s", forbidden, data)
		}
	}
	restored := s.Value(filtered, true).(map[string]any)
	if restored["id"] != raw["id"] || restored["url"] != raw["url"] || restored["text"] != "Let's meet tomorrow." {
		t.Fatal(restored)
	}
	if raw["text"] == restored["text"] {
		t.Fatal("modified stored mail")
	}
	calendar := map[string]any{"title": "Meeting with Jane Doe", "start": "2026-10-02T10:00:00Z", "location": "42 Private Lane", "url": "https://calendar.google.com/private"}
	protected := s.Value(s.Calendar(calendar), false).(map[string]any)
	if protected["location"] == calendar["location"] || protected["start"] != calendar["start"] {
		t.Fatal(protected)
	}
}

func TestNestedJSONAndLargeIDs(t *testing.T) {
	s := session()
	raw := `{"content":[{"text":"jane@example.com"}],"id":9223372036854775807}`
	masked := s.Value(raw, false).(string)
	if strings.Contains(masked, "jane@example.com") || !strings.Contains(masked, "9223372036854775807") {
		t.Fatal(masked)
	}
	if s.Value(masked, true).(string) != raw {
		t.Fatal(s.Value(masked, true))
	}
	typed := []map[string]string{{"text": "jane@example.com"}}
	b, _ := json.Marshal(s.Value(typed, false))
	if strings.Contains(string(b), "jane@example.com") {
		t.Fatal(string(b))
	}
}

func TestStreamingEverySplit(t *testing.T) {
	s := session()
	raw := "Email jane@example.com, then jane@example.com."
	masked := s.Protect(raw)
	for i := 0; i <= len(masked); i++ {
		text := s.Text()
		got := text.Write(masked[:i]) + text.Write(masked[i:]) + text.Flush()
		if got != raw {
			t.Fatalf("split %d: %q", i, got)
		}
	}
	text := s.Text()
	var out strings.Builder
	for _, c := range masked {
		out.WriteString(text.Write(string(c)))
	}
	out.WriteString(text.Flush())
	if out.String() != raw {
		t.Fatal(out.String())
	}
}

func TestConcurrentResults(t *testing.T) {
	s := session()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw := "jane@example.com"
			if s.Restore(s.Protect(raw)) != raw {
				t.Error("broken concurrent mapping")
			}
		}()
	}
	wg.Wait()
}

func TestPrivateLinksAndRevokedMapping(t *testing.T) {
	s := session()
	link := "https://docs.google.com/document/d/private-document/edit"
	masked := s.Protect("Read " + link)
	if strings.Contains(masked, "private-document") || s.Restore(masked) != "Read "+link {
		t.Fatal(masked)
	}
	s.Remember("opaque-secret")
	token := s.Protect("opaque-secret")
	s.Forget("opaque-secret")
	s.Remember("another-value")
	if s.Restore(token) != token {
		t.Fatal("revoked mapping restored or reused")
	}
	file := map[string]any{"id": "private-file", "webViewLink": link, "name": "Report"}
	filtered := s.Value(s.Drive(file), false).(map[string]any)
	if filtered["id"] == file["id"] || filtered["name"] != "Report" {
		t.Fatal(filtered)
	}
}

func TestStructuredReplyEscaping(t *testing.T) {
	s := session()
	name := `Jane "Junior" Doe`
	s.Remember(name)
	r := response(s, &gmai.Response{Reply: `{"name":"` + s.Protect(name) + `"}`})
	var decoded map[string]string
	if err := json.Unmarshal([]byte(r.Reply), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["name"] != name {
		t.Fatal(decoded)
	}
}
