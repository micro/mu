# Architecture — fully onto Go Micro (target model)

Companion to [THESIS.md](THESIS.md). The North Star says *why* (an agent for
everyday, on go-micro). This says *how the system is shaped* as we finish moving
Mu entirely onto go-micro — without degrading performance and without a
half-migrated blend.

## Where go-micro is going: an agent harness

Per go-micro's README, it is an **agent harness** and service framework. The
model that matters for Mu:

- **Tools are services** — every endpoint is an AI-callable tool (RPC executes it).
- **Agents are services** — an agent registers in the registry, exposes
  `Agent.Chat`, is discoverable and load-balanced like any service. `micro chat`
  (and `delegate`) **route a query to the most relevant agent**.
- **Workflows are durable flows** — known paths as flows; unknown paths dispatched
  to agents.
- **Broker / store / registry** are the shared, pluggable runtime primitives.

So the query model is: *you issue a request and the agent that owns the relevant
services picks it up* — the same way you used to call a microservice, but the
routing target is an agent. Mu's hand-rolled `agent/micro` (registry + router +
executor + orchestrator) is the **internal reinvention** of exactly this.

## Two planes (the key to not degrading performance)

The mistake to avoid is turning a page render into N live RPCs. Mu's data is
already background-refreshed into caches (e.g. `markets.MarketsHTML()` returns a
pre-rendered string a `refreshMarkets()` goroutine maintains; home cards are
cached with a TTL). We keep that. Two distinct planes:

### Read plane (renders) — serve pre-cached snapshots, no fan-out
- Each domain **service** owns its background refresh and produces a **snapshot**.
- The service publishes the snapshot to the go-micro **store** (durable) and/or
  announces updates on the **broker**.
- The web/cards layer renders from a **local mirror** of the latest snapshot
  (updated by a broker subscription) — a memory read, same cost as today. No
  per-render RPC fan-out.

### Query plane (actions) — live, agent-routed
- Real queries — news search, video search, weather-for-a-location, "ask Mu" —
  are dispatched live. Today they go through `service.Call`; the direction is to
  route them to the **most relevant agent** (`Agent.Chat` / delegate), so the
  agent that owns those services handles the request.

Renders read snapshots; only genuine queries hit the live plane. That is how "fully
on go-micro" and "fast, cheap pages" hold at the same time.

## Internal vs framework — the judgment calls

The rule (per the user): wherever go-micro has a robust version of something we
hand-rolled, **use it** (and improve go-micro where it falls short); keep only the
things that are genuinely Mu-specific.

| Concern | Mu today | Go Micro | Call |
|---|---|---|---|
| Pub/sub events | `internal/event` (in-memory typed channels) | **broker** (memory/http/nats/rabbitmq) — robust, pluggable | **Adopt the broker.** `internal/event` is reinventing it. Wrap to keep ergonomics if needed. |
| Snapshot/cache distribution | package-global caches + `data` JSON files | **store** (memory/file/postgres/nats-kv) | **Adopt the store** for snapshots in the read plane. |
| Full-text search / indexing | `data` package (SQLite FTS) | store is KV, not search | **Keep internal.** Beyond what the store provides. |
| Multi-agent routing | `agent/micro` (router/executor/orchestrator) | agents-as-services + registry + `Agent.Chat` + `delegate` + chat routing | **Migrate to go-micro routing**, retire `agent/micro`. The big one. |
| Streaming agent w/ tool events | hand-rolled `/agent` planner | not yet (filed go-micro#3341) | **Improve the framework** (dogfood), then cut over. |
| Multi-skill A2A | mu's `/a2a` | task lifecycle shipped; multi-skill cards filed (go-micro#3342) | **Improve the framework**, then cut over. |

## Migration order (incremental, no regression; verify each)

1. ✅ **Reference vertical: markets.** Done — `markets/snapshot.go`: the service
   publishes its rendered card to the go-micro store + broker; `MarketsHTML()`
   serves a broker-fed mirror with a fallback to locally-generated HTML. Render
   stays a memory read.
2. ◑ **Replicate the snapshot read-model** to the other display cards — one
   service at a time, each verified.
   - ✅ **news** — `news/snapshot.go`; `Headlines()` serves the mirror.
   - TODO **video, social, blog.** Caveat learned: markets/news each have a
     single cache-update site, so one `publishSnapshot` covers them. video sets
     `latestHtml` in several places, social updates via `updateCacheLocked`, blog
     via `RefreshCache` (+ others) — publish from every update site (or
     centralize the cache write first) or the snapshot goes stale. Add the
     round-trip + fallback tests each time.
3. **Unify events onto the broker.** Make `internal/event` a thin wrapper over the
   go-micro broker (preserve Subscribe/Publish ergonomics) so there is one bus.
   **Caveat:** the broker carries `[]byte`, so wrapping JSON-marshals each
   `Event.Data` map — numbers come back as `float64`. Audit every consumer's type
   assertions before cutting over (today they appear string-only).
4. **Agent-routed queries.** Run specialist agents as services; route/delegate the
   query plane to the most relevant agent; retire `agent/micro`. *(Architectural —
   human-supervised, not auto-merged. Gated on go-micro#3341 for the streaming
   path.)*
5. **A2A cutover** once go-micro#3342 lands.

**Autonomy note:** steps 1–3 are scoped and safe enough for the loop. Steps 4–5
are architectural/contract-changing — human-supervised, surfaced as notes, never
auto-merged (see THESIS autonomy boundaries).
