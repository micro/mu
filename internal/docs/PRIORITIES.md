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

1. **Filter unrelated headlines from market-mover answers** ([#927](https://github.com/micro/mu/issues/927)). Guest market prompts are now readable and stream a structured answer, but live review still found the answer mixing unrelated business/tech headlines into “What is moving in markets today?”. Preserve `/agent` and service contracts while keeping movers/prices first and only including news that directly explains a listed mover, index/sector move, or named asset/company.

### Already shipped (do not re-queue)

- ✅ **Concise market-mover agent answers.** Guest market prompts now produce readable, source-linked answers rather than raw market payloads, closing #920.
- ✅ **Mixed-source agent answers synthesized instead of raw tool payloads.** Mixed blog/social/search/news prompts now guard against raw JSON-like provider payloads and keep unavailable-provider disclosures human-readable, closing #915 / PR #918.
- ✅ **Weather freshness disclosed in agent summaries.** Weather-backed agent summaries now include source/freshness context and stale/unavailable handling, closing #910 / PR #913.
- ✅ **Chat topic summary freshness disclosed.** Guest `/chat` topic summaries now expose generated-at/source/status metadata, closing #904 / PR #907.
- ✅ **Market data freshness disclosed.** Guest `/markets` now shows data sources and last refresh cadence, and markets-backed agent answers stream readable market tables, closing #899 / PR #902.
- ✅ **Missing weather readings treated as unavailable.** Weather-backed agent answers now avoid presenting absent optional observations as real zeroes, closing #894 / PR #897.
- ✅ **Guest navigation matches account state.** Signed-out pages now hide signed-in Account/Logout actions while preserving signed-in navigation, closing #889 / PR #892.
- ✅ **Readable markets agent answers.** Guest `What is moving in markets?` prompts now stream synthesized movers, prices, and percentage moves instead of raw JSON/tool payloads or MathJax-mangled dollar prices, closing #880 / PR #887.
- ✅ **Tightened agent news topic relevance.** Latest technology-news prompts now prefer clearly topic-matching headlines or disclose adjacent-only coverage, closing #875 / PR #878.
- ✅ **Guest chat authentication clarified.** Guest `/chat` now shows an inline account-required explanation with login/signup and public-agent paths instead of submitting to a raw `401 Authentication required` response, closing #870 / PR #873.
- ✅ **Guest search query rendering fixed.** Guest `/search` recent searches now preserve spaces while still escaping HTML-sensitive characters, closing #861 / PR #864.
- ✅ **Guest weather access clarified.** The guest `/weather` page now gives an actionable agent-backed weather path and explains which saved-location/pollen/refresh features require login, closing #856 / PR #859.
- ✅ **Deduplicated mixed-source news surfaces.** Mixed-source news cleanup shipped in #851 / PR #854, so the queue now moves to the next highest-value first-run/service-access refinement.
- ✅ **Article links preserved in live news agent answers.** Fresh guest technology-news prompts now complete quickly with readable, grounded headlines and article URLs in the final answer, closing #846 / PR #849.
- ✅ **News-backed answers restored when feeds are partially unavailable.** Live guest technology-news prompts now complete with readable headlines from usable provider context instead of only an unavailable-state response, closing #841 / PR #844.
- ✅ **Fresh guest prompt isolation.** Independent guest `/agent` requests no longer leak prior topic context into unrelated prompts, closing #836 / PR #839.
- ✅ **Mixed-provider news fallback made user-readable.** News-backed agent answers now synthesize readable output from usable provider context and disclose unavailable providers without raw internal payloads, closing #831 / PR #834.
- ✅ **Impossible weather dates fixed.** Weather-backed synthesis now anchors forecast context to real provider dates and avoids fabricated invalid calendar dates, closing #824 / PR #827 / PR #829.
- ✅ **Source-linked news answers.** News-backed synthesis now includes readable source names and URLs instead of opaque internal ids, closing #819 / PR #822.
- ✅ **Grounded web-search answers.** Web-search synthesis now preserves query intent, includes source URLs in the model-ready context, and asks for refinement when weak results do not support an answer, closing #812 / #814 / PR #815.
- ✅ **Grounded topic news search.** Latest/topic news prompts now use grounded feed-backed results or an explicit unavailable state instead of plausible placeholder headlines after provider errors, closing #807 / PR #810.
- ✅ **Request-date anchoring for live agent answers.** Weather, news, markets, and other `today/latest/current` answers now anchor to the request date or disclose provider staleness, closing the stale-date friction found on 2026-06-30 (#802 / PR #805).
- ✅ **Real news answers instead of progress-only search fallbacks.** Guest news prompts now synthesize actual news/search results or show an explicit unavailable state instead of ending on progress narration (#797 / PR #800).
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
