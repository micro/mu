package server

import (
	"encoding/json"
	"fmt"
	"mu/internal/auth"
	"mu/service/alerts"
	"mu/service/bookmarks"
	"strings"
	"testing"
	"time"
)

func TestRecordServiceContracts(t *testing.T) {
	bookmarks.Load()
	alerts.Load()
	for _, tc := range []struct{ name, create, body, mutate string }{
		{"bookmarks", "Bookmarks.Add", `{"url":"https://example.com/contract","title":"private contract record","note":"private note"}`, "Bookmarks.Delete"},
		{"alerts", "Alerts.Raise", `{"key":"contract","title":"private contract record","detail":"private detail"}`, "Alerts.Update"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, other := tc.name+"_contract_owner", tc.name+"_contract_other"
			tokens := map[string]string{}
			for _, id := range []string{owner, other} {
				auth.SetAccountForTest(&auth.Account{ID: id, Admin: true})
				t.Cleanup(func() { auth.RemoveAccountForTest(id) })
				_, token, err := auth.CreateToken(id, "contract", []string{"read", "write", "service:" + tc.name}, time.Time{})
				if err != nil {
					t.Fatal(err)
				}
				tokens[id] = token
			}
			t.Cleanup(func() { bookmarks.DeleteAll(owner); alerts.DeleteAll(owner) })
			r := compositionCall(tokens[owner], tc.name, tc.create, tc.body)
			if r.Code != 200 {
				t.Fatal(r.Code, r.Body.String())
			}
			var result struct {
				Item struct {
					ID string `json:"id"`
				}
			}
			if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil || result.Item.ID == "" {
				t.Fatal(err, r.Body.String())
			}
			endpoint := strings.Split(tc.create, ".")[0] + ".List"
			r = compositionCall(tokens[owner], tc.name, endpoint, `{"offset":"0","limit":"20"}`)
			if r.Code != 200 || !strings.Contains(r.Body.String(), "private contract record") {
				t.Fatal(r.Code, r.Body.String())
			}
			r = compositionCall(tokens[other], tc.name, endpoint, `{}`)
			if r.Code != 200 || strings.Contains(r.Body.String(), "private contract record") {
				t.Fatal(r.Code, r.Body.String())
			}
			body := fmt.Sprintf(`{"id":%q}`, result.Item.ID)
			if tc.name == "alerts" {
				body = fmt.Sprintf(`{"id":%q,"state":"resolved"}`, result.Item.ID)
			}
			for _, scopes := range [][]string{{"read", "service:" + tc.name}, {"read", "write", "api:agent"}} {
				_, token, err := auth.CreateToken(owner, "limited", scopes, time.Time{})
				if err != nil {
					t.Fatal(err)
				}
				r = compositionCall(token, tc.name, tc.mutate, body)
				if r.Code != 403 {
					t.Fatal(r.Code, r.Body.String())
				}
			}
			r = compositionCall(tokens[owner], tc.name, endpoint, `{"account":"other"}`)
			if r.Code != 400 {
				t.Fatal(r.Code, r.Body.String())
			}
			r = compositionCall(tokens[owner], tc.name, tc.mutate, body)
			if r.Code != 200 {
				t.Fatal(r.Code, r.Body.String())
			}
		})
	}
}
