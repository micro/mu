package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type contentTransport func(*http.Request) (*http.Response, error)

func (f contentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func contentFixture(t *testing.T, handler func(*http.Request) string) {
	t.Helper()
	reset()
	old := httpClient
	httpClient = &http.Client{Transport: contentTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatalf("read-only integration wrote using %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer reader-token" {
			t.Fatal("wrong account credential")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(handler(r))), Header: make(http.Header)}, nil
	})}
	mu.Lock()
	conns["reader"] = &Connection{AccountID: "reader", RefreshToken: "refresh", Scopes: []string{GmailScope, DriveScope}}
	access["reader"] = cachedToken{token: "reader-token", expires: time.Now().Add(time.Hour)}
	mu.Unlock()
	t.Cleanup(func() { httpClient = old; reset() })
}
func TestContentRequiresTheCallersOwnScope(t *testing.T) {
	calls := 0
	contentFixture(t, func(r *http.Request) string { calls++; return `{}` })
	for _, owner := range []string{"", "other"} {
		if _, err := SearchGmail(context.Background(), owner, "invoice", "", 10); err == nil {
			t.Fatal("unconnected caller read Gmail")
		}
		if _, err := SearchDrive(context.Background(), owner, "invoice", "", 10); err == nil {
			t.Fatal("unconnected caller read Drive")
		}
	}
	mu.Lock()
	conns["reader"].Scopes = []string{CalendarScope}
	mu.Unlock()
	if _, err := ReadGmail(context.Background(), "reader", "message"); err == nil {
		t.Fatal("calendar permission granted mail access")
	}
	if _, err := ReadDrive(context.Background(), "reader", "file"); err == nil {
		t.Fatal("calendar permission granted file access")
	}
	if calls != 0 {
		t.Fatal("unauthorized request reached Google")
	}
}
func TestGmailSearchUsesMetadataAndPreservesPagination(t *testing.T) {
	contentFixture(t, func(r *http.Request) string {
		if r.URL.Path == "/gmail/v1/users/me/messages" {
			if r.URL.Query().Get("q") != "is:unread" || r.URL.Query().Get("pageToken") != "page-two" || r.URL.Query().Get("maxResults") != "20" {
				t.Fatalf("bad search: %s", r.URL)
			}
			return `{"messages":[{"id":"abc123"}],"nextPageToken":"page-three"}`
		}
		if r.URL.Query().Get("format") != "metadata" {
			t.Fatal("search fetched message bodies")
		}
		return `{"id":"abc123","threadId":"thread1","snippet":"Invoice summary","payload":{"headers":[{"name":"Subject","value":"Invoice"},{"name":"From","value":"a@example.com"}]}}`
	})
	got, err := SearchGmail(context.Background(), "reader", "is:unread", "page-two", 500)
	if err != nil || len(got.Messages) != 1 || got.Messages[0].Subject != "Invoice" || got.NextPage != "page-three" {
		t.Fatalf("search: %+v %v", got, err)
	}
}
func TestGmailReadIgnoresAttachments(t *testing.T) {
	contentFixture(t, func(r *http.Request) string {
		if r.URL.Query().Get("format") != "full" {
			t.Fatal("missing body request")
		}
		b, _ := json.Marshal(map[string]any{"id": "abc", "payload": map[string]any{"parts": []any{
			map[string]any{"mimeType": "text/plain", "body": map[string]string{"data": base64.RawURLEncoding.EncodeToString([]byte("A readable message"))}},
			map[string]any{"mimeType": "text/plain", "filename": "secret.txt", "body": map[string]string{"data": base64.RawURLEncoding.EncodeToString([]byte("attachment"))}},
		}}})
		return string(b)
	})
	got, err := ReadGmail(context.Background(), "reader", "abc")
	if err != nil || got.Text != "A readable message" {
		t.Fatalf("read: %+v %v", got, err)
	}
}
func TestDriveSearchEscapesQueryAndReadExportsDocs(t *testing.T) {
	contentFixture(t, func(r *http.Request) string {
		if r.URL.Path == "/drive/v3/files" {
			if r.URL.Query().Get("q") != `trashed = false and fullText contains 'owner\'s notes'` {
				t.Fatalf("query not escaped: %s", r.URL.Query().Get("q"))
			}
			return `{"files":[{"id":"doc1","name":"Notes","mimeType":"application/vnd.google-apps.document"}]}`
		}
		if strings.HasSuffix(r.URL.Path, "/export") {
			if r.URL.Query().Get("mimeType") != "text/plain" {
				t.Fatal("wrong export format")
			}
			return "My document"
		}
		return `{"id":"doc1","name":"Notes","mimeType":"application/vnd.google-apps.document"}`
	})
	if _, err := SearchDrive(context.Background(), "reader", "owner's notes", "", 1); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDrive(context.Background(), "reader", "doc1")
	if err != nil || got.Text != "My document" {
		t.Fatalf("export: %+v %v", got, err)
	}
	for _, id := range []string{"../other", "https://example.com", "a?alt=media", ""} {
		if _, err := ReadDrive(context.Background(), "reader", id); err == nil {
			t.Fatalf("accepted id %q", id)
		}
	}
}
func TestContentResponseIsBounded(t *testing.T) {
	contentFixture(t, func(r *http.Request) string { return strings.Repeat("x", contentLimit+1) })
	if _, err := readAPI(context.Background(), "reader", DriveScope, "https://www.googleapis.com/drive/v3/files"); err == nil {
		t.Fatal("accepted oversized response")
	}
}

func TestHTMLMailIsReadAsTextWithoutScripts(t *testing.T) {
	p := gmailPart{MimeType: "text/html"}
	p.Body.Data = base64.RawURLEncoding.EncodeToString([]byte(`<p>Hello <b>reader</b></p><script>never run this</script><img src="https://example.com/tracker">`))
	if got := htmlParts(p); got != "Hello reader" {
		t.Fatalf("unsafe or missing mail text: %q", got)
	}
}
