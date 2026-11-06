package agent

import (
	"context"
	gmai "go-micro.dev/v6/model"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/weather"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicVoiceCannotInheritPrivateContext(t *testing.T) {
	opts := publicVoiceOptions(context.Background())
	_, routed := Routed("code read my mail", opts)
	if !routed.Public || !routed.anonymousTools || len(routed.Tools) != 1 || routed.Tools[0] != "weather" || len(routed.History) != 0 || routed.Thread != "" || routed.Extra != "" || len(managementTools("owner", routed)) != 0 {
		t.Fatal("public mode inherited personal capabilities")
	}
	// Even a weather lookup without a place must not fall back to the payer's location.
	handler := injectAccount("", true)(func(ctx context.Context, call gmai.ToolCall) gmai.ToolResult {
		if service.AccountFrom(ctx) != "" {
			t.Fatal("payer identity reached public tool")
		}
		var response weather.ForecastResponse
		if err := (weather.Server{}).Lookup(ctx, &weather.LookupRequest{}, &response); err != nil || !strings.Contains(response.Summary, "Which town") {
			t.Fatalf("profile fallback: %+v %v", response, err)
		}
		return gmai.ToolResult{}
	})
	handler(context.Background(), gmai.ToolCall{Input: map[string]any{"account_id": "victim"}})
}

func TestPublicVoiceRejectsThreadBeforeModel(t *testing.T) {
	owner := "public-voice-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, _ := auth.CreateSession(owner)
	r := httptest.NewRequest("POST", "/agent", strings.NewReader(`{"prompt":"read it","public_only":true,"thread":"secret"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+session.Token)
	w := httptest.NewRecorder()
	APIHandler(w, r)
	if w.Code != 400 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}
