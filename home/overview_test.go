package home

import (
	"fmt"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/apps"
	"testing"
	"time"
)

func TestOverviewRefreshDoesNotBlockAndNeverSharesPersonalCards(t *testing.T) {
	entered, release := make(chan string, 2), make(chan struct{})
	defer close(release)
	name := "home-cache-test"
	if err := service.Register(service.Spec{Name: name, Handler: &apps.Server{}, Page: "/" + name, Card: service.Personal(func(who service.Viewer) string {
		entered <- who.Account
		<-release
		return fmt.Sprintf("private:%s", who.Account)
	})}); err != nil {
		t.Fatal(err)
	}
	a := &auth.Account{ID: "overview-cache-a", Pinned: []string{name}}
	b := &auth.Account{ID: "overview-cache-b", Pinned: []string{name}}
	result := make(chan bool, 1)
	go func() { snapshot, pending := overview(a); result <- pending && len(snapshot.cards) == 0 }()
	select {
	case ok := <-result:
		if !ok {
			t.Fatal("expected cold cache")
		}
	case <-time.After(time.Second):
		t.Fatal("Home waited for renderer")
	}
	select {
	case owner := <-entered:
		if owner != a.ID {
			t.Fatal(owner)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh not started")
	}
	overviewCache.Lock()
	overviewCache.values[a.ID] = overviewSnapshot{key: overviewKey(a), cards: map[string]string{name: "only A"}, at: time.Now()}
	overviewCache.Unlock()
	snapshot, _ := overview(b)
	if len(snapshot.cards) != 0 {
		t.Fatal("another account received private card")
	}
	changed := *a
	changed.Lat = 42
	snapshot, _ = overview(&changed)
	if len(snapshot.cards) != 0 {
		t.Fatal("old place card survived location change")
	}
}
