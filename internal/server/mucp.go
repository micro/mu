package server

import (
	"google.golang.org/protobuf/proto"
	"mu/agent"
	"mu/home"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/mucp"
	"mu/internal/service"
	pb "mu/proto/assistant/v1"
	alertspb "mu/proto/service/alerts/v1"
	bookmarkspb "mu/proto/service/bookmarks/v1"
	chatpb "mu/proto/service/chat/v1"
	grouppb "mu/proto/service/groups/v1"
	notespb "mu/proto/service/notes/v1"
	taskspb "mu/proto/service/tasks/v1"
	"mu/service/alerts"
	"mu/service/bookmarks"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/groups"
	"mu/service/notes"
	"mu/service/tasks"
	"net/http"
	"sort"
	"strings"
)

func mucpOperations() []mucp.Operation {
	ops := append(agent.MUCPOperations(), home.MUCPOperations()...)
	ops = append(ops, inbox.MUCPOperations()...)
	ops = append(ops, contacts.MUCPOperations()...)
	ops = append(ops, contractOperations(chat.Spec, chatpb.File_service_chat_v1_chat_proto.Services().ByName("Chat"))...)
	ops = append(ops, contractOperations(groups.Spec, grouppb.File_service_groups_v1_groups_proto.Services().ByName("Groups"))...)
	ops = append(ops, contractOperations(notes.Spec, notespb.File_service_notes_v1_notes_proto.Services().ByName("Notes"))...)
	ops = append(ops, contractOperations(bookmarks.Spec, bookmarkspb.File_service_bookmarks_v1_bookmarks_proto.Services().ByName("Bookmarks"))...)
	ops = append(ops, contractOperations(alerts.Spec, alertspb.File_service_alerts_v1_alerts_proto.Services().ByName("Alerts"))...)
	ops = append(ops, contractOperations(tasks.Spec, taskspb.File_service_tasks_v1_tasks_proto.Services().ByName("Tasks"))...)
	return append(ops,
		mucp.Operation{Service: "assistant", Endpoint: "Services.List", Capability: "services", Request: func() proto.Message { return &pb.ListServicesRequest{} }, Handle: servicesCall},
		mucp.Operation{Service: "assistant", Endpoint: "Services.Describe", Capability: "services", Request: func() proto.Message { return &pb.DescribeServiceRequest{} }, Handle: servicesCall})
}
func MUCPHandler(w http.ResponseWriter, r *http.Request) { mucp.Handler(mucpOperations())(w, r) }
func serviceDescriptors() []*pb.ServiceDescriptor {
	out := []*pb.ServiceDescriptor{}
	for _, s := range service.Specs() {
		d := &pb.ServiceDescriptor{Name: s.Name, Description: s.Description}
		var direct []mucp.Operation
		switch s.Name {
		case "contacts":
			direct = contacts.MUCPOperations()
		case "bookmarks":
			direct = contractOperations(bookmarks.Spec, bookmarkspb.File_service_bookmarks_v1_bookmarks_proto.Services().ByName("Bookmarks"))
		case "alerts":
			direct = contractOperations(alerts.Spec, alertspb.File_service_alerts_v1_alerts_proto.Services().ByName("Alerts"))
		case "tasks":
			direct = contractOperations(tasks.Spec, taskspb.File_service_tasks_v1_tasks_proto.Services().ByName("Tasks"))
		case "notes":
			direct = contractOperations(notes.Spec, notespb.File_service_notes_v1_notes_proto.Services().ByName("Notes"))
		case "chat":
			direct = contractOperations(chat.Spec, chatpb.File_service_chat_v1_chat_proto.Services().ByName("Chat"))
		case "groups":
			direct = contractOperations(groups.Spec, grouppb.File_service_groups_v1_groups_proto.Services().ByName("Groups"))
		}
		if len(direct) > 0 {
			d.Contract = "mu.service." + s.Name + ".v1"
			for _, op := range direct {
				_, method, _ := strings.Cut(op.Endpoint, ".")
				e := s.Endpoints[method]
				d.Operations = append(d.Operations, &pb.ServiceOperation{Endpoint: op.Endpoint, Description: e.Doc, Writes: op.Writes, Destructive: e.Destructive})
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func servicesCall(_ http.ResponseWriter, _ *http.Request, owner, _ string, q proto.Message) (proto.Message, error) {
	items := serviceDescriptors()
	switch q := q.(type) {
	case *pb.ListServicesRequest:
		page, next, err := mucp.Page(items, q.PageSize, q.PageToken, owner+"/services/list")
		return &pb.ListServicesResponse{Services: page, NextPageToken: next}, err
	case *pb.DescribeServiceRequest:
		for _, s := range items {
			if s.Name == q.Name {
				return s, nil
			}
		}
		return nil, api.Fail(404, "not_found", "Service not found")
	}
	return nil, api.Fail(400, "invalid_arguments", "Unknown service discovery request")
}
