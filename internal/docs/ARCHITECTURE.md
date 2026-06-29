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

## Durability (no external infrastructure)

The shared store (`service.Store()`) is the **file store** — bbolt under
`~/.mu/store` — not memory. So data written through it survives a restart with no
infrastructure to run. Today that means the card **snapshots persist**: on boot
`snapshot.New` primes the mirror from the store, so cards are warm immediately
instead of blank until the first refresh. If the file store can't open, snapshots
fall back to locally-generated HTML (no regression).

**Events** are different: the memory **broker** is ephemeral (anything in flight
at restart is lost). go-micro's **`events`** package is the durable answer —
store-backed `Publish`/`Consume` with replay-by-offset — but its `NewStream`
hardcoded an in-memory store. We fixed that upstream (**go-micro v6.3.9**:
`events.WithStore`), so an events stream can now be backed by the file store and
replay across restarts. Adopting it for `internal/event` is the **next step**, and
it is a deliberate one because:
- The events stream's delivery differs from the broker (async, per-event
  goroutines, unbuffered) — the current channel-based API and its timing-
  sensitive tests need adapting.
- Without per-consumer acks (our consumers are fire-and-forget), "process only
  the *unprocessed* events" needs either a retention/replay-window policy
  (idempotent re-delivery) or an ack-delete queue. A decision, not a default.
- In practice the exposure is small: mu's events are idempotent regeneration
  triggers that the hourly background loops re-derive anyway, so a lost in-flight
  event self-heals on the next pass.

Plan: back `internal/event` with `events.NewStream(events.WithStore(fileStore))`,
replay a bounded recent window on startup, prune by age. Tracked, not auto-merged.

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
2. ✅ **Replicate the snapshot read-model** to every display card. Done — the
   pattern was extracted into a shared helper, **`internal/snapshot`** (one tested
   `Snapshot` type: `New(name)` subscribes + primes from the store, `Publish`
   writes store + broker, `Get` reads the mirror). All five cards use it:
   **markets, news, video, social, blog** — each serves `Get()` with a fallback
   to its locally-cached HTML, each with round-trip + fallback tests. Publish is
   hooked at every cache-rebuild site (video's two finalize points; social's
   `updateCacheLocked`; blog's `updateCacheUnlocked`).
3. ✅ **Unify events onto the broker.** Done — `internal/event` is now a thin
   wrapper over the go-micro broker: `Publish` JSON-encodes `Event.Data` onto the
   broker topic; `Subscribe` keeps the same buffered-channel API, fed by a
   broker subscription (guarded against send-on-closed for `Close`). One bus, no
   hand-rolled pub/sub beside the framework's. Verified the JSON round-trip is
   lossless: all 17 consumer assertions on `Event.Data` are `.(string)`. The
   existing event test-suite and the consumers pass (incl. `-race`).
4. ◑ **Query plane on the native agent.** In progress.
   - ✅ **Streaming `/agent` cut over.** go-micro#3341 shipped as `agent.StreamAsk`
     (in v6.3.9): the agent runs its tool loop, emits `tool_start`/`tool_end`
     events, and streams the final answer in chunks. mu's streaming handler now
     drives it (`agent/native.go` `streamNative` + `agent/agent.go`
     `streamNativeSSE`): tool events → `tool_start`/`tool_done`, answer →
     `stream_start`/`stream_token` (now genuinely incremental, where it used to
     arrive in one chunk). The hand-rolled plan/execute/synthesize pipeline
     remains only as a **fallback** — used when no native provider is configured
     or the agent fails before any output. `AGENT_NATIVE=off` forces the planner.
     Trade-off: native mode renders the answer inline but not the planner's
     per-tool "reference" cards (the answer itself is the content).
   - TODO: retire the hand-rolled planner and `agent/micro` once the native path
     has proven out on real traffic; route to **multiple specialist agents** via
     `delegate`/chat routing rather than one assistant agent.
5. **A2A cutover** once go-micro#3342 lands.

**Autonomy note:** steps 1–3 are scoped and safe enough for the loop. Steps 4–5
are architectural/contract-changing — human-supervised, surfaced as notes, never
auto-merged (see THESIS autonomy boundaries).
