package mail

import (
	"encoding/json"
	"mu/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJMAPAuthenticationIsolationAndReadState(t *testing.T) {
	owner := "jmap-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	_, token, err := auth.CreateToken(owner, "jmap-test", []string{"read", "write", "protocol:mail"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	old := messages
	setMessages([]*Message{{ID: "mine", ToID: owner, FromID: "sender@example.com", Subject: "My mail", Body: "Hello", CreatedAt: time.Now()}, {ID: "foreign", ToID: "other", Subject: "Private", Body: "secret", CreatedAt: time.Now()}})
	defer setMessages(old)
	request := func(body, credential string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/mail/jmap", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if credential != "" {
			r.SetBasicAuth(owner, credential)
		}
		w := httptest.NewRecorder()
		JMAPHandler(w, r)
		return w
	}
	if w := request(`{}`, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	body := `{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],"methodCalls":[["Email/query",{"accountId":"jmap-owner"},"a"],["Email/get",{"accountId":"jmap-owner","#ids":{"resultOf":"a","name":"Email/query","path":"/ids"}},"b"]]}`
	w := request(body, token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "My mail") || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(strings.ReplaceAll(body, "jmap-owner", "other"), token); !strings.Contains(w.Body.String(), "accountNotFound") {
		t.Fatal(w.Body.String())
	}
	id := jmapID("mine")
	args := jmapObject{"update": map[string]any{id: map[string]any{"keywords/$seen": true}}}
	result := jmapMethod(owner, "Email/set", args)
	if len(result["updated"].(map[string]any)) != 1 {
		t.Fatal(result)
	}
	mutex.RLock()
	read := messages[0].Read
	mutex.RUnlock()
	if !read {
		t.Fatal("read state not shared with Mail")
	}
	invalid := jmapMethod(owner, "Email/set", jmapObject{"update": map[string]any{jmapID("foreign"): map[string]any{"keywords/$seen": true}}})
	b, _ := json.Marshal(invalid)
	if !strings.Contains(string(b), "notFound") {
		t.Fatal(string(b))
	}
	_, readonly, err := auth.CreateToken(owner, "read only", []string{"read", "service:mail"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	mutation := `{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],"methodCalls":[["Email/set",{"accountId":"jmap-owner","destroy":["` + id + `"]},"x"]]}`
	if result := request(mutation, readonly); !strings.Contains(result.Body.String(), "forbidden") {
		t.Fatal("read-only token could mutate", result.Body.String())
	}
	_, wrong, err := auth.CreateToken(owner, "wrong scope", []string{"protocol:chat"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if w = request(body, wrong); w.Code != 401 {
		t.Fatal("wrong scope accepted", w.Code)
	}
}
func TestJMAPQueryFiltersAndPaging(t *testing.T) {
	v := jmapView{messages: []*Message{{ID: "a", Subject: "hello", CreatedAt: time.Now()}, {ID: "b", Subject: "other", CreatedAt: time.Now()}}, membership: map[string]map[string]bool{}, state: "test"}
	q := jmapQuery("owner", v, jmapObject{"filter": map[string]any{"subject": "hello"}, "limit": float64(1)})
	if len(q["ids"].([]any)) != 1 {
		t.Fatal(q)
	}
	q = jmapQuery("owner", v, jmapObject{"filter": map[string]any{"unknown": true}})
	if q["type"] != "unsupportedFilter" {
		t.Fatal(q)
	}
	q = jmapQuery("owner", v, jmapObject{"anchor": "missing"})
	if q["type"] != "anchorNotFound" {
		t.Fatal(q)
	}
}
