package server

import (
	"mu/internal/auth"
	"mu/internal/push"
	"mu/service/notify"
	"strings"
	"testing"
	"time"
)

func TestNotifyContractIsolation(t *testing.T) {
	notify.LoadService()
	tokens := map[string]string{}
	for _, id := range []string{"notify_contract_owner", "notify_contract_other"} {
		auth.SetAccountForTest(&auth.Account{ID: id, Admin: true})
		t.Cleanup(func() { notify.DeleteAll(id); auth.RemoveAccountForTest(id) })
		_, token, err := auth.CreateToken(id, "notify", []string{"read", "write", "service:notify"}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		tokens[id] = token
	}
	// A stored fixture only: no test sends a notification to this endpoint.
	_, err := push.Subscribe("notify_contract_owner", push.Subscription{Endpoint: "https://example.invalid/private-push", P256dh: "AQ", Auth: "Ag", Label: "private fixture phone"})
	if err != nil {
		t.Fatal(err)
	}
	owner := tokens["notify_contract_owner"]
	r := compositionCall(owner, "notify", "Notify.Devices", `{}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "private fixture phone") {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, private := range []string{"private-push", `"p256dh"`, `"auth"`, `"account"`} {
		if strings.Contains(r.Body.String(), private) {
			t.Fatalf("private subscription data leaked: %s", r.Body.String())
		}
	}
	r = compositionCall(tokens["notify_contract_other"], "notify", "Notify.Devices", `{}`)
	if r.Code != 200 || strings.Contains(r.Body.String(), "private fixture phone") {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, scopes := range [][]string{{"read", "service:notify"}, {"read", "write", "api:agent"}} {
		_, token, err := auth.CreateToken("notify_contract_owner", "limited", scopes, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		r = compositionCall(token, "notify", "Notify.Send", `{"title":"fixture"}`)
		if r.Code != 403 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	r = compositionCall(owner, "notify", "Notify.Send", `{"title":"fixture","account":"notify_contract_other"}`)
	if r.Code != 400 {
		t.Fatal(r.Code, r.Body.String())
	}
	r = compositionCall(owner, "notify", "Notify.Send", `{"title":"fixture","recipient":"other"}`)
	if r.Code != 400 {
		t.Fatal(r.Code, r.Body.String())
	}
	// Exercise the real handler's validation without invoking push delivery.
	r = compositionCall(owner, "notify", "Notify.Send", `{"title":" "}`)
	if r.Code != 400 || !strings.Contains(r.Body.String(), "title") {
		t.Fatal(r.Code, r.Body.String())
	}
	if len(notify.History("notify_contract_owner", 10)) != 0 {
		t.Fatal("validation failure sent a notification")
	}
}
