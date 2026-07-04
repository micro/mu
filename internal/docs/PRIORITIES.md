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

1. **Keep AI-news fallback answers date-specific** ([#1091](https://github.com/micro/mu/issues/1091)). The 2026-07-04 live guest review showed the core web ask → answer loop is fast and weather/markets now answer first, but `Find today's AI news` still falls back from an unavailable `news_search` provider to a generic AI-news directory page and undated snippets. This is the highest-value remaining Now-phase refinement because it preserves the first-run promise — ask for today's news, get clearly current, source-backed stories — without adding surface area or changing public contracts.

### Already shipped (do not re-queue)

- ✅ **MCP auth session hardening shipped.** PR #1089 closed #1079, so the previous top queue item is not re-used; the 2026-07-04 product review found no open codex issues or open codex PRs after that merge, and the remaining live core-loop friction is now AI-news fallback freshness/date specificity tracked in #1091.
- ✅ **Weather date anchoring shipped.** PR #1084 closed #1081: the 2026-07-04 live guest review for #1086 now shows `Weather in New York today` completing in about 3.5s, leading with `Today: Saturday, 4 July 2026 (2026-07-04)` while preserving dated forecast rows, Google Weather freshness metadata, and provider references.
- ✅ **Weather fallback answer-first polish shipped but live date anchoring remains wrong.** PR #1077 closed #1074 by improving weather fallback answer leads, so the old answer-first queue item is not re-used; the 2026-07-04 live review for #1080 still found `Weather in New York today` labelling the previous provider row (Friday, 3 July 2026) as today despite a Saturday, 4 July 2026 request date, now tracked as #1081.
- ✅ **Fallback answer-first polish shipped but live regression remains after PR #1072.** PR #1072 closed #1068 by further polishing fallback answer leads in tests; the 2026-07-04 live review for #1073 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1074.
- ✅ **Fallback answer-first polish shipped but live regression remains after PR #1066.** PR #1066 closed #1063 by keeping fallback answers answer-first in tests; the 2026-07-03 live review for #1067 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1068.
- ✅ **Fallback answer-first polish shipped but live regression remains after PR #1061.** PR #1061 closed #1058 by tightening answer-first fallback output in tests; the 2026-07-03 live review for #1062 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1063.
- ✅ **Fallback answer-first polish shipped but live regression remains after PR #1056.** PR #1056 closed #1053 by adding more fallback ordering tests; the 2026-07-03 live review for #1057 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1058.
- ✅ **Fallback answer-lead polish shipped but live regression remains after PR #1051.** PR #1051 closed #1048 and #1050 by tightening weather and news fallback leads in tests; the 2026-07-03 live review for #1052 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1053.
- ✅ **Fallback result-first polish shipped but live regression remains after PR #1046.** PR #1046 closed #1043 by further tightening result-first fallback presentation in tests; the 2026-07-03 live review for #1047 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1048.
- ✅ **Fallback result-first polish shipped but live regression remains after PR #1041.** PR #1041 closed #1038 by further tightening result-first fallback presentation in tests; the 2026-07-03 live review for #1042 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1043.
- ✅ **Fallback result-first polish shipped but live regression remains after PR #1034.** PR #1034 closed #1031 by tightening answer-first fallback presentation in tests; the 2026-07-03 live review for #1037 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1038.
- ✅ **Fallback result-first polish shipped but live regression remains.** PR #1029 closed #1026 by keeping fallback synthesis result-first in tests; the 2026-07-03 live review for #1030 still reproduced metadata-first weather output and search-result-style AI-news fallback output on the deployed product, now tracked as #1031.
- ✅ **Fallback answer-first polish shipped but live regression remains.** PR #1024 closed #1021 / #1023 by adding answer-first fallback guards; the 2026-07-03 live review for #1025 still reproduced metadata-first weather/markets answers and search-result-style AI-news fallback output on the deployed product, now tracked as #1026.
- ✅ **Fallback answer heading polish shipped.** PR #1019 closed #1012 by removing implementation-style tool headings, so the previous queue item is not re-used; the 2026-07-03 live review still found answer-first ordering gaps in weather, markets, and AI-news fallback responses, now tracked as #1021.
- ✅ **News web fallback answer polish shipped.** PR #1010 closed #998, so the previous active queue item is not re-used; the 2026-07-02 live review now shows AI-news fallback completing quickly with web sources, but cross-service answers still read like tool reports, now tracked as #1012.
- ✅ **Usable weather forecasts no longer reported unavailable.** PR #1006 closed #1003: live guest `Weather in New York today` now uses Google Weather current conditions, dated forecast rows, and freshness metadata instead of ending at `Unavailable: weather_forecast`; the later news fallback polish shipped in #1010; the remaining active core-loop formatting gap is now the cross-service answer-first presentation tracked in #1012.
- ✅ **Weather unavailable fallback fix merged but live gap persists.** PR #1001 closed #997, so the old queue item is not re-used; the 2026-07-02 live guest review still reproduced the unavailable-only final answer despite usable Google Weather references, now tracked as #1003.
- ✅ **Live weather-backed provider path restored.** PR #995 closed #992: weather references now include usable Google Weather current conditions, dated forecast rows, and freshness context during the 2026-07-02 guest review; the remaining defect is answer synthesis incorrectly reporting the successful tool as unavailable, now tracked in #997.
- ✅ **AI-news fallback answer synthesis latency reduced.** Guest `Find today's AI news` now emits early progress, discloses the unavailable `news_search` provider, falls back to web results, and completes with a readable source-linked answer in about 1s, closing #987 / PR #990; the follow-up weather synthesis defect is now tracked in #997.
- ✅ **Market-mover agent answer latency reduced.** Guest `What is moving in markets?` now emits immediate market tool events and starts the final answer in under 1s in the 2026-07-02 live review, closing #980 / PR #983; AI-news fallback synthesis latency was then tracked in #987 and later shipped in PR #990.
- ✅ **Weather-backed agent answer completion latency reduced.** Guest `Weather in New York today` now emits early progress and completes in about 3s when the weather provider reports unavailable, closing #974 / PR #978; market-mover answer latency was then tracked in #980 and later shipped in PR #983.
- ✅ **Ground news fallback answers in source snippets.** Topic-specific unavailable-news web fallbacks now require snippet-supported stories or disclose limited evidence, closing #969 / PR #972; weather-backed answer completion latency was then tracked in #974 and later shipped in PR #978.
- ✅ **Unavailable news fallbacks kept topic-specific.** Guest `Find today's AI news` now keeps the web fallback focused on AI instead of broad technology/sports filler, closing #964 / PR #967; source-evidence grounding later shipped in #969 / PR #972.
- ✅ **Live news fallback restored when news_search is unavailable.** Guest `Find today's AI news` now emits early progress, discloses the unavailable news provider, falls back to web results, and streams a readable source-linked answer, closing #959 / PR #962; topic precision later shipped in #964 / PR #967, and source-evidence grounding later shipped in #969 / PR #972.
- ✅ **Guest agent answer latency reduced.** Market/weather/news prompts now emit early progress and PR #957 reduced first-token latency for the market-mover path, closing #951 / PR #957; unavailable-news topic precision later shipped in #964 / PR #967, and source-evidence grounding later shipped in #969 / PR #972.
- ✅ **Guest agent planning latency reduced.** Tool-backed guest prompts now emit early progress and avoid some duplicated planning work, closing #949 / PR #953; answer-token latency later improved in #951 / PR #957; current weather completion latency is tracked in #974.
- ✅ **Duplicate market tool calls avoided.** Guest market answers now use a single market-price pass before synthesis instead of repeated identical calls, closing #944 / PR #947.
- ✅ **Blog currency amounts protected from MathJax.** Blog/card rendering now protects dollar amounts such as “$1 billion” from MathJax delimiters, closing #939 / PR #942.
- ✅ **Agent market prices protected from MathJax.** Guest market answers now keep dollar prices readable in streamed and final answers, closing #934 / PR #937.
- ✅ **Filtered unrelated market-mover headlines.** Market-mover answers now keep movers/prices first and avoid unrelated business/tech headlines, closing #927 / PR #930 / PR #932.
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
