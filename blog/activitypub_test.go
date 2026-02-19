package blog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"mu/auth"
)

func TestAPDomain(t *testing.T) {
	// Save and restore env
	origMU := os.Getenv("MU_DOMAIN")
	origMail := os.Getenv("MAIL_DOMAIN")
	defer func() {
		os.Setenv("MU_DOMAIN", origMU)
		os.Setenv("MAIL_DOMAIN", origMail)
	}()

	// Test MU_DOMAIN takes priority
	os.Setenv("MU_DOMAIN", "example.com")
	os.Setenv("MAIL_DOMAIN", "mail.example.com")
	if got := APDomain(); got != "example.com" {
		t.Errorf("APDomain() = %q, want %q", got, "example.com")
	}

	// Test MAIL_DOMAIN fallback
	os.Setenv("MU_DOMAIN", "")
	if got := APDomain(); got != "mail.example.com" {
		t.Errorf("APDomain() = %q, want %q", got, "mail.example.com")
	}

	// Test localhost default
	os.Setenv("MAIL_DOMAIN", "")
	if got := APDomain(); got != "localhost" {
		t.Errorf("APDomain() = %q, want %q", got, "localhost")
	}
}

func TestAPBaseURL(t *testing.T) {
	origMU := os.Getenv("MU_DOMAIN")
	defer os.Setenv("MU_DOMAIN", origMU)

	os.Setenv("MU_DOMAIN", "mu.xyz")
	if got := apBaseURL(); got != "https://mu.xyz" {
		t.Errorf("apBaseURL() = %q, want %q", got, "https://mu.xyz")
	}

	os.Setenv("MU_DOMAIN", "localhost")
	if got := apBaseURL(); got != "http://localhost:8080" {
		t.Errorf("apBaseURL() = %q, want %q", got, "http://localhost:8080")
	}
}

func TestWantsActivityPub(t *testing.T) {
	tests := []struct {
		accept string
		want   bool
	}{
		{"application/activity+json", true},
		{"application/ld+json", true},
		{"application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"", true},
		{"text/html", false},
		{"application/json", false},
		{"", false},
	}

	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		if tt.accept != "" {
			r.Header.Set("Accept", tt.accept)
		}
		if got := WantsActivityPub(r); got != tt.want {
			t.Errorf("WantsActivityPub(Accept: %q) = %v, want %v", tt.accept, got, tt.want)
		}
	}
}

func TestWebFingerHandler_MissingResource(t *testing.T) {
	r := httptest.NewRequest("GET", "/.well-known/webfinger", nil)
	w := httptest.NewRecorder()

	WebFingerHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWebFingerHandler_InvalidFormat(t *testing.T) {
	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=http://example.com", nil)
	w := httptest.NewRecorder()

	WebFingerHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWebFingerHandler_InvalidAcct(t *testing.T) {
	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:noatsign", nil)
	w := httptest.NewRecorder()

	WebFingerHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWebFingerHandler_WrongDomain(t *testing.T) {
	origMU := os.Getenv("MU_DOMAIN")
	defer os.Setenv("MU_DOMAIN", origMU)
	os.Setenv("MU_DOMAIN", "mu.xyz")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:alice@wrong.com", nil)
	w := httptest.NewRecorder()

	WebFingerHandler(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestInboxHandler_MethodNotAllowed(t *testing.T) {
	r := httptest.NewRequest("GET", "/@alice/inbox", nil)
	w := httptest.NewRecorder()

	InboxHandler(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestInboxHandler_POST(t *testing.T) {
	r := httptest.NewRequest("POST", "/@alice/inbox", nil)
	w := httptest.NewRecorder()

	InboxHandler(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestPostToObject(t *testing.T) {
	post := &Post{
		ID:        "123",
		Title:     "Test Post",
		Content:   "Hello world",
		Author:    "Alice",
		AuthorID:  "alice",
		Tags:      "Tech, Dev",
		CreatedAt: parseTime("2024-01-01T00:00:00Z"),
	}
	acc := &auth.Account{
		ID:   "alice",
		Name: "Alice",
	}

	obj := postToObject("https://mu.xyz", acc, post)

	// Check basic fields
	if obj["type"] != "Note" {
		t.Errorf("type = %v, want Note", obj["type"])
	}
	if obj["id"] != "https://mu.xyz/post?id=123" {
		t.Errorf("id = %v, want https://mu.xyz/post?id=123", obj["id"])
	}
	if obj["attributedTo"] != "https://mu.xyz/@alice" {
		t.Errorf("attributedTo = %v, want https://mu.xyz/@alice", obj["attributedTo"])
	}
	if obj["name"] != "Test Post" {
		t.Errorf("name = %v, want Test Post", obj["name"])
	}

	// Check tags
	tags, ok := obj["tag"].([]map[string]interface{})
	if !ok || len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", obj["tag"])
	}

	// Check to (public)
	to, ok := obj["to"].([]string)
	if !ok || len(to) != 1 || to[0] != "https://www.w3.org/ns/activitystreams#Public" {
		t.Errorf("to = %v, want public addressing", obj["to"])
	}

	// Verify content is rendered HTML
	content, ok := obj["content"].(string)
	if !ok || content == "" {
		t.Errorf("content should be non-empty rendered HTML")
	}

	// Verify it serializes to valid JSON
	_, err := json.Marshal(obj)
	if err != nil {
		t.Errorf("failed to marshal: %v", err)
	}
}

func TestPostToObject_NoTitle(t *testing.T) {
	post := &Post{
		ID:        "456",
		Content:   "No title post",
		Author:    "Bob",
		AuthorID:  "bob",
		CreatedAt: parseTime("2024-06-15T12:00:00Z"),
	}
	acc := &auth.Account{ID: "bob", Name: "Bob"}

	obj := postToObject("https://mu.xyz", acc, post)

	if _, hasName := obj["name"]; hasName {
		t.Error("name should not be set for posts without title")
	}
	if _, hasTags := obj["tag"]; hasTags {
		t.Error("tag should not be set for posts without tags")
	}
}

func parseTime(s string) (t time.Time) {
	t, _ = time.Parse(time.RFC3339, s)
	return
}
