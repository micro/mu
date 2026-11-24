package safefetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSignedCallbackDoesNotFollowRedirect(t *testing.T) {
	calls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/first" {
			w.Header().Set("Location", "http://1.1.1.1/second")
			w.WriteHeader(307)
			return
		}
		w.WriteHeader(204)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	response, e := Fetch(context.Background(), "http://1.1.1.1/first", Options{Method: "POST", Body: "receipt", NoRedirect: true})
	if e != nil || response.Status != 307 || calls != 1 {
		t.Fatal("callback followed redirect", response, e, calls)
	}
	response, e = Fetch(context.Background(), "http://1.1.1.1/first", Options{})
	if e != nil || response.Status != 204 || calls != 3 {
		t.Fatal("ordinary redirect behavior changed", response, e, calls)
	}
	if _, e = Fetch(context.Background(), "http://127.0.0.1/private", Options{NoRedirect: true}); e == nil {
		t.Fatal("private destination accepted")
	}
}
