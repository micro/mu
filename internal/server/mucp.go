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
	"mu/service/contacts"
	"net/http"
	"sort"
	"strings"
)

func mucpOperations() []mucp.Operation {
	ops := append(agent.MUCPOperations(), home.MUCPOperations()...)
	ops = append(ops, inbox.MUCPOperations()...)
	ops = append(ops, contacts.MUCPOperations()...)
	return append(ops,
		mucp.Operation{Service: "assistant", Endpoint: "Services.List", Capability: "services", Request: func() proto.Message { return &pb.ListServicesRequest{} }, Handle: servicesCall},
		mucp.Operation{Service: "assistant", Endpoint: "Services.Describe", Capability: "services", Request: func() proto.Message { return &pb.DescribeServiceRequest{} }, Handle: servicesCall})
}
func MUCPHandler(w http.ResponseWriter, r *http.Request) { mucp.Handler(mucpOperations())(w, r) }
func serviceDescriptors() []*pb.ServiceDescriptor {
	out := []*pb.ServiceDescriptor{}
	for _, s := range service.Specs() {
		d := &pb.ServiceDescriptor{Name: s.Name, Description: s.Description}
		if s.Name == "contacts" {
			d.Contract = "mu.service.contacts.v1"
			for _, op := range contacts.MUCPOperations() {
				e := s.Endpoints[strings.TrimPrefix(op.Endpoint, "Contacts.")]
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
