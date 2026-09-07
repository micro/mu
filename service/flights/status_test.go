package flights

import (
	"context"
	"mu/internal/quota"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusToolLeavesBillingToGateway(t *testing.T) {
	t.Setenv("AVIATIONSTACK_API_KEY", "test-only")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "20" || r.URL.Query().Get("flight_iata") != "BA117" {
			t.Errorf("wrong request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"flight":{"iata":"BA117"},"arrival":{"scheduled":"2026-09-07T12:00:00+00:00","estimated":null}}]}`))
	}))
	defer upstream.Close()
	oldURL := statusURL
	statusURL = upstream.URL
	t.Cleanup(func() { statusURL = oldURL })
	enabled, balance, deduct := quota.Enabled, quota.Balance, quota.Deduct
	quota.Enabled = func() bool { return true }
	quota.Balance = func(string) int { t.Error("tool checked an account outside gateway"); return 0 }
	quota.Deduct = func(string, string, int, map[string]interface{}) error {
		t.Error("tool charged outside gateway")
		return nil
	}
	t.Cleanup(func() { quota.Enabled, quota.Balance, quota.Deduct = enabled, balance, deduct })
	var result StatusResponse
	if err := (Server{}).Status(context.Background(), &StatusRequest{Flight: "BA117"}, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Flights) != 1 || result.Flights[0].Arrival.Estimated != "" {
		t.Fatalf("missing estimate invented: %+v", result)
	}
}
