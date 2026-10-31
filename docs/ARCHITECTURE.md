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

External mDNS discovery is disabled. Mu uses in-process service transport even
when `MU_REGISTRY=mdns` is set, and logs that the setting was refused. The old
network RPC mode accepted unauthenticated account metadata. External services
need authenticated, account-scoped transport before discovery can be restored.
