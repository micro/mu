package blog

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWalletAuthorShowsOnlyPublicPosts(t *testing.T) {
	const id = "x402:0x0873d31be103a7ff9328dcd633d2c6285c5c1692"
	mutex.Lock()
	before := posts
	posts = append(append([]*Post{}, before...), &Post{ID: "wallet-public-test", AuthorID: id, Title: "Public technical note"}, &Post{ID: "wallet-private-test", AuthorID: id, Title: "Hidden draft", Private: true})
	mutex.Unlock()
	defer func() { mutex.Lock(); posts = before; mutex.Unlock() }()
	w := httptest.NewRecorder()
	WalletAuthorHandler(w, httptest.NewRequest("GET", "/@"+id, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Public technical note") || strings.Contains(w.Body.String(), "Hidden draft") {
		t.Fatal("bad public author page", w.Code)
	}
}
