package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStripeFailureKeepsOnlySafeDiagnostics(t *testing.T) {
	subscriptionFixture(t)
	billingHTTP = &http.Client{Transport: stripeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{"Request-Id": {"req_checkout"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"permission_denied","param":"enabled_events","message":"secret_request_value"}}`))}, nil
	})}
	err := ensureSubscriptionWebhook(context.Background(), "https://micro.test")
	var failure *stripeAPIError
	if !errors.Is(err, errSubscriptionWebhook) || !errors.As(err, &failure) || failure.Status != 403 || failure.RequestID != "req_checkout" || failure.Code != "permission_denied" {
		t.Fatalf("missing checkout diagnostics: %v", err)
	}
	if strings.Contains(err.Error(), "secret_request_value") {
		t.Fatal("echoed request values in error")
	}
}
