# Priorities

The ranked product work queue for Mu's autonomous loop. The **product-review**
pass (the *product agent*) owns this file: each run it turns the North Star
([THESIS.md](THESIS.md)) plus a hands-on look at the live product into a single
ordered list — highest-value first — and links each item to a tracking issue. The
hourly **continuous-improvement** pass works the **top item whose issue is still
open**. So the product agent decides *what*, and the increment loop *builds* it.

**Reading / editing.** An item is done when its linked issue closes (the increment
that builds it adds `Closes #<issue>`). Roadmap phase (Now → Next → Later in
THESIS.md) is the primary ordering — and this phase is **Now: seamlessness**, so
refinements that make the existing product work better rank above new surface
area. The human can reorder this list — or the issues — at any time to redirect
the loop; direction always wins.

**Off-limits to the loop** (the product agent proposes these as notes, never as
queue items the loop can auto-merge): brand/positioning copy, pricing, breaking
public-contract changes (MCP/A2A/REST/webhooks/env vars), architectural rewrites,
and publishing marketing content. Those go to the human.

## Work queue (ranked)

1. **[#787 Fix malformed bold text in streamed agent answer lists.](https://github.com/micro/mu/issues/787)** The highest-value live regression is in the core guest ask → answer loop: a brief weather answer streams correctly, then final HTML can show stray markdown delimiters (for example `*London today:** ...`). Fix the markdown normalization/conversion path and cover it with CI-verifiable tests so web and chat clients show clean answers.

### Already shipped (do not re-queue)

- ✅ **Read plane on go-micro for every display card.** Shared `internal/snapshot`
  helper (store + broker, broker-fed mirror); markets, news, video, social, blog
  all serve from it with a fallback. No per-render RPC fan-out.
- ✅ **One event bus.** `internal/event` is now a thin wrapper over the go-micro
  broker (no hand-rolled pub/sub beside the framework's).
- ✅ **Durable shared store.** `service.Store()` is file-backed (`~/.mu/store`),
  so snapshots persist across restart (warm cards on boot); memory fallback.

### Human-supervised (architectural — not for the auto-merge loop)

- **Durable event bus.** Back `internal/event` with a file-backed events stream
  (`events.NewStream(events.WithStore(fileStore))`, shipped in go-micro v6.3.9),
  replay a bounded recent window on startup, prune by age. Deliberate: the events
  stream's delivery differs from the broker and the replay/retention policy is a
  decision (see ARCHITECTURE.md "Durability"). Surface findings; do not auto-merge.
- **Agent-routed query plane.** Run specialist agents as services and route/
  delegate the query plane to the most relevant agent (go-micro `Agent.Chat` /
  `delegate` / chat routing), retiring the hand-rolled `agent/micro`. Gated on
  go-micro#3341 (streaming agent with tool events) for the streaming `/agent`
  path. Surface findings; do not auto-merge.
- **A2A cutover** once go-micro#3342 (multi-skill agent cards) lands.

_Seeded by Claude Code from the North Star; thereafter maintained by the
product-review pass._
