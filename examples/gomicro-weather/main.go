// Command gomicro-weather is a spike: mu's "weather" capability rebuilt as a
// go-micro v6 service, to answer one question — can mu's services and agents
// become go-micro services and agents cleanly?
//
// It needs no API key and is deterministic. Against the real framework
// (go-micro.dev/v6) it demonstrates:
//
//  1. A plain Go method with a doc-comment IS the conversion of a mu service.
//  2. A direct typed RPC call to that service works.
//  3. go-micro auto-derives an AI tool (name + JSON schema) from the method
//     and its @example comment — zero hand-written tool registration. This is
//     what replaces mu's internal/api/mcp.go tool registry.
//  4. With an LLM key, a real go-micro agent over the service answers a
//     natural-language question end to end — discovering the tool, calling it,
//     and synthesising the result — with no tool glue written.
//
// This is a SEPARATE Go module (its own go.mod), so it does not add
// go-micro.dev/v6 to mu's main binary. It is a throwaway proof, not product.
// The real conversion would have Forecast call mu/weather.FetchWeather.
//
// Run: go run .
// Note: under an HTTP(S)_PROXY, set NO_PROXY to include the service's
// advertised IP, or loopback RPC is hijacked by the proxy (see README).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go-micro.dev/v6/agent"
	"go-micro.dev/v6/ai"
	"go-micro.dev/v6/client"
	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
	"go-micro.dev/v6/service"
	"go-micro.dev/v6/store"
)

// --- The weather service: this is the whole "conversion" of mu/weather. ---
//
// In mu today this is a package function (weather.ForecastText) plus a tool
// hand-registered in internal/api/mcp.go. Here it is just a method; go-micro
// turns the method into the tool.

type ForecastRequest struct {
	Location string  `json:"location" description:"Place name, e.g. London"`
	Lat      float64 `json:"lat" description:"Latitude (optional)"`
	Lon      float64 `json:"lon" description:"Longitude (optional)"`
}

type ForecastResponse struct {
	Summary string `json:"summary" description:"Human-readable weather summary"`
}

// Weather answers weather questions for a location.
type Weather struct{}

// Forecast returns a short weather summary for a place (by name, or lat/lon).
// @example {"location": "London"}
func (Weather) Forecast(_ context.Context, req *ForecastRequest, rsp *ForecastResponse) error {
	// Real conversion: rsp.Summary = weather.ForecastText(req.Lat, req.Lon).
	rsp.Summary = sampleForecast(req.Location)
	return nil
}

func main() {
	// Shared in-process infrastructure: no network, no separate processes.
	// The key point for mu: adopting go-micro does NOT force the monolith to
	// physically split — the whole fleet can run in one binary.
	reg := registry.NewMemoryRegistry()
	cl := client.NewClient(client.Registry(reg), client.Selector(selector.NewSelector(selector.Registry(reg))))

	svc := service.New(service.Name("weather"), service.Registry(reg), service.Client(cl))
	if err := svc.Handle(new(Weather)); err != nil {
		fmt.Println("handle:", err)
		os.Exit(1)
	}
	go svc.Run()
	defer svc.Stop()
	waitFor(reg, "weather")

	fmt.Println("\n\033[1mgo-micro weather spike\033[0m")

	// 1) The service works as a normal go-micro RPC service.
	fmt.Println("\n[1] Direct typed RPC call")
	var rsp ForecastResponse
	if err := cl.Call(context.Background(), cl.NewRequest("weather", "Weather.Forecast", &ForecastRequest{Location: "London"}), &rsp); err != nil {
		fmt.Println("   call error:", err)
	} else {
		fmt.Printf("   OK %s\n", firstLine(rsp.Summary))
	}

	// 2) go-micro auto-derives an AI tool from the method. No registration.
	fmt.Println("\n[2] Auto-discovered AI tool (from the method + @example)")
	tools := ai.NewTools(reg, ai.ToolClient(cl))
	discovered, err := tools.Discover()
	if err != nil {
		fmt.Println("   discover error:", err)
	}
	for _, t := range discovered {
		props, _ := json.Marshal(t.Properties)
		fmt.Printf("   tool %q\n     desc: %s\n     schema: %s\n", t.Name, t.Description, props)
	}

	// 3) Execute via the agent's own executor (ai.Tools.Handler) — the exact
	//    path an agent uses to call a discovered service tool.
	fmt.Println("\n[3] Execute the tool the way an agent does")
	var toolName string
	for _, t := range discovered {
		if t.OriginalName == "weather.Weather.Forecast" {
			toolName = t.Name
		}
	}
	res := tools.Handler()(context.Background(), ai.ToolCall{
		ID:    "1",
		Name:  toolName,
		Input: map[string]any{"location": "London"},
	})
	fmt.Printf("   result content: %s\n", firstLine(res.Content))

	// 4) Optional: a real agent over the service, end to end. Needs an LLM
	//    key. The agent discovers the tool (step 2), calls it (step 3), and
	//    synthesises an answer from the result — no tool glue written.
	if key := os.Getenv("ATLAS_API_KEY"); key != "" {
		fmt.Println("\n[4] Agent end to end (atlascloud)")
		runAgent(reg, cl, key)
	} else {
		fmt.Println("\n[4] Agent end to end — skipped (set ATLAS_API_KEY to run)")
	}
}

func runAgent(reg registry.Registry, cl client.Client, key string) {
	a := agent.New(
		agent.Name("assistant"),
		agent.Services("weather"),
		agent.Prompt("You are helpful. Use the weather tools, then answer the user in plain language."),
		agent.Provider("atlascloud"), agent.APIKey(key), agent.Model("deepseek-ai/deepseek-v4-pro"),
		agent.WithRegistry(reg), agent.WithClient(cl), agent.WithStore(store.NewMemoryStore()),
	)
	go a.Run()
	defer a.Stop()
	time.Sleep(300 * time.Millisecond)

	q := "What's the weather in London right now and over the next few days?"
	fmt.Printf("   Q: %s\n", q)
	resp, err := a.Ask(context.Background(), q)
	if err != nil {
		fmt.Println("   ask error:", err)
		return
	}
	fmt.Printf("   A: %s\n", resp.Reply)
}

func waitFor(reg registry.Registry, names ...string) {
	deadline := time.Now().Add(5 * time.Second)
	for _, name := range names {
		for time.Now().Before(deadline) {
			if svcs, err := reg.GetService(name); err == nil && len(svcs) > 0 && len(svcs[0].Nodes) > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// sampleForecast returns deterministic sample weather, standing in for
// mu/weather.FetchWeather so the spike has no external dependency.
func sampleForecast(location string) string {
	if location == "" {
		location = "your area"
	}
	return "Weather for " + location + ".\nNow: 16C, humidity 65%, wind 18 km/h, overcast.\n" +
		"Next days:\nMon: 11-17C, rain 2mm\nTue: 12-18C, dry\nWed: 10-16C, rain 4mm"
}
