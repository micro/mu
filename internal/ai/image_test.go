package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"mu/internal/app"
)

type imageTestTransport func(*http.Request) (*http.Response, error)

func (f imageTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestImageRequestsRecordOutcomes(t *testing.T) {
	previous := imageHTTPClient
	t.Cleanup(func() { imageHTTPClient = previous })
	for _, tc := range []struct {
		name   string
		method string
		status int
		body   string
		failed bool
	}{
		{"submitted", "POST", 200, `{"code":200,"data":{"id":"prediction"}}`, false},
		{"pending", "GET", 200, `{"data":{"status":"processing"}}`, false},
		{"completed", "GET", 200, `{"data":{"status":"completed","outputs":["https://private/image"]}}`, false},
		{"denied", "POST", 200, `{"code":403,"msg":"Upstream access denied, please contact administrator."}`, true},
		{"poll denied", "GET", 403, `{"msg":"Access denied"}`, true},
		{"failed", "GET", 200, `{"data":{"status":"failed","error":"private-prompt Bearer test-key"}}`, true},
		{"invalid JSON", "POST", 200, "private-prompt test-key", true},
		{"missing output", "GET", 200, `{"data":{"status":"completed"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imageHTTPClient = &http.Client{Transport: imageTestTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			req, _ := http.NewRequestWithContext(context.Background(), tc.method, atlasImageBase+"/test", nil)
			_, err := imageRequest(req, "test-key", "private-prompt")
			if (err != nil) != tc.failed {
				t.Fatalf("error = %v, want failure %v", err, tc.failed)
			}
			e := app.APILog()[0]
			if e.Service != "images" || e.Status != tc.status || (e.Error != "") != tc.failed {
				t.Fatalf("unexpected log: %+v", e)
			}
			for _, secret := range []string{"test-key", "private-prompt", "https://private/image"} {
				if strings.Contains(fmt.Sprintf("%+v", e), secret) {
					t.Fatalf("log retained private data: %s", secret)
				}
			}
		})
	}
	imageHTTPClient = &http.Client{Transport: imageTestTransport(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection failed")
	})}
	req, _ := http.NewRequest("POST", atlasImageBase+"/test", nil)
	if _, err := imageRequest(req, "", ""); err == nil {
		t.Fatal("expected transport failure")
	}
	if e := app.APILog()[0]; e.ErrorKind != "transport" || e.Status != 0 {
		t.Fatalf("transport failure not recorded: %+v", e)
	}
}
