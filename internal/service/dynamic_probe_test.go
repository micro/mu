package service

import (
	"context"
	"testing"
)

// SDKProbe* must be exported: go-micro's RPC router only accepts exported
// handler types with exported argument types.
type SDKProbeReq struct {
	Name string `json:"name"`
}
type SDKProbeRsp struct {
	Greeting string `json:"greeting"`
}
type SDKProbeHandler struct{}

func (SDKProbeHandler) Hello(_ context.Context, req *SDKProbeReq, rsp *SDKProbeRsp) error {
	rsp.Greeting = "hello " + req.Name
	return nil
}

// Can a caller invoke a registered service with an untyped map request and
// decode into an untyped map response? That is what a generic SDK dispatcher
// needs: it knows the service and method by name only, never the Go types.
func TestDynamicMapDispatch(t *testing.T) {
	if err := Register("sdkprobe", SDKProbeHandler{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := map[string]any{"name": "world"}
	var rsp map[string]any
	if err := Call(context.Background(), "sdkprobe", "SDKProbeHandler.Hello", &req, &rsp); err != nil {
		t.Fatalf("dynamic call failed: %v", err)
	}
	if got := rsp["greeting"]; got != "hello world" {
		t.Fatalf("greeting = %v, want %q (full rsp: %+v)", got, "hello world", rsp)
	}
}

// The registry must expose registered services and their endpoints, so the SDK
// surface can be derived from the registry rather than hardcoded.
func TestRegistryExposesEndpoints(t *testing.T) {
	if err := Register("sdkprobe2", SDKProbeHandler{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ensure()
	svcs, err := reg.GetService("sdkprobe2")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	var found bool
	for _, s := range svcs {
		for _, e := range s.Endpoints {
			if e.Name == "SDKProbeHandler.Hello" {
				found = true
			}
		}
	}
	if !found {
		t.Error("SDKProbeHandler.Hello not found in registry endpoints")
	}
}
