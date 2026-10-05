# Architecture and development

Micro is the personal assistant; Mu is the runtime that hosts it. One Go binary
provides the web app, CLI, APIs and protocol servers over the same services.

## How the parts fit together

- **Services** own capabilities and their data. Their declared endpoints supply
  the tools available to agents and external clients.
- **Agents** combine instructions, permitted tools and a model provider to answer
  requests. Each run is subject to the caller's permissions.
- **Inbox** brings correspondence and agent conversations into one account-scoped
  view. Mail, chat and SMS retain their original service records.
- **Work** tracks delegated tasks. Execution lives under `agent/exec`; task storage
  lives under `service/tasks`. Schedules publish events that can start execution.
- **Clients** reach the same runtime through the web, CLI, HTTP, MCP and messaging
  protocols. Changing the client does not change the account's permissions.

The source tree and service registry are authoritative for implementation details.
See [Developers](/developers) for client examples and [Tools](/tools) for the
service catalogue. Deployment belongs in [Hosting](HOSTING.md); settings belong
in [Configuration](CONFIGURATION.md).

## Build from source

You need Git and Go 1.26 or later:

```sh
git clone https://github.com/micro/mu.git
cd mu
go build -o mu .
./mu --serve --address 127.0.0.1:8080
```

Open <http://localhost:8080> to complete setup.

## Extending a running instance

Mu uses go-micro to run its built-in services and agent loops. There are three
ways to build on an instance; they have different deployment requirements.

### Agents hosted by Mu

Open `/agents` while signed in and create an agent with a name, instructions,
and the services it may use. This is stored configuration: you do not fork,
recompile or restart Mu to create an agent. Mu runs its loop using the instance's
configured model providers. Talk to it from its agent page or use:

```sh
mu ask --agent research "Summarise the latest news"
```

The CLI must be logged into the intended instance. Creating an agent does not
install arbitrary executable code or start a separate operating-system process.

### Programs using Mu

Programs call Agent, Work and Inbox through JSON POST operations at `/api/v1`
or the same operations as MCP tools at `/mcp`. `/developers` documents the current
contract and `/api/v1` lists its operations. Mu owns agent execution and tool
use; clients submit goals and read results.

Create an API token at `/account/tokens`. Select the needed API capabilities; these apply
across the token owner's account. They do not isolate an application's
conversations or disable memory. Service-scoped tokens are refused at the
outcome API. The first-party service playground and sandboxed app SDK continue
to use internal service capabilities. Services tokens also select the service API on the same host through `/api/v1`
and `/mcp`. A separately configured x402 host retains its existing contract.

### Services running outside Mu

Mu can discover separately running go-micro services through mDNS. The external
service must register its RPC handlers with a go-micro mDNS registry and use a
compatible HTTP transport. Its advertised address must be reachable from Mu.
Use a unique service name that does not collide with a built-in service.

Start Mu in the corresponding discovery mode:

```sh
MU_REGISTRY=mdns mu --serve
```

Run the external service separately on the same discovery network. Mu reads
service names and endpoints from the registry and can invoke them dynamically;
adding another compatible service does not require rebuilding Mu. Discovery
of RPC methods does not supply a custom service web page.

This mode is for an isolated, trusted network. Registry entries and RPC calls
are not authenticated by this mechanism. Mu also advertises its own services,
and switching to mDNS switches its internal calls from in-memory transport to
HTTP. Do not expose these RPC listeners to an untrusted network. The event
broker remains in memory; service discovery does not establish a shared event
bus. Configure `MU_REGISTRY` in the process environment and restart Mu when
changing discovery mode.

Use your process manager, such as systemd or Docker, to start, supervise and
stop the external service. Mu currently discovers and calls it; Mu does not
install or manage its process. A separately written Go module linked into Mu
is different again: importing that module requires a rebuild. Forking is only
necessary when you want to maintain changes to Mu itself.
