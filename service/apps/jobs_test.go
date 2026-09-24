package apps

import (
	"context"
	"mu/internal/auth"
	"mu/internal/persist"
	"mu/internal/service"
	"strings"
	"testing"
)

func TestBuildPublishesReferenceWithoutReadingConversationStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "build_event_test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	ctx := service.WithSourceThread(service.WithAccount(context.Background(), owner), "opaque-origin")
	var rsp BuildResponse
	if err := (Server{}).Build(ctx, &BuildRequest{Prompt: "A packing checklist", RequestKey: "event-test"}, &rsp); err != nil {
		t.Fatal(err)
	}
	j, err := readBuild(rsp.ID, owner)
	if err != nil || j.Thread != "opaque-origin" || !j.EventStored {
		t.Fatal("source/event not persisted")
	}
	j.State = "complete"
	j.App = &App{Slug: "event-test-app", Name: "Checklist", AuthorID: owner}
	if !checkpointBuild(j) {
		t.Fatal("checkpoint failed")
	}
	names, err := persist.List("outbox/log")
	if err != nil || len(names) < 2 {
		t.Fatal("missing events", err)
	}
	for _, name := range names {
		b, _ := persist.Read("outbox/log/" + name)
		if strings.Contains(string(b), "packing checklist") {
			t.Fatal("prompt copied into event")
		}
	}
	if _, err := readBuild(rsp.ID, "other"); err == nil {
		t.Fatal("foreign build accessible")
	}
}

func TestReadUsesEditedAppAndPreservesOmittedPrice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "app-iteration-test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	ctx := service.WithAccount(context.Background(), owner)
	a, err := CreateApp(owner, "Iteration", "build-iteration-test", "", "", "<p>Original</p>", "", 7, false, "source-test")
	if err != nil {
		t.Fatal(err)
	}
	buildMu.Lock()
	buildJobs["iteration-test"] = &BuildJob{ID: "iteration-test", Account: owner, State: "complete", App: &App{Slug: a.Slug, AuthorID: owner, HTML: "<p>Original</p>"}}
	buildMu.Unlock()
	var edited EditResponse
	if err := (Server{}).Edit(ctx, &EditRequest{Slug: a.Slug, HTML: "<p>Updated</p>"}, &edited); err != nil {
		t.Fatal(err)
	}
	var read AppReadResponse
	if err := (Server{}).Read(ctx, &AppReadRequest{Slug: a.Slug}, &read); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.HTML, "Updated") || read.Source != "source-test" {
		t.Fatalf("stale app read: %+v", read)
	}
	if GetApp(a.Slug).Price != 7 {
		t.Fatal("omitted price reset app pricing")
	}
	var collection CollectionResponse
	if err := (Server{}).Collection(service.WithAccount(context.Background(), "another-owner"), &CollectionRequest{}, &collection); err != nil {
		t.Fatal(err)
	}
	if len(collection.Items) != 0 {
		t.Fatal("foreign app in collection")
	}
}
