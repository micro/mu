package mesh

import (
	"go-micro.dev/v6/agent"
)

// NewAgent builds a go-micro agent wired to the in-process mesh: it shares the
// same registry, client and store, so the agent discovers the registered
// domain services (weather, news, markets, …) and calls their methods as tools
// with no hand-written glue.
//
// This is the foundation for moving mu's agent pipeline onto go-micro. It is
// additive — callers opt in; the existing agent.Query path is untouched.
//
//	a := mesh.NewAgent("assistant",
//	    "You are mu's assistant. Use the tools to answer.",
//	    "atlascloud", apiKey,
//	    []string{"weather", "news", "markets"})
//	resp, _ := a.Ask(ctx, "what's the weather in London?")
func NewAgent(name, prompt, provider, apiKey string, services []string, opts ...agent.Option) agent.Agent {
	ensure()
	base := []agent.Option{
		agent.Name(name),
		agent.Services(services...),
		agent.Prompt(prompt),
		agent.Provider(provider),
		agent.APIKey(apiKey),
		agent.WithRegistry(reg),
		agent.WithClient(cl),
		agent.WithStore(st),
	}
	return agent.New(append(base, opts...)...)
}
