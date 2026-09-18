package account

import (
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"mu/internal/auth"
)

func TestClientSetupForms(t *testing.T) {
	for _, tc := range []struct {
		query, want string
		absent      []string
	}{
		{"", "Add client", []string{`id="create-token-form"`, `name="redirect_uris"`, `name="client" value="mail"`}},
		{"?add=mail", `name="client" value="mail"`, []string{`id="create-token-form"`, `name="redirect_uris"`, `<td data-label="Client">XMPP</td>`}},
		{"?add=xmpp", `name="client" value="chat"`, []string{`id="create-token-form"`, `name="redirect_uris"`, `<td data-label="Client">IMAP</td>`}},
		{"?add=api", `id="create-token-form"`, []string{`name="redirect_uris"`, `name="client" value="mail"`}},
		{"?add=oauth", `name="redirect_uris"`, []string{`id="create-token-form"`, `name="client" value="mail"`}},
	} {
		r := httptest.NewRequest("GET", "/account/clients"+tc.query, nil)
		w := httptest.NewRecorder()
		handleTokenPage(w, r, "setup_view_owner", "")
		body := w.Body.String()
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s missing %s", tc.query, tc.want)
		}
		for _, s := range tc.absent {
			if strings.Contains(body, s) {
				t.Errorf("%s shows unrelated setup %s", tc.query, s)
			}
		}
	}
}

func TestGoogleControlsStayInsideManage(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	r := httptest.NewRequest("GET", "/account", nil)
	body := renderGoogleCard(r, &auth.Account{ID: "google_ui_owner"}, "")
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inside bool) {
		if n.Type == html.ElementNode && n.Data == "details" {
			inside = true
			for _, a := range n.Attr {
				if a.Key == "open" {
					t.Error("Google controls expanded on arrival")
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && strings.HasPrefix(a.Val, "/oauth2/google/") {
					count++
					if !inside {
						t.Error("Google action outside Manage")
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inside)
		}
	}
	walk(root, false)
	if count != 5 {
		t.Fatalf("missing Google actions: %d", count)
	}
}
