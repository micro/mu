package agent

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"mu/internal/pay"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
	"net/http"
	"sort"
	"strings"
)

// AppRequest identifies exactly the customer API, never creator settings or HTML.
func AppRequest(r *http.Request) bool {
	p, ok := strings.CutPrefix(r.URL.Path, "/agent/")
	if !ok {
		return false
	}
	parts := strings.Split(p, "/")
	if len(parts) < 3 || parts[1] != "app" || reservedName(parts[0]) {
		return false
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "profile", "sessions":
			return r.Method == "GET"
		case "threads", "history", "chat":
			return r.Method == "POST"
		}
	}
	return len(parts) == 4 && parts[2] == "session" && parts[3] != "" && (r.Method == "GET" || r.Method == "POST")
}
func appProto(w http.ResponseWriter, v proto.Message) {
	raw, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(v)
	if err != nil {
		http.Error(w, "Could not encode response", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}
func appClientHandler(w http.ResponseWriter, r *http.Request, a *Agent, customer, route string) bool {
	switch {
	case r.Method == "GET" && route == "profile":
		appProto(w, &pb.AppProfile{Id: a.ID, Slug: Slug(a), Name: appName(a), Description: a.App.Description, Paid: a.App.Paid})
		return true
	case r.Method == "GET" && route == "sessions":
		sessions, err := pay.ForCustomer(a.Owner, a.ID, customer)
		if err != nil {
			http.Error(w, "Sessions unavailable", 503)
		} else {
			appProto(w, &pb.AppSessions{Sessions: sessions})
		}
		return true
	case r.Method == "POST" && route == "threads":
		var q pb.ListThreadsRequest
		if decodeAppProto(w, r, &q) != nil {
			http.Error(w, "Invalid request", 400)
			return true
		}
		n, err := pageSize(q.PageSize, 25)
		scope := pageScope(customer, "app-threads", a.ID)
		p, e := pagePosition(q.PageToken, scope)
		if err != nil || e != nil || (q.AgentId != "" && q.AgentId != a.ID) {
			http.Error(w, "Invalid page", 400)
			return true
		}
		all := thread.List(customer, 0)
		sort.Slice(all, func(i, j int) bool {
			if all[i].Updated.Equal(all[j].Updated) {
				return all[i].ID < all[j].ID
			}
			return all[i].Updated.After(all[j].Updated)
		})
		out := &pb.ListThreadsResponse{}
		for _, t := range all {
			if t.Client != "agent-app" || t.Agent != a.ID {
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
			out.Threads = append(out.Threads, &pb.Thread{Id: t.ID, AgentId: a.ID, Subject: t.Subject, Started: timestamppb.New(t.Started), Updated: timestamppb.New(t.Updated)})
		}
		appProto(w, out)
		return true
	case r.Method == "POST" && route == "history":
		var q pb.ReadThreadRequest
		if decodeAppProto(w, r, &q) != nil {
			http.Error(w, "Invalid request", 400)
			return true
		}
		t := appThread(customer, a.ID, q.Thread)
		if t == nil {
			http.NotFound(w, r)
			return true
		}
		n, err := pageSize(q.PageSize, 50)
		scope := pageScope(customer, "app-messages", a.ID+":"+t.ID)
		p, e := pagePosition(q.PageToken, scope)
		if err != nil || e != nil {
			http.Error(w, "Invalid page", 400)
			return true
		}
		all := thread.Messages(customer, t.ID, 0)
		sort.Slice(all, func(i, j int) bool {
			if all[i].At.Equal(all[j].At) {
				return all[i].ID < all[j].ID
			}
			return all[i].At.Before(all[j].At)
		})
		out := &pb.ReadThreadResponse{Thread: &pb.Thread{Id: t.ID, AgentId: a.ID, Subject: t.Subject}}
		for _, m := range all {
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
			out.Messages = append(out.Messages, &pb.Message{Id: m.ID, Role: role, Text: m.Text, At: timestamppb.New(m.At)})
		}
		appProto(w, out)
		return true
	}
	return false
}

func decodeAppProto(w http.ResponseWriter, r *http.Request, v proto.Message) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if err != nil {
		return err
	}
	return protojson.Unmarshal(raw, v)
}
