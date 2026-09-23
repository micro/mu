package apps

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeContractDiagnostics(t *testing.T) {
	for _, js := range []string{
		`mu.api('markets.list')`,
		`fetch('https://api.coingecko.com/api/v3/coins/markets')`,
		"const url = `https://api.coingecko.com/api/v3/coins/markets`; fetch(url)",
	} {
		if len(runtimeIssues("<script>"+js+"</script>")) == 0 {
			t.Fatalf("missed broken app: %s", js)
		}
	}
	for _, js := range []string{`fetch('/api/v1/markets/list')`, `mu.service('markets','list',{})`, `mu.web.fetch('https://example.com')`} {
		if issues := runtimeIssues("<script>" + js + "</script>"); len(issues) > 0 {
			t.Fatalf("rejected supported call: %v", issues)
		}
	}
	// Validation happens before mutation, so a failed repair preserves the app.
	mutex.Lock()
	apps["contract-test"] = &App{Slug: "contract-test", HTML: "original", Name: "Original"}
	mutex.Unlock()
	defer func() { mutex.Lock(); delete(apps, "contract-test"); mutex.Unlock() }()
	if _, err := UpdateApp("contract-test", "Changed", "", "", "<script>mu.api('markets')</script>", "", -1); err == nil {
		t.Fatal("accepted broken edit")
	}
	if a := GetApp("contract-test"); a.HTML != "original" || a.Name != "Original" {
		t.Fatal("failed edit mutated app")
	}
	result := TestHTML(`<html><body>A sufficiently long document with an interactive script but an invalid runtime call.</body><script>mu.api('markets')</script></html>`, "")
	if result.OK || !strings.Contains(strings.Join(result.Issues, " "), "mu.api") {
		t.Fatal("test reported broken SDK as successful")
	}
}

type ContractReadServer struct{}
type ContractReadRequest struct{}
type ContractReadResponse struct {
	Account string `json:"account"`
}

func (*ContractReadServer) List(ctx context.Context, _ *ContractReadRequest, out *ContractReadResponse) error {
	out.Account = service.AccountFrom(ctx)
	return nil
}

func TestPublicServiceReadHasNoAccount(t *testing.T) {
	err := service.Register(service.Spec{Name: "appcontracttest", Handler: &ContractReadServer{}, Endpoints: map[string]service.Endpoint{
		"List": {}, "Private": {Needs: service.Caller}, "Paid": {Cost: "test"}, "Write": {Writes: true}, "Delete": {Destructive: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"Private", "Paid", "Write", "Delete", "Missing"} {
		if publicServiceRead("appcontracttest", method) {
			t.Fatalf("unapproved access to %s", method)
		}
	}
	if !publicServiceRead("appcontracttest", "list") {
		t.Fatal("public read requires approval")
	}
	auth.SetAccountForTest(&auth.Account{ID: "app_contract_owner"})
	defer auth.RemoveAccountForTest("app_contract_owner")
	sess, err := auth.CreateSession("app_contract_owner")
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	r := httptest.NewRequest("POST", "/apps/test-app/sdk/service", strings.NewReader(`{"service":"appcontracttest","method":"List","anonymous":true}`))
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	handleSDKService(w, r, "test-app")
	if w.Code != 200 || strings.Contains(w.Body.String(), "app_contract_owner") {
		t.Fatalf("public request retained identity: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("POST", "/apps/test-app/sdk/service", strings.NewReader(`{"service":"appcontracttest","method":"Private","anonymous":true}`))
	w = httptest.NewRecorder()
	handleSDKService(w, r, "test-app")
	if w.Code != 403 {
		t.Fatal("forged anonymous private request accepted")
	}
}

func TestRetiredTemplatesStayAddressableButLeaveCatalog(t *testing.T) {
	mutex.Lock()
	apps["retired-template-test"] = &App{Slug: "retired-template-test", Official: true, Public: true, AuthorID: "mu"}
	mutex.Unlock()
	defer func() { mutex.Lock(); delete(apps, "retired-template-test"); mutex.Unlock() }()
	r := httptest.NewRequest("GET", "/apps", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	handleList(w, r)
	if strings.Contains(w.Body.String(), "retired-template-test") {
		t.Fatal("template still listed")
	}
	if GetApp("retired-template-test") == nil {
		t.Fatal("retired app was deleted")
	}
}
