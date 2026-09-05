# Mu as a service operating system

## Thesis

The next computer is ambient: it is reached through text, voice, messages,
programs and events rather than through one screen. Mu is the open-source
runtime for that computer. Micro is the personal assistant people use to reach
it.

The model is not the product. It interprets intent and chooses actions. The
durable value is the system around it: identity, memory, conversations, runs,
permissions and a dependable set of services that act on the digital and
physical world.

In this model:

- a service is the unit of capability;
- a tool is an operation a service exposes to an agent;
- a flow is the durable record of one piece of work;
- the agent loop is the shell that composes tools;
- a saved composition is the equivalent of a script or executable; and
- web, CLI, MCP, mail, SMS, WhatsApp and voice are clients of the same runtime.

Unix made small programs useful by giving them a common process and stream
model. Plan 9 made remote resources feel local by giving them a common
interface. Mu should do the corresponding thing for services: make a capability
discoverable, callable, observable and composable in the same way wherever it
runs.

## Existing foundation

Mu already has more of this system than the chat interface makes visible:

- `internal/service` is the service and tool registry used by the web, API,
  CLI, MCP and agents.
- `agent.Flow` and `agent.FlowStep` record runs and tool use.
- `internal/thread` is the durable conversation record across clients.
- `agent.QueryWithOpts` is the shared agent loop.
- Native model calls have an idle timeout, retry budget, loop guard, step bound
  and whole-turn deadline.
- Web runs are created as `running` before execution, continue independently of
  the HTTP request and can be recovered after the browser disconnects.
- Tool annotations already distinguish operations that are safe to retry from
  writes requiring care or idempotency.

The work is therefore not to introduce another workflow abstraction. It is to
finish making flows the reliable lifecycle already implied by the code.

## Reliability contract

Every accepted piece of work should have a durable identity and one explicit
state:

- `running`: work is progressing;
- `waiting`: the system needs input, permission or payment;
- `done`: the requested outcome and its record are complete;
- `error`: work stopped, with a useful reason and the completed steps retained;
- `cancelled`: somebody deliberately stopped it.

A client connection is only a view onto a flow. Disconnecting must not cancel
eligible work, erase progress or create a second execution when the client
returns.

Each tool call should be recorded before it starts and updated when it ends,
including its stable call ID, tool name, safe display label, status, duration
and error. Reads may be retried within a bounded policy. Writes require an
idempotency key, a service guarantee, or explicit confirmation before a retry.
The whole flow has a deadline, but individual services own shorter deadlines
appropriate to their operations.

Progress and terminal state must be readable through the same record from every
client. SSE can make the live view pleasant, but correctness cannot depend on an
open SSE connection.

## Composition

The first pipe is structured data, not punctuation. Tool results should carry a
schema and provenance so the agent can pass one result to the next without
flattening it into prose:

```text
web_fetch -> text_extract -> notes_add -> tasks_create
```

The flow records that graph and the values that crossed each edge. A useful
flow can later be named, parameterised and run again as a new tool. This is the
service equivalent of turning a shell pipeline into a script.

Composition must not weaken the security of the components. The effective
permissions of a flow are the intersection of the caller, agent and service;
credentials are resolved only at the operation that needs them.

## Services and the outside world

Mu should not be limited to productivity-software connectors. Services can
terminate protocols and participate directly in the world: HTTP, SMTP, XMPP,
SSH, SFTP, telephony, SMS and messaging networks. That creates three broad
families:

1. digital primitives such as files, search, computation and storage;
2. live infrastructure such as communications, devices and places; and
3. third-party applications such as GitHub, Linear or Slack.

They should share one service contract even though their credentials differ.
A deployed service may use instance credentials to provide a capability to
everyone, account credentials to act as one person, or a delegated grant for a
narrow resource. Those modes must be declared per operation. A service must
never infer that an instance credential authorises it to impersonate a user.

Self-hosted installations can run a service per owner with local credentials.
Multi-tenant installations need account-scoped encrypted credentials or OAuth
grants, isolation at invocation, and revocation. This is a deployment choice,
not a reason for services to expose different tool shapes.

## Dynamic services

Adding a service should eventually require no edit to Mu's central source. A
service package needs a manifest containing:

- identity, version and health endpoint;
- operations with input and output schemas;
- retry, idempotency and side-effect annotations;
- required credential modes and scopes;
- invocation transports such as local process, HTTP or MCP; and
- optional human pages and protocol listeners.

Mu can then install or register the package, validate its declaration, expose
its operations through the common catalogue and make them immediately usable
by agents. A public directory may distribute declarations and packages, while
each instance decides what to install and which credentials to grant.

## Roadmap

### 1. Make flows dependable

- Persist flow creation before execution on every client, not only the web.
- Persist tool starts and completions while the run is active.
- Reconcile `running` flows after restart instead of leaving them ambiguous.
- Expose progress and terminal state consistently to web, CLI, MCP and inbound
  protocol clients.
- Classify errors and implement bounded, safety-aware retries.
- Add cancellation and `waiting` states.

### 2. Make flows inspectable

- Give conversations a clear link to their active and completed flows.
- Show current step, completed steps, elapsed time and actionable failures.
- Add structured operational metrics for model and tool latency, retries and
  failure rates.

### 3. Make services composable

- Define structured outputs and provenance.
- Represent dependencies between steps as a flow graph.
- Support parameters, conditions and approval boundaries.
- Promote a completed flow into a reusable named tool.

### 4. Make services installable

- Specify the service manifest and lifecycle.
- Support local-process, HTTP and MCP transports behind one registry contract.
- Add package verification, health checks, upgrades and removal.
- Build an optional directory without making a central directory a runtime
  dependency.

### 5. Broaden the ambient interface

- Treat voice, calls, messages and events as first-class clients.
- Preserve one identity, permission model, thread and flow record across them.
- Let proactive work enter the same durable lifecycle as an interactive ask.

## Market comparison to investigate

The relevant comparison is architectural, not a feature checklist. Research
should examine how OpenAI, Anthropic, Google, Microsoft, open-source assistants
and agent runtimes handle:

- first-party capabilities versus third-party connectors;
- dynamic tool discovery and installation;
- per-user OAuth grants versus operator credentials;
- durable execution, retries, resumability and background work;
- protocols beyond SaaS APIs;
- structured composition and reusable workflows; and
- self-hosted versus multi-tenant isolation.

The strategic question is where Mu should interoperate and where it should own
the abstraction. The likely boundary is: interoperate with external ecosystems
through services, but own the common service contract, flow lifecycle and
cross-protocol runtime.
