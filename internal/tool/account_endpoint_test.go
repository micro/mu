package tool

// A method that needs a caller gets one, even on a service that does not.
//
// Scoped is per service and answers "is any of this private". Some open
// services have one method that still needs to know who is asking: chat is
// readable by a guest and posting to it is not. A derived tool on an unscoped
// service was dispatched with a hard-coded empty account, so such a method
// could never work — it read no caller and refused its own call.
//
// chat_send did exactly that from the day it shipped: over MCP, with a valid
// token, it answered "sign in to post to a discussion". stream_post only
// escaped because a hand-written registration happens to override the derived
// one and forwards the session.

import (
	"context"
	"testing"

	"mu/internal/api"

	"mu/internal/service"
)

type OpenProbe struct{}

type OpenReq struct {
	Content string `json:"content" description:"What to say"`
}
type OpenRsp struct {
	Text string `json:"text"`
}

func (OpenProbe) List(_ context.Context, _ *OpenReq, rsp *OpenRsp) error { return nil }
func (OpenProbe) Post(_ context.Context, _ *OpenReq, rsp *OpenRsp) error { return nil }

// toolNamed finds a registered tool.
func toolNamed(name string) (api.Tool, bool) {
	for _, t := range api.Tools() {
		if t.Name == name {
			return t, true
		}
	}
	return api.Tool{}, false
}

func TestAnAccountEndpointOnAnOpenServiceIsBoundToItsCaller(t *testing.T) {
	spec := service.Spec{
		Name:    "openthing",
		Handler: new(OpenProbe),
		// Not scoped: anyone may read it.
		Endpoints: map[string]service.Endpoint{
			"List": {Doc: "read it"},
			"Post": {Doc: "write to it", Needs: service.Caller},
		},
	}
	if _, already := service.SpecFor("openthing"); !already {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	DeriveTools()

	read, ok := toolNamed("openthing_list")
	if !ok {
		t.Fatal("the read tool was not derived")
	}
	if read.HandleAuth != nil || read.HandleCall != nil {
		t.Error("an open read was registered as auth-only, which puts an account behind news and weather")
	}

	write, ok := toolNamed("openthing_post")
	if !ok {
		t.Fatal("the write tool was not derived")
	}
	if write.HandleAuth == nil && write.HandleCall == nil {
		t.Error("a method declaring Account was registered with a hard-coded empty caller, " +
			"so it will refuse every call it receives")
	}
}

// The services this exists for declare it, so the bug cannot come back by
// somebody removing the flag rather than the mechanism.
func TestThePostingMethodsDeclareTheyNeedACaller(t *testing.T) {
	for _, c := range []struct{ svc, method string }{
		{"chat", "Send"},
		{"stream", "Post"},
	} {
		spec, ok := service.SpecFor(c.svc)
		if !ok {
			continue // not linked into this test binary
		}
		ep, ok := spec.Endpoints[c.method]
		if !ok {
			t.Errorf("%s has no %s endpoint", c.svc, c.method)
			continue
		}
		if !spec.Scoped && ep.Needs == service.Open {
			t.Errorf("%s.%s writes as the caller on an unscoped service but does not "+
				"declare Account, so it is dispatched with no caller and refuses itself",
				c.svc, c.method)
		}
	}
}
