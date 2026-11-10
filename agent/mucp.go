package agent

// MUCP's first external binding uses HTTP headers and protobuf JSON. It never
// forwards caller metadata to the internal service transport.
import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/mucp"
	"mu/internal/result"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
)

// MUCPHandler retains the agent-only adapter for embedded callers and tests.
func MUCPHandler(w http.ResponseWriter, r *http.Request) { mucp.Handler(MUCPOperations())(w, r) }
func MUCPOperations() []mucp.Operation {
	return []mucp.Operation{
		{Service: "assistant", Endpoint: "Assistant.Submit", Capability: "agent", Writes: true, Request: func() proto.Message { return &pb.SubmitRequest{} }, Handle: assistantCall},
		{Service: "assistant", Endpoint: "Assistant.ReadOperation", Capability: "agent", Request: func() proto.Message { return &pb.ReadOperationRequest{} }, Handle: assistantCall},
		{Service: "assistant", Endpoint: "Assistant.ListAgents", Capability: "agent", Request: func() proto.Message { return &pb.ListAgentsRequest{} }, Handle: assistantCall},
		{Service: "assistant", Endpoint: "Assistant.ListThreads", Capability: "agent", Request: func() proto.Message { return &pb.ListThreadsRequest{} }, Handle: assistantCall},
		{Service: "assistant", Endpoint: "Assistant.ReadThread", Capability: "agent", Request: func() proto.Message { return &pb.ReadThreadRequest{} }, Handle: assistantCall},
		{Service: "assistant", Endpoint: "Assistant.Ask", Capability: "agent", Writes: true, Request: func() proto.Message { return &pb.AskRequest{} }, Handle: assistantCall},
	}
}
func assistantCall(w http.ResponseWriter, r *http.Request, owner, id string, q proto.Message) (proto.Message, error) {
	var response proto.Message
	var err error
	switch q := q.(type) {
	case *pb.SubmitRequest:
		response, err = assistantSubmit(owner, q)
	case *pb.ReadOperationRequest:
		response, err = assistantOperation(owner, q.Id)
	case *pb.ListAgentsRequest:
		response = assistantAgents(owner)
	case *pb.ListThreadsRequest:
		response, err = assistantThreads(owner, q)
	case *pb.ReadThreadRequest:
		response, err = assistantThread(owner, q)
	case *pb.AskRequest:
		response, err = assistantAsk(w, r, owner, id, q)
	}
	var e *assistantError
	if errors.As(err, &e) {
		err = api.Fail(e.status, e.code, e.message)
	}
	return response, err
}

type assistantError struct {
	status        int
	code, message string
}

func (e *assistantError) Error() string { return e.message }
func invalid(message string) error      { return &assistantError{400, "invalid_arguments", message} }
func missing() error                    { return &assistantError{404, "not_found", "Agent or personal thread not found"} }
func assistantAgents(owner string) *pb.ListAgentsResponse {
	out := &pb.ListAgentsResponse{PublicQuestions: true}
	for _, a := range ClientAgents(owner) {
		out.Agents = append(out.Agents, &pb.Agent{Id: a.ID, Name: a.Name, Description: a.Description, Slug: a.Slug})
	}
	return out
}
func assistantAgent(owner, id string) (string, error) {
	if id == "" {
		id = DefaultPlatformAgent
	}
	for _, a := range ClientAgents(owner) {
		if a.ID == id {
			return id, nil
		}
	}
	return "", missing()
}
func assistantOwnedThread(owner, id string) (*thread.Thread, error) {
	t := thread.Get(owner, id)
	if t == nil || t.Client != thread.WebClient || t.Held || t.Canonical != "" {
		return nil, missing()
	}
	if _, err := assistantAgent(owner, t.Agent); err != nil {
		return nil, err
	}
	return t, nil
}
func assistantThreadInfo(t thread.Thread) *pb.Thread {
	agent := t.Agent
	if agent == "" {
		agent = DefaultPlatformAgent
	}
	return &pb.Thread{Id: t.ID, AgentId: agent, Subject: t.Subject, Started: timestamppb.New(t.Started), Updated: timestamppb.New(t.Updated)}
}

// Page tokens authenticate a position, never authorise a record. They expire on
// restart or after 15 minutes. Every page repeats account and agent checks.
var assistantPageKey = sync.OnceValue(func() []byte {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return b
})

type assistantPage struct {
	Scope   string
	At      time.Time
	ID      string
	Expires int64
}

func pageScope(owner, operation, filter string) string {
	sum := sha256.Sum256([]byte(owner + "\x00" + operation + "\x00" + filter))
	return fmt.Sprintf("%x", sum)
}
func pageToken(scope string, at time.Time, id string) string {
	raw, _ := json.Marshal(assistantPage{scope, at, id, time.Now().Add(15 * time.Minute).Unix()})
	mac := hmac.New(sha256.New, assistantPageKey())
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func pagePosition(token, scope string) (assistantPage, error) {
	var p assistantPage
	if token == "" {
		return p, nil
	}
	if len(token) > 2048 {
		return p, invalid("Invalid page token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return p, invalid("Invalid page token")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return p, invalid("Invalid page token")
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return p, invalid("Invalid page token")
	}
	mac := hmac.New(sha256.New, assistantPageKey())
	mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(raw, &p) != nil || p.Scope != scope || p.Expires <= time.Now().Unix() {
		return p, invalid("Invalid or expired page token")
	}
	return p, nil
}
func pageSize(n int32, fallback int) (int, error) {
	if n == 0 {
		return fallback, nil
	}
	if n < 1 || n > 100 {
		return 0, invalid("pageSize must be 1–100")
	}
	return int(n), nil
}
func assistantThreads(owner string, q *pb.ListThreadsRequest) (*pb.ListThreadsResponse, error) {
	n, err := pageSize(q.PageSize, 25)
	if err != nil {
		return nil, err
	}
	if q.AgentId != "" {
		if _, err = assistantAgent(owner, q.AgentId); err != nil {
			return nil, err
		}
	}
	scope := pageScope(owner, "threads", q.AgentId)
	p, err := pagePosition(q.PageToken, scope)
	if err != nil {
		return nil, err
	}
	all := thread.List(owner, 0)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Updated.Equal(all[j].Updated) {
			return all[i].ID < all[j].ID
		}
		return all[i].Updated.After(all[j].Updated)
	})
	out := &pb.ListThreadsResponse{}
	for _, t := range all {
		if _, err = assistantOwnedThread(owner, t.ID); err != nil {
			continue
		}
		info := assistantThreadInfo(t)
		if q.AgentId != "" && q.AgentId != info.AgentId {
			continue
		}
		if p.ID != "" && (t.Updated.After(p.At) || t.Updated.Equal(p.At) && t.ID <= p.ID) {
			continue
		}
		if len(out.Threads) == n {
			last := out.Threads[n-1]
			out.NextPageToken = pageToken(scope, last.Updated.AsTime(), last.Id)
			break
		}
		out.Threads = append(out.Threads, info)
	}
	return out, nil
}
func assistantThread(owner string, q *pb.ReadThreadRequest) (*pb.ReadThreadResponse, error) {
	t, err := assistantOwnedThread(owner, q.Thread)
	if err != nil {
		return nil, err
	}
	n, err := pageSize(q.PageSize, 50)
	if err != nil {
		return nil, err
	}
	scope := pageScope(owner, "messages", t.ID)
	p, err := pagePosition(q.PageToken, scope)
	if err != nil {
		return nil, err
	}
	all := thread.Messages(owner, t.ID, 0)
	sort.Slice(all, func(i, j int) bool {
		if all[i].At.Equal(all[j].At) {
			return all[i].ID < all[j].ID
		}
		return all[i].At.Before(all[j].At)
	})
	out := &pb.ReadThreadResponse{Thread: assistantThreadInfo(*t), Pending: replyStatus(owner, t.ID) != ""}
	for _, m := range all {
		if m.Role != thread.RoleAgent && m.Role != thread.RolePerson {
			continue
		}
		if p.ID != "" && (m.At.Before(p.At) || m.At.Equal(p.At) && m.ID <= p.ID) {
			continue
		}
		if len(out.Messages) == n {
			last := out.Messages[n-1]
			out.NextPageToken = pageToken(scope, last.At.AsTime(), last.Id)
			break
		}
		role := pb.Role_ROLE_PERSON
		if m.Role == thread.RoleAgent {
			role = pb.Role_ROLE_AGENT
		}
		out.Messages = append(out.Messages, &pb.Message{Id: m.ID, Role: role, Text: m.Text, At: timestamppb.New(m.At), Content: assistantContent(m.Results)})
	}
	return out, nil
}
func assistantAsk(w http.ResponseWriter, r *http.Request, owner, ref string, q *pb.AskRequest) (*pb.AskResponse, error) {
	if !utf8.ValidString(q.Prompt) || strings.TrimSpace(q.Prompt) == "" || len(q.Prompt) > askLimit {
		return nil, invalid("Write a message of up to 8,000 UTF-8 bytes")
	}
	if q.Mode != pb.AccessMode_ACCESS_MODE_PERSONAL && q.Mode != pb.AccessMode_ACCESS_MODE_PUBLIC {
		return nil, invalid("Select PERSONAL or PUBLIC access explicitly")
	}
	if q.Mode == pb.AccessMode_ACCESS_MODE_PUBLIC {
		if q.Thread != "" || q.AgentId != "" {
			return nil, invalid("Public questions cannot select an agent or thread")
		}
		if reason, ok := affordable(owner); !ok {
			return nil, &assistantError{402, "payment_required", reason}
		}
		text, err := QueryWithOpts(owner, q.Prompt, publicVoiceOptions(r.Context()))
		if err != nil {
			return nil, err
		}
		return &pb.AskResponse{Text: text}, nil
	}
	selected, err := assistantAgent(owner, q.AgentId)
	if err != nil {
		return nil, err
	}
	if q.Thread != "" {
		t, e := assistantOwnedThread(owner, q.Thread)
		if e != nil {
			return nil, e
		}
		if assistantThreadInfo(*t).AgentId != selected {
			return nil, missing()
		}
	}
	if reason, ok := affordable(owner); !ok {
		return nil, &assistantError{402, "payment_required", reason}
	}
	if !auth.Trusted(owner) {
		return nil, &assistantError{403, "forbidden", "Account verification required"}
	}
	id, err := queuePrompt(owner, q.Thread, q.Prompt, ref, selected, "", ClientContext{Voice: q.Voice})
	if err != nil {
		return nil, &assistantError{409, "not_accepted", err.Error()}
	}
	w.Header().Set("Micro-Thread", id)
	key := replyID(owner, id, ref)
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		// Revocation and deletion while the work runs must revoke the response too.
		if _, err = assistantOwnedThread(owner, id); err != nil {
			return nil, err
		}
		if _, _, err = auth.RequireSession(r); err != nil {
			return nil, &assistantError{401, "unauthenticated", "Sign in again to read the result"}
		}
		for _, m := range thread.Messages(owner, id, 0) {
			if m.Role != thread.RoleAgent {
				continue
			}
			if m.Ref == "reply-answer:"+key {
				if err := thread.Flush(); err != nil {
					return nil, err
				}
				return &pb.AskResponse{Thread: id, AgentId: selected, Text: m.Text, Content: assistantContent(m.Results)}, nil
			}
			if m.Ref == "reply-error:"+key || m.Ref == "reply-interrupted:"+key {
				return nil, &assistantError{500, "execution_failed", m.Text}
			}
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-timer.C:
			return nil, &assistantError{504, "outcome_pending", "The request was accepted. Read your thread for its outcome; do not resend automatically."}
		case <-tick.C:
		}
	}
}
func assistantContent(items []result.Item) []*pb.Content {
	out := []*pb.Content{}
	for _, item := range items {
		fallback := strings.TrimSpace(strings.Join([]string{item.Title, item.Summary, item.Body}, "\n"))
		if fallback == "" {
			fallback = item.Kind
		}
		c := &pb.Content{Fallback: fallback}
		switch item.Kind {
		case "article":
			c.Value = &pb.Content_Source{Source: &pb.Source{Title: item.Title, Url: item.URL, Summary: item.Summary}}
		case "image":
			c.Value = &pb.Content_Image{Image: &pb.Image{Url: item.URL, Description: item.Title}}
		case "video":
			c.Value = &pb.Content_Video{Video: &pb.Video{Provider: "youtube", Id: item.ID, Url: item.URL, Title: item.Title}}
		case "route":
			v := &pb.Route{Steps: item.Steps}
			for _, p := range item.Shape {
				if !math.IsNaN(p.Lat) && !math.IsInf(p.Lat, 0) && !math.IsNaN(p.Lon) && !math.IsInf(p.Lon, 0) && p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180 {
					v.Shape = append(v.Shape, &pb.Point{Latitude: p.Lat, Longitude: p.Lon})
				}
			}
			c.Value = &pb.Content_Route{Route: v}
		default:
			c.Value = &pb.Content_Resource{Resource: &pb.Resource{Kind: item.Kind, Id: item.ID, Url: item.URL, Title: item.Title, Summary: item.Summary}}
		}
		out = append(out, c)
	}
	return out
}
