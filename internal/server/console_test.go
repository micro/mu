package server

import (
	"io"
	"mu/agent"
	"mu/home"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/news"
	"mu/service/video"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNavigationRedirectsTerminateThroughLegacyMiddleware(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "redirect-loop-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	th := thread.Open(owner, thread.WebClient, "existing-conversation")
	thread.Name(owner, th.ID, "Existing conversation")
	mux := http.NewServeMux()
	mux.HandleFunc("/", IndexHandler)
	mux.HandleFunc("/home", home.Handler)
	mux.HandleFunc("/home/apps", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("App collection reached")) })
	mux.HandleFunc("/agent", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agent/micro?"+r.URL.RawQuery, http.StatusSeeOther)
	})
	mux.HandleFunc("/agent/", agent.Handler)
	// Same ordering as serve.go: root handler, then legacy redirects, then mux.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			IndexHandler(w, r)
			return
		}
		if consoleRedirect(w, r) {
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	base, _ := url.Parse(srv.URL)
	jar.SetCookies(base, []*http.Cookie{{Name: "session", Value: session.Token}})
	client := srv.Client()
	client.Jar = jar
	client.Timeout = 2 * time.Second
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	for _, tc := range []struct{ path, want string }{
		{"/", "/home"}, {"/home", "/home"}, {"/home/apps", "/home/apps"},
		{"/agent", "/agent/micro"}, {"/agent/micro", "/agent/micro"},
		{"/?session=" + th.ID, "/agent/micro"}, {"/home?session=" + th.ID, "/agent/micro"},
		{"/agent/micro?session=" + th.ID, "/agent/micro"}, {"/assistant?session=" + th.ID, "/agent/micro"},
	} {
		rsp, err := client.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(rsp.Body)
		rsp.Body.Close()
		if rsp.StatusCode != 200 || rsp.Request.URL.Path != tc.want {
			t.Fatalf("%s: ended at %s status %d location %s", tc.path, rsp.Request.URL, rsp.StatusCode, rsp.Header.Get("Location"))
		}
		if strings.Contains(tc.path, "session=") && !strings.Contains(string(body), "Existing conversation") {
			t.Fatalf("lost conversation at %s", tc.path)
		}
	}
}

// Exercise the middleware before real service handlers with an ordinary account.
func TestServiceNavigationIsNotAnAdminOrAgentDetour(t *testing.T) {
	if err := service.Register(news.Spec); err != nil {
		t.Fatal(err)
	}
	owner := "ordinary-service-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/services", api.ToolsPageHandler}, {"/news", news.Handler}, {"/video", video.Handler},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		if consoleRedirect(w, r) {
			t.Fatalf("service redirected: %s", tc.path)
		}
		tc.handler(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestHomePinsRequireOwnerSessionAndCSRF(t *testing.T) {
	owner, other := "pin-owner", "pin-other"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true, Pinned: []string{}})
	auth.SetAccountForTest(&auth.Account{ID: other, Approved: true, Pinned: []string{}})
	defer auth.RemoveAccountForTest(owner)
	defer auth.RemoveAccountForTest(other)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(news.Spec); err != nil {
		t.Fatal(err)
	}
	for _, csrf := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/services", strings.NewReader("pin=news&account=pin-other"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		api.ToolsPageHandler(w, r)
		want := 403
		if csrf {
			want = 303
		}
		if w.Code != want {
			t.Fatalf("pin csrf=%v: %d", csrf, w.Code)
		}
	}
	own, _ := auth.GetAccount(owner)
	foreign, _ := auth.GetAccount(other)
	if len(own.PinnedServices()) != 1 || own.PinnedServices()[0] != "news" || len(foreign.PinnedServices()) != 0 {
		t.Fatal("pin ownership violated")
	}
}
