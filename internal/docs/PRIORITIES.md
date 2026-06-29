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

> Seeded from the North Star. The product-review pass files a tracking issue for
> each unlinked item on its first run (`gh issue create --label codex`) and links
> it here; until then the continuous-improvement loop falls back to picking the
> highest-value item itself.

1. **Fully onto go-micro — reference vertical: markets.** Per
   [ARCHITECTURE.md](ARCHITECTURE.md), prove the read-plane pattern on one
   surface: the markets service owns its background refresh and publishes a
   snapshot to the go-micro store; the card/page renders from a broker-fed local
   mirror (a memory read — no per-render RPC fan-out). Measure render latency
   before/after to prove parity. Sets the pattern for the rest. *(Needs issue.)*
2. **Unify events onto the go-micro broker.** Make `internal/event` a thin wrapper
   over the broker (preserve Subscribe/Publish ergonomics) so there is one bus,
   not a hand-rolled one beside the framework's. Behaviour identical; verify.
   *(Needs issue.)*
3. **Replicate the snapshot read-model** to news, video, social, blog — one
   service per increment, each verified for render-latency parity. *(Needs issue.)*
4. **[#747 Harden the core ask → answer loop end to end.](https://github.com/micro/mu/issues/747)** A guest and a signed-in user
   should get a correct, well-formatted, fast answer on web and on the chat
   clients. Add integration/smoke coverage of the ask → tool → answer path across
   the core services (weather, news, markets, mail, search) so regressions in the
   most important flow are caught.
5. **[#748 Every service degrades gracefully.](https://github.com/micro/mu/issues/748)** Audit each home card and each
   agent-callable service for the provider-down case — no dead cards, no silent
   failures, a clear "unavailable" instead. One service per increment.
6. **[#749 First-run experience.](https://github.com/micro/mu/issues/749)** A new visitor understands what Mu is and gets value
   from one prompt without an account — tighten the guest landing, suggestions,
   and the sign-up moment (when the free limit is hit) for clarity, not friction.
7. **[#750 Answer formatting quality.](https://github.com/micro/mu/issues/750)** Rendered answers (news, markets, weather) look
   right everywhere they appear — web (guest + signed-in), Discord, Telegram —
   with consistent spacing, headings, and links.

### Human-supervised (architectural — not for the auto-merge loop)

- **Agent-routed query plane.** Run specialist agents as services and route/
  delegate the query plane to the most relevant agent (go-micro `Agent.Chat` /
  `delegate` / chat routing), retiring the hand-rolled `agent/micro`. Gated on
  go-micro#3341 (streaming agent with tool events) for the streaming `/agent`
  path. Surface findings; do not auto-merge.
- **A2A cutover** once go-micro#3342 (multi-skill agent cards) lands.

_Seeded by Claude Code from the North Star; thereafter maintained by the
product-review pass._
