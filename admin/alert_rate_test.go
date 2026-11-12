package admin

import (
	"mu/internal/usage"
	"strings"
	"testing"
)

func TestRateRecoveryNeedsHeadroom(t *testing.T) {
	for _, n := range []int{800, 999, 1000, 1100} {
		if rateRecovered(n, 1000) {
			t.Fatalf("recovered too early at %d", n)
		}
	}
	if !rateRecovered(799, 1000) {
		t.Fatal("did not recover below 80 percent")
	}
	if !rateRecovered(0, 1) || rateRecovered(1, 1) {
		t.Fatal("small threshold recovery")
	}
}
func TestAccountRateDetailUsesOwnHourlyEndpoints(t *testing.T) {
	const account = "alert_rate_detail_fixture"
	usage.Record("web", "/inbox", account)
	usage.Record("web", "/inbox", account)
	usage.Record("web", "/agent", "alert_rate_other_fixture")
	detail := accountRateDetail(account)
	if !strings.Contains(detail, "/inbox: 2") || strings.Contains(detail, "/agent") {
		t.Fatal(detail)
	}
}
