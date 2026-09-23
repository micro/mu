package apps

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/thread"
	"testing"
)

func TestBuildReturnsToOwnedConversationOnce(t *testing.T) {
	const owner = "build_delivery_test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	th := thread.Open(owner, thread.WebClient, "build-delivery-test")
	defer thread.Forget(owner)
	ctx := service.WithSourceThread(service.WithAccount(context.Background(), owner), th.ID)
	var rsp BuildResponse
	if err := (Server{}).Build(ctx, &BuildRequest{Prompt: "A packing checklist", RequestKey: "delivery-test"}, &rsp); err != nil {
		t.Fatal(err)
	}
	j, err := readBuild(rsp.ID, owner)
	if err != nil || j.Thread != th.ID {
		t.Fatal("conversation not persisted")
	}
	defer func() { buildMu.Lock(); delete(buildJobs, j.ID); buildMu.Unlock() }()
	j.State = "complete"
	j.App = &App{Slug: "test-delivered-app", Name: "Packing checklist", AuthorID: owner}
	if !checkpointBuild(j) {
		t.Fatal("checkpoint failed")
	}
	deliverBuilds()
	// Simulate a restart after message persistence but before delivery checkpoint.
	j.Delivered = false
	checkpointBuild(j)
	deliverBuilds()
	messages := thread.Messages(owner, th.ID, 10)
	if len(messages) != 1 || len(messages[0].Results) != 1 || messages[0].Results[0].ID != j.App.Slug {
		t.Fatal("result missing or duplicated", messages)
	}
	if _, err := submitBuild("test", owner, "wrong-owner", thread.Open("another-owner", thread.WebClient, "other").ID); err == nil {
		t.Fatal("foreign thread accepted")
	}
}
