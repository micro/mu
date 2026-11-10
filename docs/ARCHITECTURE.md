# Architecture and development

Mu is an open runtime for personal assistants. Micro is its general-purpose agent;
other agents can have different instructions and permitted tools. Go Micro is the
underlying framework. Mu combines agents, services, accounts and persistent history
into a running system that clients can use.

## Client, protocol and server

```text
 CLIENT                       BOUNDARY                MU RUNTIME
 +-------------------+                                +-------------------+
 | Android           |                                | Authentication    |
 | Web client        | -- assistant requests --------> | + authorisation   |
 | Future clients    |    MUCP / HTTPS / JSON (*)      +---------+---------+
 |                   |                                          |
 | Text, voice,      |                                +---------v---------+
 | result rendering  | <--- answers + typed results -- | Assistant entry   |
 +-------------------+                                | in agent/         |
                                                      +---------+---------+
                                                                |
                                                      +---------v---------+
                                                      | Selected agent    |
                                                      | + thread history  |
                                                      +---------+---------+
                                                                |
                                                      +---------v---------+
                                                      | Permitted tools   |
                                                      +---------+---------+
                                                                |
                                                      +---------v---------+
                                                      | Services          |
                                                      | mail, events, ... |
                                                      +-------------------+
```

(*) Android and the focused client in `client/web` use the MUCP HTTPS/JSON
binding for agents, threads and replies. The React/TypeScript web client runs independently and connects to a selected
runtime using OAuth/PKCE and explicit tokens. Mu can also serve its build at `/app/`.
The PWA belongs to the full runtime web experience, not the standalone client.
The server uses go-micro with an in-memory transport internally.
The web console is server-rendered composition, not currently an independent MUCP
client. It remains useful for overview, administration and direct service use.

## Layers and ownership

| Layer / package | Responsibility |
| --- | --- |
| `client/` and standalone web client | Input, voice capture, playback and safe result rendering |
| `proto/` | Shared interface/type definitions; not runtime implementation |
| `internal/server` | Compose routes, authenticated protocol entry points and product pages |
| `agent/` | Assistant operations, agent selection and execution; uses permitted services |
| `service/` | Bottom-layer capabilities and their data; independently callable through authorised interfaces |
| `internal/` | Shared substrate including identity, threads, quota and storage |
| Home / Inbox / Agents / Services views | Web composition over those capabilities |

The assistant is the client-facing interface to agents, not an additional service
beneath them. There is no `service/assistant` wrapper calling upward into `agent/`.
A protobuf `service Assistant` is an RPC declaration, not a package-layer rule.
Services do not call each other or launch agent reasoning; they publish facts and
higher-level subscribers decide what work to run. Server composition may reach
both layers; that does not authorise upward dependencies from services.

`proto/assistant/v1` owns the assistant contract. Future capability contracts
may live under `proto/service/<name>/v1`; wholesale service extraction is deferred
until the assistant binding demonstrates interoperability. The existing service
Specs and implementation types remain authoritative for current APIs and tools.

## One assistant interaction

```text
 Client                 Assistant / agent                  Services
   |                            |                              |
   | list agents, list threads  |                              |
   |--------------------------->|                              |
   | descriptors, thread IDs    |                              |
   |<---------------------------|                              |
   | ask(agent, thread, prompt) |                              |
   |--------------------------->| authenticate / check scope   |
   |                            | resolve agent + history      |
   |                            | reason / choose tools        |
   |                            | permitted capability call    |
   |                            |----------------------------->|
   |                            | tool result                  |
   |                            |<-----------------------------|
   |                            | finish answer, retain turn   |
   | text + content + thread    |                              |
   |<---------------------------|                              |
   | render / speak             |                              |
```

The `/` adapter implements discovery, personal thread reads and Ask. Existing execution
already resolves agents, checks permissions, uses tools and retains personal
threads. `thread` is the existing identifier for what the UI calls a conversation;
there is no second conversation-ID namespace. A request ID identifies an exchange,
not the persistent thread. Listing and reading must project authorised personal
assistant threads, not dump mail, other users' records or internal execution logs.

Clients render typed content as data. A video reference is not proof of playback
and does not override provider restrictions. Stopping speech is local and immediate;
cancelling server work is separate, best effort and not yet defined in the draft.

## Cross-cutting guarantees

Authentication establishes account identity at the boundary; caller-supplied IDs
or metadata cannot establish it. Agent instructions and tool permissions are
resolved on the server. Quotas, billing and service authorisation remain enforced
when requests arrive through another client or encoding. Public-question mode is
explicitly restricted and must never silently become a personal request.

`internal/thread` stores conversation history. Inbox projects incoming messages
and notifications; it is not a second execution engine. `agent/exec` owns delegated
execution and `agent/work` its product view; `service/tasks` retains task records.
Transport retries do not imply idempotency, and a lost connection does not prove
that an action failed. Clients must not automatically repeat ambiguous mutations.

Self-hosting controls the runtime but does not make external model calls local.
Data boundaries must hold when tools access private sources and when information
is sent to configured model providers. MUCP does not itself promise federation,
end-to-end encryption, offline execution or portability of stored account data.

See [MUCP](MUCP.md) for the contract/transport distinction and implementation status.
Deployment belongs in [Hosting](HOSTING.md); settings in [Configuration](CONFIGURATION.md).

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
