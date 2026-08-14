package account

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondTransferErrorEscapesRedirectMessage(t *testing.T) {
	req := httptest.NewRequest("POST", "/account/transfer", nil)
	rr := httptest.NewRecorder()

	respondTransferError(rr, req, "insufficient balance & retry")

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	loc := rr.Header().Get("Location")
	if loc != "/account/transfer?error=insufficient+balance+%26+retry" {
		t.Fatalf("Location = %q", loc)
	}
}
