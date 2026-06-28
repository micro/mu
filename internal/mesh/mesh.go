// Package mesh is mu's go-micro runtime core.
//
// It owns the shared go-micro infrastructure — registry, client, broker,
// store — and hosts mu's domain capabilities as in-process go-micro services.
// Every domain (news, markets, weather, …) registers a handler here; the HTTP
// layer and the agent reach those capabilities by calling the service through
// this package, so go-micro is the spine and HTTP is only a front.
//
// Services run in-process behind an in-memory registry: adopting go-micro does
// not force mu to physically distribute. The same handlers can later be split
// into separate processes by swapping the registry, with no handler changes.
package mesh

import (
	"context"
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/client"
	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
	"go-micro.dev/v6/server"
	"go-micro.dev/v6/service"
	"go-micro.dev/v6/store"
)

var (
	mu       sync.Mutex
	inited   bool
	reg      registry.Registry
	cl       client.Client
	br       broker.Broker
	st       store.Store
	services []service.Service
)

// Init builds the shared go-micro infrastructure. It is idempotent and safe to
// call from multiple Load() functions; the first call wins.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if inited {
		return
	}
	reg = registry.NewMemoryRegistry()
	br = broker.NewMemoryBroker()
	_ = br.Connect()
	cl = client.NewClient(
		client.Registry(reg),
		client.Selector(selector.NewSelector(selector.Registry(reg))),
		client.Broker(br),
	)
	st = store.NewMemoryStore()
	inited = true
}

func ensure() {
	if !inited {
		Init()
	}
}

// Registry returns the shared service registry.
func Registry() registry.Registry { ensure(); return reg }

// Client returns the shared RPC client.
func Client() client.Client { ensure(); return cl }

// Broker returns the shared message broker.
func Broker() broker.Broker { ensure(); return br }

// Store returns the shared key-value store.
func Store() store.Store { ensure(); return st }

// Register stands up an in-process go-micro service of the given name hosting
// the provided handler structs, and starts it. Each handler's exported methods
// of the form func(ctx, *Req, *Rsp) error become RPC endpoints — and, via the
// agent and gateways, AI tools.
//
// It returns once the service is registered and reachable.
func Register(name string, handlers ...any) error {
	ensure()
	svc := service.New(
		service.Name(name),
		service.Registry(reg),
		service.Client(cl),
		service.Broker(br),
	)
	for _, h := range handlers {
		if err := svc.Handle(h); err != nil {
			return err
		}
	}
	if err := svc.Start(); err != nil {
		return err
	}
	mu.Lock()
	services = append(services, svc)
	mu.Unlock()
	return nil
}

// HandlerOpts registers handlers with go-micro server options (e.g. endpoint
// metadata). Most callers want Register.
func HandlerOpts(name string, h any, opts ...server.HandlerOption) error {
	ensure()
	svc := service.New(service.Name(name), service.Registry(reg), service.Client(cl), service.Broker(br))
	if err := svc.Handle(h, opts...); err != nil {
		return err
	}
	if err := svc.Start(); err != nil {
		return err
	}
	mu.Lock()
	services = append(services, svc)
	mu.Unlock()
	return nil
}

// Call invokes a service endpoint with typed request/response values.
//
//	var rsp weather.ForecastResponse
//	mesh.Call(ctx, "weather", "Weather.Forecast", &weather.ForecastRequest{...}, &rsp)
func Call(ctx context.Context, svcName, endpoint string, req, rsp any) error {
	ensure()
	return cl.Call(ctx, cl.NewRequest(svcName, endpoint, req), rsp)
}

// Stop shuts down all hosted services. Used on graceful shutdown.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	for _, s := range services {
		_ = s.Stop()
	}
	services = nil
}
