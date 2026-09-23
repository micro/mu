package api

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type EmbedCardFixture struct{}
type EmbedCardRequest struct{}
type EmbedCardResponse struct{}

func (*EmbedCardFixture) List(context.Context, *EmbedCardRequest, *EmbedCardResponse) error {
	return nil
}
func TestCardEmbedUsesViewerAndPrivateCache(t *testing.T) {
	err := service.Register(service.Spec{Name: "embedfixture", Handler: &EmbedCardFixture{}, Scoped: true, Endpoints: map[string]service.Endpoint{"List": {}}, Card: service.Personal(func(v service.Viewer) string { return "owner:" + v.Account })})
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"card-alice", "card-bob"} {
		auth.SetAccountForTest(&auth.Account{ID: owner})
		defer auth.RemoveAccountForTest(owner)
		sess, err := auth.CreateSession(owner)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "/card/embedfixture?embed=1", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		CardHandler(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "owner:"+owner) || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("incorrect private card: %d %s", w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/card/embedfixture?embed=1", "/card/embedfixture"} {
		w := httptest.NewRecorder()
		CardHandler(w, httptest.NewRequest("GET", path, nil))
		if w.Code == 200 || strings.Contains(w.Body.String(), "owner:") {
			t.Fatal("anonymous card exposed")
		}
	}
}
