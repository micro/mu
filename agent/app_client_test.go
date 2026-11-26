package agent

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func nativeAppRequest(method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://micro.test"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)
	return w
}
func TestAppCustomerTokenBoundary(t *testing.T) {
	a, accounts := appFixture(t)
	makeToken := func(who string, scopes ...string) string {
		_, token, err := auth.CreateToken(accounts[who+"ID"], "native", scopes, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	alice := makeToken("alice", "read", "write", "api:agent")
	bob := makeToken("bob", "read", "write", "api:agent")
	read := makeToken("alice", "read", "api:agent")
	wrong := makeToken("alice", "read", "write", "api:inbox")
	own := thread.Open(accounts["aliceID"], "agent-app", "native-own")
	thread.SetAgent(accounts["aliceID"], own.ID, a.ID)
	foreign := thread.Open(accounts["bobID"], "agent-app", "native-foreign")
	thread.SetAgent(accounts["bobID"], foreign.ID, a.ID)
	personal := thread.Open(accounts["aliceID"], thread.WebClient, "personal")
	thread.SetAgent(accounts["aliceID"], personal.ID, a.ID)
	for _, tc := range []struct {
		method, route, body, token string
		status                     int
	}{
		{"GET", "profile", "", alice, 200}, {"GET", "profile", "", wrong, 403},
		{"GET", "profile", "", "invalid", 401}, {"GET", "sessions", "", read, 200},
		{"POST", "threads", "{}", read, 200},
		{"POST", "history", `{"thread":"` + own.ID + `"}`, alice, 200},
		{"POST", "history", `{"thread":"` + own.ID + `"}`, bob, 404},
		{"POST", "history", `{"thread":"` + personal.ID + `"}`, alice, 404},
		{"POST", "history", `{"thread":"` + foreign.ID + `"}`, alice, 404},
		{"POST", "chat", `{"prompt":"hello"}`, read, 403},
		{"POST", "chat", `{"prompt":"hello","thread":"` + foreign.ID + `"}`, alice, 400},
		{"POST", "settings", "{}", alice, 403}, {"POST", "checkout", "{}", alice, 403},
	} {
		w := nativeAppRequest(tc.method, appPath(a)+tc.route, tc.body, tc.token)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d want %d: %s", tc.method, tc.route, w.Code, tc.status, w.Body.String())
		}
	}
	w := nativeAppRequest("POST", appPath(a)+"threads", "{}", alice)
	var list struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Threads) != 1 || list.Threads[0].ID != own.ID {
		t.Fatal("customer list leaked or omitted threads", w.Body.String())
	}
	a.App.Published = false
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	if w = nativeAppRequest("GET", appPath(a)+"profile", "", alice); w.Code != 404 {
		t.Fatal("unpublished app accessible", w.Code)
	}
}
func TestAppRequestSurface(t *testing.T) {
	for _, path := range []string{"/agent/teacup/app/settings", "/agent/teacup/app/checkout", "/agent/teacup/app/", "/agent/teacup/app/session/id/extra"} {
		if AppRequest(httptest.NewRequest("POST", path, nil)) {
			t.Fatal("unexpected native route", path)
		}
	}
}

func TestAppThreadPaginationIsCustomerScoped(t *testing.T) {
	a, accounts := appFixture(t)
	for i := 0; i < 3; i++ {
		th := thread.Open(accounts["aliceID"], "agent-app", newFlowID())
		thread.SetAgent(accounts["aliceID"], th.ID, a.ID)
	}
	_, token, err := auth.CreateToken(accounts["aliceID"], "native", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, other, err := auth.CreateToken(accounts["bobID"], "native", []string{"read", "api:agent"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	page := ""
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]any{"pageSize": 1, "pageToken": page})
		w := nativeAppRequest("POST", appPath(a)+"threads", string(body), token)
		var response struct {
			Threads []struct {
				ID string `json:"id"`
			} `json:"threads"`
			Next string `json:"nextPageToken"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Threads) != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		id := response.Threads[0].ID
		if seen[id] {
			t.Fatal("duplicate page", id)
		}
		seen[id] = true
		page = response.Next
		if i == 0 {
			next, _ := json.Marshal(map[string]any{"pageToken": page})
			if w = nativeAppRequest("POST", appPath(a)+"threads", string(next), other); w.Code != 400 {
				t.Fatal("cursor reused across customers", w.Code)
			}
		}
	}
	if page != "" {
		t.Fatal("extra cursor after last page")
	}
}
