package account

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"mu/internal/data"
)

type revokeTransport func(*http.Request) (*http.Response, error)

func (f revokeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRetirementRemovesLegacyCredentialsAndRevokesEachGrantOnce(t *testing.T) {
	for _, failure := range []bool{false, true} {
		for _, key := range []string{"google_connections.json", "google_connections.json.prev"} {
			if err := data.SaveJSON(key, []map[string]string{{"refresh_token": "retired-test-token"}}); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		client := &http.Client{Transport: revokeTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || r.URL.String() != "https://oauth2.googleapis.com/revoke" {
				t.Fatalf("unexpected revocation request: %s %s", r.Method, r.URL)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "token=retired-test-token" {
				t.Fatal("credential missing from body")
			}
			if failure {
				return nil, errors.New("offline")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		})}
		err := retireGoogleGrantsWith(client)
		if (err != nil) != failure {
			t.Fatalf("failure=%v: %v", failure, err)
		}
		if calls != 1 {
			t.Fatalf("duplicate grant revoked %d times", calls)
		}
		for _, key := range []string{"google_connections.json", "google_connections.json.prev"} {
			if _, err := data.LoadFile(key); !os.IsNotExist(err) {
				t.Fatalf("legacy credentials remain: %s: %v", key, err)
			}
		}
		if err := retireGoogleGrantsWith(client); err != nil || calls != 1 {
			t.Fatal("retirement repeated after cleanup")
		}
	}
}
