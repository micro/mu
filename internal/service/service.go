// Package service is mu's go-micro runtime core.
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
package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"go-micro.dev/v6/broker"
	"go-micro.dev/v6/client"
	gwmcp "go-micro.dev/v6/gateway/mcp"
	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
	"go-micro.dev/v6/server"
	gomicro "go-micro.dev/v6/service"
	"go-micro.dev/v6/store"
	"go-micro.dev/v6/transport"
)

func init() {
	// In-process services advertise loopback and are reached over loopback.
	// If an HTTP(S)_PROXY is configured, Go's transport would otherwise route
	// those loopback dials through the proxy, which hijacks them. Ensure
	// loopback always bypasses the proxy. Runs at package init, before any
	// request is made, so the proxy-env cache reads the updated value.
	bypassProxyForLoopback()
}

func bypassProxyForLoopback() {
	const loopback = "127.0.0.1,localhost,::1,0.0.0.0"
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		cur := os.Getenv(key)
		if cur == "" {
			os.Setenv(key, loopback)
			continue
		}
		if !strings.Contains(cur, "127.0.0.1") {
			os.Setenv(key, cur+","+loopback)
		}
	}
}

var (
	mu       sync.Mutex
	inited   bool
	reg      registry.Registry
	cl       client.Client
	br       broker.Broker
	st       store.Store
	tr       transport.Transport
	services []gomicro.Service
)

// Init builds the shared go-micro infrastructure. It is idempotent and safe to
// call from multiple Load() functions; the first call wins.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if inited {
		return
	}
	// MU_REGISTRY selects how services find each other, and so whether a
	// service can live outside this process at all.
	//
	//	unset / "memory"  — everything in one binary. The default: no ports, no
	//	                    daemon, nothing to configure, and nothing external
	//	                    can register.
	//	"mdns"            — zero-config discovery on the local network. A
	//	                    service run as its own process from any repo shows
	//	                    up in Mu without Mu importing it.
	//
	// mdns is not a free upgrade, and is not the default for two reasons.
	// Discovery is symmetric: Mu does not only listen, it announces. Every
	// service this process hosts — wallet, mail, db — is published on the
	// local network and reachable by anything on it, and nothing
	// authenticates an RPC. And every internal call becomes an HTTP round
	// trip (~400µs) instead of a channel send, several times per page render.
	// Turn it on for a trusted network where an external service is actually
	// wanted; leave it off otherwise.
	//
	// etcd, consul and nats registries exist in go-micro subpackages and are
	// deliberately not imported: each drags a substantial dependency tree into
	// a binary that mostly runs standalone. Add one here when an instance needs
	// discovery beyond a single network.
	//
	// Anything other than memory means calls cross a process boundary, so the
	// in-memory transport cannot be used.
	//
	// Note nothing authenticates a registry entry. On a shared registry,
	// anything that can reach it can register under any name — see the guard in
	// Services(), which keeps a remote node from taking over a local one, and
	// treat the network itself as the trust boundary until there is a real one.
	switch spec := registrySpec(); spec {
	case "", "memory":
		reg = registry.NewMemoryRegistry()
		// In-memory transport: services are in-process, so calls pass messages
		// over channels instead of a loopback TCP/HTTP socket. Client and every
		// server must share this one instance. This is what keeps a
		// service.Call cheap (single-digit µs) rather than a ~400µs HTTP
		// round-trip.
		tr = transport.NewMemoryTransport()
	case "mdns":
		reg = registry.NewMDNSRegistry()
		tr = transport.NewHTTPTransport()
	default:
		log.Printf("service: unknown MU_REGISTRY %q, falling back to in-process", spec)
		reg = registry.NewMemoryRegistry()
		tr = transport.NewMemoryTransport()
	}
	br = broker.NewMemoryBroker()
	_ = br.Connect()
	cl = client.NewClient(
		client.Registry(reg),
		client.Selector(selector.NewSelector(selector.Registry(reg))),
		client.Broker(br),
		client.Transport(tr),
	)
	st = newDurableStore()
	inited = true
}

// registrySpec reads the registry selection. Env only: it is needed during
// Init, before the settings store is loaded.
func registrySpec() string {
	return strings.TrimSpace(strings.ToLower(os.Getenv("MU_REGISTRY")))
}

// Advertise is the address an out-of-process service should be reachable on.
// In-process services advertise loopback with an ephemeral port; when the
// registry is networked, that address has to be one other hosts can dial.
func advertiseAddress() string {
	if addr := strings.TrimSpace(os.Getenv("MU_ADVERTISE")); addr != "" {
		return addr
	}
	if spec := registrySpec(); spec != "" && spec != "memory" {
		return ":0" // all interfaces, ephemeral port
	}
	return "127.0.0.1:0"
}

// newDurableStore returns a file-backed store (bbolt under ~/.mu/store) so data
// written through the shared store — snapshots today, durable events next —
// survives a restart with no external infrastructure. Falls back to an
// in-memory store if the directory can't be created.
func newDurableStore() store.Store {
	dir := os.ExpandEnv("$HOME/.mu/store")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return store.NewMemoryStore()
	}
	return store.NewFileStore(store.DirOption(dir))
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

// Register stands up an in-process go-micro service from its Spec and starts
// it. The handler's exported methods of the form func(ctx, *Req, *Rsp) error
// become RPC endpoints — and, via the agent and gateways, AI tools.
//
// The Spec is the single declaration of the service: name, handler, what each
// endpoint does and costs, where it appears, who may reach it. Every surface
// derives from it rather than keeping its own list. See spec.go.
//
// It returns once the service is registered and reachable.
func Register(spec Spec) error {
	ensure()
	if spec.Name == "" {
		return fmt.Errorf("service: Spec.Name is required")
	}
	if spec.Handler == nil {
		return fmt.Errorf("service: %s has no Handler", spec.Name)
	}
	svc, err := startService(spec)
	if err != nil {
		return err
	}
	recordSpec(spec)
	mu.Lock()
	services = append(services, svc)
	mu.Unlock()
	return nil
}

// startAttempts is how many times Register will re-pick an address.
//
// The in-process transport does not bind a socket; it invents an address and
// keeps a map of them, so two services can be handed the same one and the
// second fails with "already listening on 127.0.0.1:15804". It is a collision
// between random numbers, not a port in use, which is why the fix is to try
// again rather than to wait.
//
// Rare, and rare is the problem: an instance registers around twenty-five
// services at boot, so the odds are small per service and not small across a
// fleet of restarts — and the failure is a service that silently is not there.
// It surfaced first as a flaky test, which is the cheap way to find out.
const startAttempts = 5

func startService(spec Spec) (gomicro.Service, error) {
	var lastErr error
	for attempt := 0; attempt < startAttempts; attempt++ {
		// A fresh Service each time: the address is chosen here, so retrying
		// the same instance would retry the same collision.
		svc := gomicro.New(
			gomicro.Name(spec.Name),
			gomicro.Address(advertiseAddress()), // loopback in-process; routable when networked
			gomicro.Registry(reg),
			gomicro.Client(cl),
			gomicro.Broker(br),
			gomicro.Transport(tr),
		)
		var opts []server.HandlerOption
		if o := endpointOptions(spec); o != nil {
			opts = append(opts, o)
		}
		if err := svc.Handle(spec.Handler, opts...); err != nil {
			return nil, err
		}
		if err := svc.Start(); err != nil {
			lastErr = err
			if isAddressTaken(err) {
				continue
			}
			return nil, err
		}
		return svc, nil
	}
	return nil, fmt.Errorf("service: %s could not get a free address in %d attempts: %w",
		spec.Name, startAttempts, lastErr)
}

// isAddressTaken reports whether an error is the transport's address collision
// rather than a real failure to start. Matched on the message because the
// transport returns a bare error with nothing else to match on.
func isAddressTaken(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already listening")
}

// HandlerOpts registers handlers with go-micro server options (e.g. endpoint
// metadata). Most callers want Register.
func HandlerOpts(name string, h any, opts ...server.HandlerOption) error {
	ensure()
	svc := gomicro.New(gomicro.Name(name), gomicro.Registry(reg), gomicro.Client(cl), gomicro.Broker(br), gomicro.Transport(tr))
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
//	service.Call(ctx, "weather", "Weather.Forecast", &weather.ForecastRequest{...}, &rsp)
func Call(ctx context.Context, svcName, endpoint string, req, rsp any) error {
	ensure()
	return cl.Call(ctx, cl.NewRequest(svcName, endpoint, req), rsp)
}

// StartMCPGateway runs go-micro's MCP gateway on addr (e.g. ":4100"),
// exposing every registered service's methods as MCP tools — schemas derived
// from the handler signatures and @example doc-comments. It blocks; run it in a
// goroutine. This is additive: it stands the framework's gateway up alongside
// mu's existing /mcp for comparison, on its own port.
func StartMCPGateway(addr string) error {
	ensure()
	return gwmcp.Serve(gwmcp.Options{
		Registry: reg,
		Client:   cl,
		Address:  addr,
	})
}

// Services returns the names of the registered in-process go-micro services.
func Services() []string {
	ensure()

	mu.Lock()
	local := make(map[string]bool, len(services))
	for _, s := range services {
		local[s.Name()] = true
	}
	mu.Unlock()

	// Read the registry, not just what this process hosts. With the default
	// in-memory registry those are the same set. With a networked one they are
	// not: a service running as its own process — from micro/xyz, or anything
	// else — registers itself and appears here without Mu importing it or
	// knowing it exists. Every surface downstream (agent tools, the picker, the
	// app SDK, status) reads this function, so discovery lands everywhere at
	// once.
	seen := map[string]bool{}
	out := make([]string, 0, len(local))
	for name := range local {
		seen[name] = true
		out = append(out, name)
	}

	if svcs, err := reg.ListServices(); err == nil {
		for _, s := range svcs {
			name := s.Name
			if name == "" || seen[name] {
				continue
			}
			// A remote node must not take over a name this process hosts.
			// Nothing authenticates a registry entry, so on a shared registry
			// anything reachable could otherwise claim "mail" or "wallet".
			// Local always wins, and the collision is worth knowing about.
			if local[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}

	sort.Strings(out)
	return out
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
