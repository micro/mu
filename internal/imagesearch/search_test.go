package imagesearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBraveImageRequestAndFiltering(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("safesearch") != "strict" || r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Error("missing safe search or authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"Lake","url":"https://example.org/page","thumbnail":{"src":"https://example.org/thumb"},"properties":{"url":"https://example.org/image"}},{"url":"javascript:alert(1)"}]}`))
	}))
	defer srv.Close()
	old := imageSearchURL
	imageSearchURL = srv.URL
	defer func() { imageSearchURL = old }()
	rows, err := Search(context.Background(), "lake")
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %#v", err, rows)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, "lake"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
