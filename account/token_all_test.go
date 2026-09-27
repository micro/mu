package account

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestAllAndCombinedClientTokens(t *testing.T) {
	owner := "all_access_test_owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	for _, kind := range []string{"all", "both"} {
		r := httptest.NewRequest("POST", "/account/tokens", strings.NewReader(`{"client":"`+kind+`","name":"test","expires_in":7}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handleCreateToken(w, r, owner)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", kind, w.Code, w.Body.String())
		}
		var response struct {
			Token string `json:"token"`
			ID    string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		defer auth.DeleteToken(response.ID, owner)
		for _, protocol := range []string{"mail", "chat"} {
			if _, err := auth.AccountForToken(owner, response.Token, protocol); err != nil {
				t.Fatalf("%s cannot use %s: %v", kind, protocol, err)
			}
			if _, err := auth.AccountForToken("another-owner", response.Token, protocol); err == nil {
				t.Fatal("cross-account access")
			}
		}
		for _, token := range auth.ListTokens(owner) {
			if token.ID != response.ID {
				continue
			}
			if token.HasPermission("admin") {
				t.Fatal("granted admin")
			}
			if (kind == "all") != !token.Scoped() {
				t.Fatalf("wrong scope for %s", kind)
			}
			if token.AllowsService("notes") != (kind == "all") {
				t.Fatal("incorrect service access")
			}
		}
	}
}
