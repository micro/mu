package agent

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/auth"
	"mu/internal/mucp"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
	"strings"
	"sync"
	"unicode/utf8"
)

var submissionMu sync.Mutex

func assistantSubmit(owner string, q *pb.SubmitRequest) (*pb.Operation, error) {
	if !mucp.ValidID(q.MessageId) || len(q.MessageId) > 120 || q.Request == nil {
		return nil, invalid("Provide a 16–120 character message ID and request")
	}
	req := q.Request
	if req.Mode != pb.AccessMode_ACCESS_MODE_PERSONAL {
		return nil, invalid("Durable submission requires PERSONAL access")
	}
	if !utf8.ValidString(req.Prompt) || strings.TrimSpace(req.Prompt) == "" || len(req.Prompt) > askLimit {
		return nil, invalid("Write a message of up to 8,000 UTF-8 bytes")
	}
	selected, err := assistantAgent(owner, req.AgentId)
	if err != nil {
		return nil, err
	}
	if req.Thread != "" {
		t, e := assistantOwnedThread(owner, req.Thread)
		if e != nil {
			return nil, e
		}
		if assistantThreadInfo(*t).AgentId != selected {
			return nil, missing()
		}
	}
	if !auth.Trusted(owner) {
		return nil, &assistantError{403, "forbidden", "Account verification required"}
	}
	submissionMu.Lock()
	defer submissionMu.Unlock()
	ref := "mucp:" + q.MessageId
	loadReplies()
	replies.Lock()
	var existing *pendingReply
	for _, j := range replies.jobs {
		if j.Account == owner && j.Ref == ref {
			copy := j
			existing = &copy
			break
		}
	}
	replies.Unlock()
	if existing != nil {
		if (req.Thread != "" && req.Thread != existing.Thread) || existing.EnvelopeDigest != replyEnvelope(req.Prompt, selected, req.Voice) {
			return nil, &assistantError{409, "conflict", "Message ID already used for a different request"}
		}
		return assistantOperation(owner, existing.ID)
	}
	if reason, ok := affordable(owner); !ok {
		return nil, &assistantError{402, "payment_required", reason}
	}
	id, err := queuePrompt(owner, req.Thread, req.Prompt, ref, selected, "", ClientContext{Voice: req.Voice})
	if err != nil {
		return nil, &assistantError{409, "not_accepted", err.Error()}
	}
	return assistantOperation(owner, replyID(owner, id, ref))
}
func assistantOperation(owner, id string) (*pb.Operation, error) {
	loadReplies()
	replies.Lock()
	j, ok := replies.jobs[id]
	replies.Unlock()
	if !ok || j.Account != owner {
		return nil, missing()
	}
	t, err := assistantOwnedThread(owner, j.Thread)
	if err != nil {
		return nil, err
	}
	out := &pb.Operation{Id: j.ID, Thread: j.Thread, AgentId: assistantThreadInfo(*t).AgentId, Created: timestamppb.New(j.Created), State: pb.OperationState_OPERATION_STATE_RUNNING}
	if j.State == "queued" {
		out.State = pb.OperationState_OPERATION_STATE_QUEUED
	}
	if j.State == "interrupted" {
		out.State = pb.OperationState_OPERATION_STATE_INTERRUPTED
		out.Error = "Execution was interrupted; inspect the thread before trying again."
	}
	for _, m := range thread.Messages(owner, j.Thread, 0) {
		if m.Role != thread.RoleAgent {
			continue
		}
		switch m.Ref {
		case "reply-answer:" + id:
			out.State = pb.OperationState_OPERATION_STATE_SUCCEEDED
			out.Result = &pb.AskResponse{Thread: j.Thread, AgentId: out.AgentId, Text: m.Text, Content: assistantContent(m.Results)}
		case "reply-error:" + id:
			out.State = pb.OperationState_OPERATION_STATE_FAILED
			out.Error = m.Text
		case "reply-interrupted:" + id:
			out.State = pb.OperationState_OPERATION_STATE_INTERRUPTED
			out.Error = m.Text
		}
	}
	if j.State == "done" && out.State == pb.OperationState_OPERATION_STATE_RUNNING {
		out.State = pb.OperationState_OPERATION_STATE_INTERRUPTED
		out.Error = "The recorded result is unavailable; inspect the thread before trying again."
	}
	return out, nil
}
