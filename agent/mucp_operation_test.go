package agent

import (
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
	"testing"
	"time"
)

func TestDurableOperationRecovery(t *testing.T) {
	replies.once.Do(func() {})
	replies.jobs = map[string]pendingReply{}
	replies.active = map[string]string{}
	replies.err = nil
	owner := "mucp_operation_owner"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	q := &pb.SubmitRequest{MessageId: "durable-message-123456", Request: &pb.AskRequest{Prompt: "Remember this request", Mode: pb.AccessMode_ACCESS_MODE_PERSONAL}}
	first, err := assistantSubmit(owner, q)
	if err != nil {
		t.Fatal(err)
	}
	again, err := assistantSubmit(owner, q)
	if err != nil || first.Id != again.Id || first.Thread != again.Thread {
		t.Fatal(again, err)
	}
	if len(thread.Messages(owner, first.Thread, 0)) != 1 {
		t.Fatal("duplicate user message")
	}
	q.Request.Voice = true
	if _, err := assistantSubmit(owner, q); err == nil {
		t.Fatal("changed envelope accepted")
	}
	q.Request.Voice = false
	if _, err := assistantOperation("other", first.Id); err == nil {
		t.Fatal("foreign operation exposed")
	}
	// Restore receipts from disk, then replay the same request without executing twice.
	replies.jobs = nil
	if err := restoreReplies(); err != nil {
		t.Fatal(err)
	}
	again, err = assistantSubmit(owner, q)
	if err != nil || again.Id != first.Id {
		t.Fatal(again, err)
	}
	processReply(func(r AskRequest) (Answer, error) {
		recordAnswer(owner, first.Thread, "Recovered result", "", "", r.AnswerRef)
		return Answer{Text: "Recovered result"}, nil
	})
	result, err := assistantOperation(owner, first.Id)
	if err != nil || result.State != pb.OperationState_OPERATION_STATE_SUCCEEDED || result.Result.Text != "Recovered result" {
		t.Fatal(result, err)
	}
	if err := data.SaveJSON(repliesFile, map[string]pendingReply{}); err != nil {
		t.Fatal(err)
	}
	replies.jobs = map[string]pendingReply{}
}
