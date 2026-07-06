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

1. **Restore stale-only disclosure in live AI-news answers** ([#1197](https://github.com/micro/mu/issues/1197)). PR #1193 closed #1187 by adding stale-only answer caveats in code, but the 2026-07-06 live guest review for #1196 still shows the deployed core ask → answer loop regressed: `Find today's AI news` reaches `news_search`, completes quickly, and keeps clean source URLs, yet the first rendered answer is `News results for "AI news"` followed by March-May 2026 TechCrunch/CoinDesk stories with no up-front no-current-results disclosure. This remains the highest-value Now-phase PMF gap because it is the main first-run promise — ask Mu something current and get a truthful, well-formatted answer. Preserve the fast path and links; make the live/synthesis path lead stale-only result sets with an unmistakable caveat and label older stories as background.
2. **Resolve DNS before web_fetch connects to prevent SSRF to internal IPs** ([#1189](https://github.com/micro/mu/issues/1189)). Agent-callable web fetch/search must not let model-chosen or tool-content URLs reach loopback, link-local, RFC1918, or metadata services via DNS resolution or rebinding. This is Now-phase reliability/trust for an existing callable service, not new surface area: keep public HTTP/HTTPS fetches and redirect handling working while blocking DNS-resolved private targets in validation and dialing, with regression coverage.
3. **Add a daily cap or confirmation for wallet credit transfers** ([#1188](https://github.com/micro/mu/issues/1188)). Wallet transfer source binding is already session-scoped, but repeated model-initiated transfers are only bounded per call. Add a per-account daily budget and/or explicit human confirmation for large or repeated transfers, keeping guests unable to transfer and preserving `sess.Account` as the only source account. This ranks after the web-fetch SSRF guard because it is signed-in/payment-surface trust rather than the first-run guest ask → answer path, but it is still a high-value refinement of existing behavior.

### Already shipped (do not re-queue)

- ✅ **Current-news candidate pool widening shipped but stale-only disclosure remains.** PR #1184 closed #1172 and #1183 by widening freshness-oriented `news_search` candidate selection before recency sorting; the 2026-07-06 live guest review for #1186 still saw `Find today's AI news` complete quickly but lead with March–May 2026 stories for a Monday, 6 July 2026 today prompt without an up-front stale-only disclosure, now tracked in #1187.

- ✅ **News freshness caveat surfacing shipped but stale-first ranking remains.** PR #1180 closed #1177 by making agent answers surface stale-result freshness caveats when the provider marks older fallback evidence; the 2026-07-06 live guest review for #1181 still saw `Find today's AI news` complete in under 1s while leading with March–May 2026 stories for a Monday, 6 July 2026 prompt, now tracked in #1172.

- ✅ **News freshness caveat metadata shipped but agent answers still need to surface it.** PR #1175 closed #1172 by adding freshness caveats to `news_search`; the 2026-07-05 product review for #1176 still saw live guest `Find today's AI news` complete in under 1s while rendering March–May 2026 stories without the no-same-day-results disclosure, now tracked in #1177.

- ✅ **AI-news metadata/source rendering polish shipped but today prompts still need freshness ranking.** PR #1170 closed #1167 by cleaning the category/date/source presentation after PR #1165; the 2026-07-05 product review for #1171 still saw `Find today's AI news` complete in about 1s while leading with March–May 2026 stories for a same-day prompt, now tracked in #1172.

- ✅ **news_search-backed AI-news answer speed and raw-reference cleanup shipped.** PR #1165 closed #1162 by making the restored `news_search` path fast and reducing the previous raw JSON-like reference exposure; the 2026-07-05 product review for #1166 now sees `Find today's AI news` complete in under 1s, but the answer can lead with stale May 2026 items and malformed category/date metadata links, now tracked in #1167.

- ✅ **Live news_search provider path restored but answer quality remains rough.** PR #1157 closed #1154 and PR #1160 restored direct `news_search` tool calls for the live agent path; the 2026-07-05 product review for #1161 now sees `Find today's AI news` reach `news_search`, but the answer takes about 21s and exposes a large raw JSON-like References payload, now tracked in #1162.

- ✅ **Limited-evidence AI-news category fallback framing shipped but live news_search remains unavailable.** PR #1152 closed #1149 by keeping category/root/topic evidence in explicit limited-evidence framing and removing the prior promoted category lead; the 2026-07-05 live review for #1153 still saw `Find today's AI news` report `news_search` unavailable and rely on directory/category web fallback evidence, now tracked in #1154.

- ✅ **AI-news directory fallback markup polish shipped but category snippets can still be over-promoted.** PR #1147 closed #1144 by cleaning directory-only fallback bullets after PR #1141; the 2026-07-05 live review for #1148 still saw `Find today's AI news` lead with the limited-evidence `tech.yahoo.com/ai/` category URL and duplicate the Meta AI-glasses snippet, now tracked in #1149.

- ✅ **Article-source preference for AI-news fallback shipped but clean directory-only rendering remains rough.** PR #1141 closed #1137 by avoiding category-page leads when article-like sources are available; the 2026-07-05 live review for #1143 still saw `Find today's AI news` rely on directory/topic sources when `news_search` was unavailable and render one duplicated nested `<strong>` story bullet, now tracked in #1144.

- ✅ **Article-level AI-news fallback preference shipped but live category-page leads remain.** PR #1133 closed #1130, so that completed issue is not re-queued; the 2026-07-05 live review for #1136 still saw `Find today's AI news` lead with category/root URLs such as Yahoo AI and Reuters artificial-intelligence as limited-evidence latest items when `news_search` was unavailable, now tracked in #1137.

- ✅ **Readable AI-news limited-evidence fallback URLs shipped but category-page evidence remains rough.** PR #1128 closed #1124 by keeping limited-evidence fallback links usable instead of rendering truncated `https://www.…` anchors; the 2026-07-05 live review for #1129 still saw AI-news unavailable fallback relying on category/root pages as limited evidence, now tracked in #1130.

- ✅ **Limited-evidence AI-news fallback labeling shipped but URL readability remains rough.** PR #1122 closed #1119 by labeling root/topic/category fallback sources as limited evidence instead of presenting them as concrete current stories; the 2026-07-04 live review for #1123 still saw a truncated `https://www.…` URL in the AI-news fallback answer, now tracked in #1124.

- ✅ **Article-level AI-news fallback preference shipped but live source labels remain generic.** PR #1117 closed #1114 and #1116 by preferring article-level web results in fallback selection; the 2026-07-04 live review for #1118 still saw `Find today's AI news` lead with a root/domain result and generic category-page labels, now tracked in #1119.

- ✅ **Snippet-backed AI-news story-title promotion shipped but article-level source selection remains rough.** PR #1112 closed #1109 by promoting snippet-supported story descriptions over generic category titles in fallback leads; the 2026-07-04 live review for #1113 still saw `Find today's AI news` cite a root/domain result and generic category-page labels as current items, now tracked in #1114.

- ✅ **Generic AI-news directory filtering shipped but category titles can still be promoted.** PR #1107 closed #1104 and #1106 by filtering generic AI-news directory/category pages when story-like results are available; the 2026-07-04 live review for #1108 still saw `Find today's AI news` lead with a Yahoo Tech category-page title even though the snippet contained the actual Meta AI-glasses story, now tracked in #1109.

- ✅ **Dated AI-news fallback story prioritization shipped but generic directory leakage remains live.** PR #1102 closed #1099 by prioritizing dated story sources in AI-news fallbacks, so that completed issue is not re-queued; the 2026-07-04 live guest review for #1103 still saw generic AI category/directory pages in the lead fallback answer when `news_search` was unavailable, now tracked in #1104.

- ✅ **Lookup-only AI-news progress narration caught.** PR #1097 closed #1096 by replacing "I'll look up..." lookup-only fallback text with source-backed web results, so that completed bug is not re-queued; the later #1099 story-prioritization issue shipped in PR #1102; remaining live generic-directory leakage is now tracked in #1104.
- ✅ **AI-news fallback date-stamping shipped.** PR #1094 closed #1091 and #1093 by date-stamping synthesized AI-news web fallback leads, so the previous top queue item is not re-used; the 2026-07-04 live review now sees a Saturday, 4 July 2026 (2026-07-04) lead, with dated story prioritization later tracked in #1099 and shipped in PR #1102; remaining live generic-directory leakage is now tracked in #1104.

- ✅ **MCP auth session hardening shipped.** PR #1089 closed #1079, so the previous top queue item is not re-used; the 2026-07-04 product review found no open codex issues or open codex PRs after that merge, and later AI-news fallback date specificity shipped in #1094, story prioritization shipped in #1102, and remaining generic-directory leakage is tracked in #1104.
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
