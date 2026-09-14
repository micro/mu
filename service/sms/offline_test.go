package sms

import (
	"errors"
	"net/http"
	"os"
	"testing"
)

// SMS tests exercise validation and durable records, never a live provider.
// Even tests of the successful validation path must not send a real message.
func TestMain(m *testing.M) {
	http.DefaultTransport = offlineTransport{}
	os.Exit(m.Run())
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("outbound provider requests are disabled in SMS tests")
}
