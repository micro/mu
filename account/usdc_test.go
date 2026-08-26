package account

import (
	"math/big"
	"os"
	"strings"
	"testing"
)

// What somebody types, and where it stops.
func TestConvertAmountBounds(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    int
		wantErr string
	}{
		{in: "5", want: 500},
		{in: " 1 ", want: 100},
		{in: "500", want: 50000},
		{in: "", wantErr: "whole dollars"},
		{in: "abc", wantErr: "whole dollars"},
		{in: "0", wantErr: "whole dollars"},
		{in: "-3", wantErr: "whole dollars"},
		{in: "501", wantErr: "largest"},
	} {
		got, err := convertAmount(tt.in)
		if tt.wantErr != "" {
			if err == nil {
				t.Errorf("convertAmount(%q) = %d, want refused", tt.in, got)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("convertAmount(%q) said %q, want something about %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("convertAmount(%q): %v", tt.in, err)
		} else if got != tt.want {
			t.Errorf("convertAmount(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// The rate is one, and this is the only place that could get it wrong.
//
// A credit is a cent, USDC carries six decimals, so a credit is 10,000 atomic
// units and a dollar is 1,000,000. If this drifts from what x402 signs, the
// pre-flight check refuses conversions that would have worked or waves through
// ones that cannot settle.
func TestACreditIsACentInUSDC(t *testing.T) {
	if got := creditsAsUSDCAtomic(1); got.Cmp(big.NewInt(10000)) != 0 {
		t.Errorf("one credit is %s atomic units, want 10000", got)
	}
	if got := creditsAsUSDCAtomic(100); got.Cmp(big.NewInt(1000000)) != 0 {
		t.Errorf("a dollar is %s atomic units, want 1000000", got)
	}
	// And it is the same number x402 builds the requirement from, which is what
	// the facilitator is actually paid.
	if got := creditsAsUSDCAtomic(250); got.Cmp(big.NewInt(2500000)) != 0 {
		t.Errorf("$2.50 is %s atomic units, want 2500000", got)
	}
}

// An instance that takes no USDC offers no conversion, rather than a form that
// can only fail.
func TestNoUSDCMeansNoConversion(t *testing.T) {
	_, _, err := convertUSDC("somebody", 500)
	if err == nil {
		t.Fatal("converted on an instance with no X402_PAY_TO")
	}
	if !strings.Contains(err.Error(), "does not take USDC") {
		t.Errorf("unhelpful refusal: %v", err)
	}
}

// A settled transfer is credited once, keyed on the transaction.
//
// Not reachable through a test — it needs a facilitator and a chain — so this
// reads the source instead. The property is worth a weaker guard than none: a
// conversion credited against the attempt rather than the transaction double-
// credits on a resubmitted form, a retried browser, or two tabs, and the only
// symptom is somebody quietly getting more credits than they paid for.
//
// Function definitions are stripped before searching, because a check for
// "CreditOnce(" would otherwise be satisfied by the line that declares it.
func TestAConversionIsCreditedOncePerTransaction(t *testing.T) {
	src := sourceWithoutDefinitions(t, "usdc.go")

	if !strings.Contains(src, `CreditOnce(accountID, credits, "topup_usdc", tx`) {
		t.Error("the conversion does not credit against the transaction hash, so a " +
			"retry can credit the same transfer twice")
	}
	if strings.Contains(src, "AddCredits(") {
		t.Error("the conversion uses AddCredits, which has no settlement key")
	}
	// And it refuses to credit at all when there is no key to settle against.
	if !strings.Contains(src, `if tx == ""`) {
		t.Error("a settlement with no transaction id is credited anyway, and cannot " +
			"be recognised as already paid")
	}
}

// sourceWithoutDefinitions is a file with its func declaration lines removed.
func sourceWithoutDefinitions(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "func ") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}
