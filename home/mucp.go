package home

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/agent"
	"mu/agent/brief"
	"mu/inbox"
	"mu/internal/auth"
	"mu/internal/mucp"
	pb "mu/proto/assistant/v1"
	"mu/service/events"
	"net/http"
	"sort"
	"time"
)

func MUCPOperations() []mucp.Operation {
	return []mucp.Operation{{Service: "assistant", Endpoint: "Home.Overview", Capability: "home", Request: func() proto.Message { return &pb.HomeRequest{} }, Handle: func(_ http.ResponseWriter, _ *http.Request, owner, _ string, _ proto.Message) (proto.Message, error) {
		acc, err := auth.GetAccount(owner)
		if err != nil {
			return nil, err
		}
		// Reuse Home's bounded background refresh; never wait on providers here.
		_, _ = overview(acc)
		out := &pb.HomeResponse{Inbox: inbox.ClientItems(owner, 5), PinnedServices: acc.PinnedServices(), CalendarPending: !events.OverviewFresh(owner), At: timestamppb.Now()}
		for _, a := range agent.ClientAgents(owner) {
			out.Agents = append(out.Agents, &pb.Agent{Id: a.ID, Name: a.Name, Slug: a.Slug, Description: a.Description})
		}
		for _, e := range events.Upcoming(owner) {
			minutes := e.Minutes
			if minutes <= 0 {
				minutes = 30
			}
			out.Events = append(out.Events, &pb.HomeEvent{Title: e.Title, Start: timestamppb.New(e.When), End: timestamppb.New(e.When.Add(time.Duration(minutes) * time.Minute)), Source: "Mu"})
		}
		for _, e := range events.CachedOverview(owner) {
			item := &pb.HomeEvent{Title: e.Title, Location: e.Location, Source: e.Source}
			if !e.Start.Equal(time.Time{}) {
				item.Start = timestamppb.New(e.Start)
			}
			if !e.End.Equal(time.Time{}) {
				item.End = timestamppb.New(e.End)
			}
			out.Events = append(out.Events, item)
		}
		sort.SliceStable(out.Events, func(i, j int) bool {
			return out.Events[i].GetStart().AsTime().Before(out.Events[j].GetStart().AsTime())
		})
		if len(out.Events) > 25 {
			out.Events = out.Events[:25]
		}
		if b, ok := brief.Latest(); ok {
			out.Brief = b.Text
		}
		return out, nil
	}}}
}
