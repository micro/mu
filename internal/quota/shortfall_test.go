package quota

import (
	"strings"
	"testing"
)

func TestShortfallDestination(t *testing.T) {
	message := Shortfall(3, 0, "group:family")
	if !strings.Contains(message, "/groups?id=family&tab=settings") || strings.Contains(message, "/account/topup") || strings.Contains(message, "free allowance") {
		t.Fatal(message)
	}
	if message := Shortfall(3, 0, "person"); !strings.Contains(message, "/account/topup") {
		t.Fatal(message)
	}
}
