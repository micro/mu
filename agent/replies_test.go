package agent

import (
	"encoding/json"
	"fmt"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/thread"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReplyQueueRecovery(t *testing.T) {
	// Keep workers stopped; exercise the same restore/worker code deterministically.
	replies.once.Do(func() {})
	replies.jobs = map[string]pendingReply{}
	replies.active = map[string]string{}
	const owner = "queue_recovery_owner"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	other := "queue_recovery_other"
	if err := auth.Create(&auth.Account{ID: other, Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"queued", "running", "finishing"} {
		t.Run(state, func(t *testing.T) {
			th := thread.Open(owner, thread.WebClient, state)
			job := pendingReply{ID: state, Account: owner, Thread: th.ID, Text: "saved request", State: state, Created: time.Now().UTC()}
			if err := thread.Flush(); err != nil {
				t.Fatal(err)
			}
			if err := data.SaveJSON(repliesFile, map[string]pendingReply{job.ID: job}); err != nil {
				t.Fatal(err)
			}
			replies.jobs = nil
			if err := restoreReplies(); err != nil {
				t.Fatal(err)
			}
			calls := 0
			ask := func(r AskRequest) (Answer, error) {
				calls++
				if r.Account != owner || r.AnswerRef != "reply-answer:"+job.ID {
					t.Fatal("wrong execution identity")
				}
				if len(thread.Messages(owner, th.ID, 0)) != 1 {
					t.Fatal("request not restored before execution")
				}
				recordAnswer(owner, th.ID, "saved answer", "", "", r.AnswerRef)
				return Answer{Text: "saved answer"}, nil
			}
			processReply(ask)
			if state == "queued" && calls != 1 || state != "queued" && calls != 0 {
				t.Fatalf("execution calls %d", calls)
			}
			msgs := thread.Messages(owner, th.ID, 0)
			if len(msgs) != 2 || msgs[0].Text != "saved request" || msgs[1].Role != thread.RoleAgent {
				t.Fatalf("missing outcome: %+v", msgs)
			}
			if len(thread.Messages(other, th.ID, 0)) != 0 {
				t.Fatal("foreign owner can read outcome")
			}
			if err := restoreReplies(); err != nil {
				t.Fatal(err)
			}
			processReply(ask)
			if len(thread.Messages(owner, th.ID, 0)) != 2 {
				t.Fatal("duplicate outcome after restore")
			}
		})
	}
	t.Run("completed answer before queue checkpoint", func(t *testing.T) {
		th := thread.Open(owner, thread.WebClient, "completed")
		job := pendingReply{ID: "completed", Account: owner, Thread: th.ID, Text: "request", State: "running", Created: time.Now()}
		if err := persistReplyMessage(job); err != nil {
			t.Fatal(err)
		}
		recordAnswer(owner, th.ID, "already answered", "", "", "reply-answer:"+job.ID)
		if err := thread.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := data.SaveJSON(repliesFile, map[string]pendingReply{job.ID: job}); err != nil {
			t.Fatal(err)
		}
		if err := restoreReplies(); err != nil {
			t.Fatal(err)
		}
		processReply(func(AskRequest) (Answer, error) { t.Fatal("replayed completed work"); return Answer{}, nil })
		if msgs := thread.Messages(owner, th.ID, 0); len(msgs) != 2 || msgs[1].Text != "already answered" {
			t.Fatalf("false interruption: %+v", msgs)
		}
	})
	t.Run("deduplicated acceptance", func(t *testing.T) {
		replies.jobs = map[string]pendingReply{}
		th := thread.Open(owner, thread.WebClient, "retry")
		ref := "durable-client-message-reference"
		for i := 0; i < 2; i++ {
			if err := SubmitReply(owner, th.ID, "once", ref); err != nil {
				t.Fatal(err)
			}
		}
		if len(replies.jobs) != 1 || len(thread.Messages(owner, th.ID, 0)) != 1 {
			t.Fatal("duplicated request")
		}
		if err := SubmitReply(owner, th.ID, "changed", ref); err == nil {
			t.Fatal("reused identity accepted changed text")
		}
		if err := SubmitReply(other, th.ID, "once", ref); err == nil {
			t.Fatal("foreign owner accepted")
		}
		processReply(func(r AskRequest) (Answer, error) { return Answer{}, fmt.Errorf("failed without model") })
		if Pending(owner, th.ID) {
			t.Fatal("failed execution left permanently pending")
		}
	})
}

func TestPendingPollReturnsOwnedSubmission(t *testing.T) {
	replies.once.Do(func() {})
	replies.jobs = map[string]pendingReply{}
	for _, owner := range []string{"poll_owner", "poll_other"} {
		if err := auth.Create(&auth.Account{ID: owner, Admin: true, Created: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	th := thread.Open("poll_owner", thread.WebClient, "poll")
	ref := "poll-first-message"
	id := replyID("poll_owner", th.ID, ref)
	job := pendingReply{ID: id, Account: "poll_owner", Thread: th.ID, Text: "first request", State: "queued", Created: time.Now()}
	if err := persistReplyMessage(job); err != nil {
		t.Fatal(err)
	}
	replies.jobs[id] = job
	_, token, err := auth.CreateToken("poll_owner", "poll", []string{"read"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, foreign, err := auth.CreateToken("poll_other", "poll", []string{"read"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	poll := func(credential string) map[string]any {
		r := httptest.NewRequest("GET", "/agent/pending?thread="+th.ID+"&message="+ref, nil)
		r.Header.Set("Authorization", "Bearer "+credential)
		w := httptest.NewRecorder()
		PendingHandler(w, r)
		if w.Code != 200 {
			t.Fatalf("poll status %d: %s", w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if poll(token)["waiting"] != true {
		t.Fatal("queue not pending")
	}
	recordAnswer("poll_owner", th.ID, "first answer", "", "", "reply-answer:"+id)
	markReply(job, "done")
	if got := poll(token); got["waiting"] != false || !strings.Contains(got["answer_html"].(string), "first answer") {
		t.Fatalf("answer missing: %+v", got)
	}
	thread.Add(thread.Message{Account: "poll_owner", Thread: th.ID, Text: "later request", Ref: "later"})
	recordAnswer("poll_owner", th.ID, "later answer", "", "", "later-answer")
	if got := poll(token); strings.Contains(got["answer_html"].(string), "later answer") {
		t.Fatal("poll returned another submission's answer")
	}
	if got := poll(foreign); got["answer_html"] != nil || got["waiting"] != false {
		t.Fatalf("foreign response disclosed: %+v", got)
	}
}
