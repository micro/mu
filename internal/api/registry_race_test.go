package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestToolsCanBeRegisteredWhileAgentsReadAndExecute(t *testing.T) {
	saved := Tools()
	defer func() { toolsMu.Lock(); tools = saved; toolsMu.Unlock() }()
	RegisterToolOpen(Tool{Name: "registry_probe"}, func(context.Context, map[string]any) (string, error) {
		return "ok", nil
	})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			<-start
			for range 100 {
				if !HasTool("registry_probe") {
					t.Error("registered tool disappeared")
				}
				if _, ok := Lookup("registry_probe"); !ok {
					t.Error("lookup lost tool")
				}
				_ = Policies()
				_ = ToolDescriptions()
				_ = sortedTools()
				_ = ToolWalletOp("registry_probe")
				_ = ToolNeedsAuth("registry_probe")
				got, failed, err := ExecuteTool(httptest.NewRequest("GET", "/", nil), "registry_probe", nil)
				if err != nil || failed || got != "ok" {
					t.Errorf("execute: %q %v %v", got, failed, err)
				}
			}
		})
	}
	wg.Go(func() {
		<-start
		for i := range 100 {
			RegisterTool(Tool{Name: fmt.Sprintf("registry_added_%d", i)})
		}
	})
	close(start)
	wg.Wait()
}
