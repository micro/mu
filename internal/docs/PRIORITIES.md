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

1. **Keep AI-news leads focused on concrete AI developments** ([#1441](https://github.com/micro/mu/issues/1441)). The 2026-07-11 product review for #1440 found #1436 closed by PR #1439, with no open codex-labeled PRs or issues before filing this follow-up. Canon still puts Now-phase seamlessness first: the core guest ask → answer loop being fast, correct, well-formatted, and honest outranks new surface area. Live first-run remains clear for guests: the landing page states `An agent for everyday`, `/agent` is directly usable without an account, `/chat` explains that saved chat requires sign-in while pointing guests to the public agent, and `/weather` explains guest weather access through the agent. Sampled service pages (`/`, `/agent`, `/chat`, `/weather`, `/markets`, `/news`, `/search`, `/video`, `/blog`) all returned 200s. Guest loop checks were fast and mostly readable: `Weather in New York today` returned a dated Saturday, 11 July 2026 forecast with `weather_forecast` in about 1.2s, and `What is moving in markets?` returned priced crypto movers with `markets` in about 0.7s. The previous top friction improved: `Find today's AI news` completed through `news_search` in about 0.6s and no longer exposed internal freshness-caveat guidance or mislabeled same-day AI items as background. The remaining highest-value rough edge is narrower topic relevance in the same core prompt: the answer led with a broad UK crypto-regulatory story before more concrete AI stories such as coordinated AI agents finding an Ethereum validator crash, an AI deepfake test, AI-chip financing, AI-agent dispute resolution, and AI-answer governance. Refinement ranks above additions, so the next increment should keep same-day AI-specific stories ahead of merely adjacent finance/crypto items, preserve the fast provider-backed path, and keep graceful fallback behavior.




### Already shipped (do not re-queue)

- ✅ **AI-news freshness-summary polish shipped.** PR #1439 closed #1436 by rendering mixed fresh/stale AI-news caveats as user-facing prose instead of exposing internal instructions or overusing `Background`. The 2026-07-11 live review for #1440 confirmed that freshness presentation is fixed; the remaining top friction is that broad adjacent crypto/regulatory items can still outrank concrete AI developments, now tracked in #1441.

- ✅ **Weak AI-news job-market and compiler-post filtering shipped.** PR #1434 closed #1431 by filtering weak AI-adjacent job-market and compiler/JIT posts from `Find today's AI news`, so the previous queue item is no longer active. The 2026-07-11 live review for #1435 confirmed the answer now stays AI-substantive and fast, but mixed fresh/stale rendering still exposes internal caveat guidance and overuses `Background`, now tracked in #1436.

- ✅ **Same-day AI-news lead substance filtering shipped.** Issue #1426 closed on 2026-07-11; the 2026-07-11 review for #1430 confirmed that weak novelty/demo-page promotion is no longer the observed top gap. The remaining live AI-news friction is broader topic relevance across synthesized leads and attached cards, now tracked in #1431.

- ✅ **Broad AI-chip finance hidden from AI-news cards.** PR #1424 closed #1421 by keeping stale broad AI-chip finance out of the attached AI-news result cards after PR #1419 had cleaned up the synthesized background bullets. The 2026-07-11 live review for #1425 confirmed that finance-card leakage is no longer the top open codex item; the remaining live AI-news friction is now weak same-day AI-adjacent novelty/demo material being promoted ahead of more substantive older context, tracked in #1426.

- ✅ **Stale AI-chip finance background filtering shipped but attached AI-news cards still show chip IPO finance.** PR #1419 closed #1414 by keeping stale broad AI-chip finance out of the synthesized AI-news background bullets after PR #1412. The 2026-07-11 live review for #1420 confirmed the final answer now leads with the no-current-results caveat and AI-substantive background items, but the attached News card still begins with SK Hynix IPO / Nasdaq finance rows before those same AI-substantive entries, now tracked in #1421.

- ✅ **Broad AI-chip finance demotion shipped but stale AI-news background still leads with chip IPO finance.** PR #1412 closed #1409 by adding coverage to demote broad AI-chip finance after PR #1407. The 2026-07-11 live review for #1413 confirmed the guest path remains fast and correctly caveats that no same-day 2026-07-11 news_search results are available, but the background set still starts with SK Hynix Nasdaq-debut / IPO finance stories before clearer AI-agent, forecasting, and model-governance items, now tracked in #1414.

- ✅ **Generic software/off-topic AI-news leftovers filtered, but broad AI-chip finance background still leads.** PR #1407 closed #1404 by filtering stale/off-topic leftovers such as generic software posts from AI-news answers. The 2026-07-11 live review for #1408 confirmed `Find today's AI news` now leads with the no-current-results caveat and no longer showed the previous `FreeCAD in the Browser` leftover, but it still starts the background set with broad SK Hynix IPO / stock-mover finance before more substantive AI-agent and AI-model governance stories, now tracked in #1409.

- ✅ **Generic finance-policy filtering shipped but weak adjacent AI-news items remain.** PR #1394 closed #1391 by keeping generic UAE export-control / Binance-stablecoin finance-policy out of AI-news leads. The 2026-07-10 live review for #1395 confirmed `Find today's AI news` is still fast and now leads with a substantive AI-model economics story, but it still includes a generic YC hiring post and broad SK Hynix Nasdaq-debut market story before clearer Gemini-model and frontier-AI safety/governance items, now tracked in #1396.

- ✅ **AI-chip finance demotion shipped but generic finance/policy still leads AI news.** PR #1389 closed #1386 by requiring concrete AI action before ranking AI-chip finance as AI news. The 2026-07-10 live review for #1390 confirmed `Find today's AI news` remains fast and readable, but it still led with a UAE export-control / Binance-stablecoin finance-policy story and SK Hynix IPO/market items before clearer AI-agent payments and AI-model evaluation stories, now tracked in #1391.

- ✅ **Broad AI-news finance filtering tightened but AI-chip IPO framing still leads live.** PR #1384 closed #1381 and #1383 by tightening broad finance / policy filtering after PR #1379. The 2026-07-10 live review for #1385 confirmed the path remains fast and readable, but `Find today's AI news` still led with SK Hynix IPO / Nasdaq-opening stories whose AI evidence was mostly chip-boom or Nvidia/customer framing before clearer AI governance and agent-payment stories, now tracked in #1386.

- ✅ **Same-day explicit-AI filtering shipped but broad finance/policy leakage remains.** PR #1379 closed #1376 by tightening same-day AI-news focus after PR #1374. The 2026-07-10 live review for #1380 confirmed the path is still fast and readable, but `Find today's AI news` again ranked broad SK Hynix IPO finance, SpaceX IPO interview, CBDC crypto-policy, and generic coding-process items as normal AI-news bullets before clearer OpenAI / AI-governance stories, now tracked in #1381.

- ✅ **Crypto market background filtering shipped but broader adjacent same-day leakage remains.** PR #1374 closed #1371 by filtering crypto/market background from same-day AI-news answers. The 2026-07-10 live review for #1375 confirmed the crypto-specific leak is gone, but `Find today's AI news` still ranked unrelated or weakly adjacent same-day items (IR TV-volume tooling, generic maintainable-code writing, and UK politics) before explicit OpenAI and AI-agent stories, now tracked in #1376.

- ✅ **Broad AI-chip market-mover filtering shipped but weak adjacent background still leaks into AI-news.** PR #1362 closed #1359 and #1361 by filtering generic AI chip/data-center stock-mover items unless they describe concrete AI infrastructure/product activity. The 2026-07-10 live review for #1363 confirms the AI-news path remains fast with clean story labels and freshness caveats, but still includes weakly related same-day SK Hynix IPO finance and fake portable-air-conditioner internet-ad stories as AI-news background, now tracked in #1364.

- ✅ **AI-news story-label polish shipped but live topic leakage regressed.** PR #1357 closed #1354 and #1356 by replacing provider-style `category` / `posted` / `source` rows with cleaner story bullets and concise source/date labels. The 2026-07-10 live review for #1358 confirms the formatting is better and fast, but `Find today's AI news` again leads with broad finance/crypto/general-internet items ahead of clearly AI-specific stories, now tracked in #1359.

- ✅ **AI-news topic filtering shipped; caveat formatting remains rough.** PR #1349 closed #1346 by filtering crypto token movers from AI-news answers, and PR #1352 closed #1351 by filtering AI-themed token portfolio finance items. The 2026-07-10 live review for #1353 now sees `Find today's AI news` stay focused and correctly caveat that no same-day 2026-07-10 results are available, but the main answer still exposes provider-style metadata, full source URLs, and truncated snippets, now tracked in #1354.

- ✅ **AI-news bullet readability polish shipped but live topic leakage remains.** PR #1344 closed #1341 by polishing AI-news bullets after the 2026-07-09 review for #1340; the 2026-07-09 live review for #1345 still saw `Find today's AI news` include broad token-mover and crypto-policy headlines ahead of/on par with genuine AI stories, now tracked in #1346.

- ✅ **Broad finance leakage filtered from AI-news answers.** PR #1339 closed #1336 by filtering or demoting broad finance items after the 2026-07-09 review for #1335; the 2026-07-09 live review for #1340 now sees `Find today's AI news` return same-day AI-related stories quickly, but the bullets still read like feed rows with category/source metadata, full URLs, and truncated snippets, now tracked in #1341.

- ✅ **AI-news topic focus tightened but broad finance leakage remains.** PR #1334 closed #1331 by making `Find today's AI news` lead with clearly AI-related same-day results after the 2026-07-09 review for #1330; the 2026-07-09 live review for #1335 now sees AI/data-center and AI-infrastructure stories first in about 1.0s, but the answer still includes broad finance/stock-mover headlines as normal AI-news bullets, now tracked in #1336.

- ✅ **Weather-backed guest answer latency reduced.** PR #1329 closed #1326 by reducing the live weather-backed guest path from the ~17.5s seen in the 2026-07-09 review for #1324 to about 1.6s in the 2026-07-09 live review for #1330; `Weather in New York today` now returns a correct, dated, source-backed Google Weather answer quickly, so #1326 is not re-queued. The same #1330 review found the next top core-loop friction in `Find today's AI news`, which is fast but currently promotes unrelated fresh general-feed items as AI news, now tracked in #1331.

- ✅ **AI-news topic relevance shipped; weather latency is now the top live friction.** PR #1322 closed #1319 by keeping `Find today's AI news` topic-relevant after PR #1317. The 2026-07-09 live guest review for #1324 now sees AI-news stay on AI-related stories with a freshness caveat, so #1319 is not re-queued; the same review found correct weather output taking about 17.5s, now tracked in #1326.

- ✅ **AI-news recency preservation shipped but live output is now off-topic.** PR #1317 closed #1314 by preserving recency in AI-news shortcut queries, so #1314 is not re-queued; the 2026-07-09 live guest review for #1318 now sees `Find today's AI news` complete in about 0.8s through `news_search` with fresh 9 July items, but the lead items are unrelated Quran/hadith/name-of-God reminder entries and finance headlines before one AI-adjacent developer article, now tracked in #1319.

- ✅ **Native AI-news freshness caveats shipped but live output still leads stale.** PR #1312 closed #1309 by guarding native AI-news freshness caveats, so #1309 is not re-queued; the 2026-07-09 live guest review for #1313 still saw `Find today's AI news` complete in about 0.5s through `news_search` but lead with 14 May, 11 May, and 12 March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1314.

- ✅ **Freshness-sorted stale AI-news results shipped but live output still leads stale.** PR #1307 closed #1304 by keeping stale AI-news results freshness-sorted, so #1304 is not re-queued; the 2026-07-08 live guest review for #1308 still saw `Find today's AI news` complete in about 0.5s through `news_search` but lead with a 14 May 2026 story for a 8 July 2026 request and no up-front freshness caveat, now tracked in #1309.

- ✅ **Stale replay ordering guard shipped but live deployed output still leads stale.** PR #1302 closed #1299 by guarding mostly-stale replay ordering in tests, so #1299 is not re-queued; the 2026-07-08 live guest review for #1303 still saw `Find today's AI news` complete in about 1.0s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1304.

- ✅ **Dated fresh AI-news prioritization shipped but live deployed output still leads stale.** PR #1297 closed #1292 by preferring dated fresh `news_search` results over undated matches in tests, so #1292 is not re-queued; the 2026-07-08 live guest review for #1298 still saw `Find today's AI news` complete in about 0.8s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1299.

- ✅ **Stale AI-news replay freshness sorting shipped but live deployed output still leads stale.** PR #1290 closed #1287 by keeping stale AI-news fallback ordering freshness-sorted in tests, so #1287 is not re-queued; the 2026-07-08 live guest review for #1291 still saw `Find today's AI news` complete in about 0.6s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1292.

- ✅ **Guard stale AI-news replay ordering shipped but live AI-news output still leads stale.** PR #1285 closed #1282 by keeping stale replay candidates behind freshness guards in tests, so #1282 is not re-queued; the 2026-07-08 live guest review for #1286 still saw `Find today's AI news` complete in about 0.8s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1287.

- ✅ **Guarded native news replay shipped but live AI-news output still leads stale.** PR #1278 closed #1275 by replaying guarded native news answers, so #1275 is not re-queued; the 2026-07-08 live guest review for #1281 still saw `Find today's AI news` complete in about 0.6s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1282.

- ✅ **Dotted-tool news freshness handling shipped but live AI-news output still leads stale.** PR #1268 closed #1263 by handling native news freshness for dotted tools, so #1263 is not re-queued; the 2026-07-08 live guest review for #1269 still saw `Find today's AI news` complete in about 1.3s through `news_search` but lead with May/March 2026 stories before a 2 July item and no up-front freshness caveat, now tracked in #1270.

- ✅ **Native news search freshness caveat shipped but live AI-news output still leads stale.** PR #1261 closed #1258 by ensuring the native news search path can surface freshness caveats, so #1258 is not re-queued; the 2026-07-07 live guest review for #1262 still saw `Find today's AI news` complete in about 0.6s through `news_search` but lead with May/March 2026 stories for a July 7 request without an up-front caveat, now tracked in #1263.

- ✅ **Indexed news timestamps shipped but live AI-news output still leads stale.** PR #1256 closed #1253 and #1255 by using indexed timestamps when `posted_at` metadata is missing, so #1253 is not re-queued; the 2026-07-07 live guest review for #1257 still saw `Find today's AI news` complete in about 0.5s through `news_search` but lead with May 2026 stories for a July 7 request without an up-front freshness caveat, now tracked in #1258.

- ✅ **News search freshness metadata shipped but live output still leads stale.** PR #1251 closed #1248 by adding freshness metadata to the native news search API, so #1248 is not re-queued; the 2026-07-07 live guest review for #1252 still saw `Find today's AI news` complete in under 1s through `news_search` but lead with May/March 2026 stories and no up-front caveat, now tracked in #1253.

- ✅ **AI-news freshness sorting robustness shipped but live output still leads stale.** PR #1246 closed #1243 by tightening freshness sorting around AI-news candidates, so #1243 is not re-queued; the 2026-07-07 post-merge live guest review still saw `Find today's AI news` complete in under 1s through `news_search` but lead with March-May 2026 stories before a 2 July item and no up-front caveat, now tracked in #1248.

- ✅ **Daily wallet credit transfer cap shipped.** PR #1241 closed #1188 by adding a per-account daily budget for wallet credit transfers, so the previous wallet trust item is no longer active. The 2026-07-07 product review for #1242 found no open codex PRs/issues after that merge, then re-opened the remaining live freshness friction as #1243.

- ✅ **Native AI-news mostly-stale caveat shipped.** PR #1235 closed #1232 by holding the native news stream long enough to prepend freshness caveats before stale evidence reaches the user, so #1232 is no longer active; the 2026-07-07 live guest review for #1238 still shows fast, source-linked `news_search` output, but the closed issue should not remain in the ranked queue. PR #1237 later added marketing-note coverage for the same freshness-streaming angle.

- ✅ **Mostly-stale AI-news guard shipped but live native output still leads stale.** PR #1229 closed #1226, so #1226 is not re-queued; the 2026-07-07 live guest review for #1231 still saw `Find today's AI news` complete in about 0.5s through `news_search` but lead with March-May 2026 stories before one 2 July item and no up-front mostly-stale/no-current-results caveat, now tracked in #1232.

- ✅ **Mostly-stale AI-news freshness guard merged but live output still leads stale.** PR #1224 closed #1221 by adding a mostly-stale guard, so #1221 is not re-queued; the 2026-07-07 live guest review for #1225 still saw `Find today's AI news` lead with May/March 2026 stories before one 2 July item and no up-front caveat, now tracked in #1226.

- ✅ **Web-fetch DNS SSRF hardening shipped.** PR #1219 closed #1189 by resolving and blocking private/link-local/loopback/metadata targets before `web_fetch` connects, so the previous top security item is no longer in the active queue. The 2026-07-07 product review for #1220 therefore re-ranked the remaining open work around the live first-run ask → answer freshness gap and wallet transfer trust.

- ✅ **AI-news stale-only guard shipped.** PR #1215 closed #1212 by guarding live AI-news answers that only have stale stories; the 2026-07-06 product review for #1216 now sees `Find today's AI news` complete in about 0.7s through `news_search` with clean links, but the closed stale-news item is no longer re-queued. The remaining open codex queue is security/trust refinement for existing agent-callable services and wallet actions.

- ✅ **Native AI-news freshness stream hold shipped but live answers still lead stale.** PR #1210 closed #1207 by holding native AI-news streams until the freshness guard can run; the 2026-07-06 live guest review for #1211 still saw `Find today's AI news` complete in about 0.8s through `news_search` but stream March–May 2026 stories first with no up-front stale-only/no-current-results disclosure, now tracked in #1212.

- ✅ **Stale news native-streaming guard shipped but live streamed AI-news still misses the caveat.** PR #1205 closed #1202 by guarding stale news native streaming; the 2026-07-06 live guest review for #1206 still saw `Find today's AI news` complete in about 0.7s through `news_search` but stream May/March 2026 stories first with no up-front stale-only/no-current-results disclosure, now tracked in #1207.

- ✅ **Stale AI-news fallback caveat patch merged but live disclosure still misses.** PR #1200 closed #1197 by adding another stale-only fallback caveat path; the 2026-07-06 live guest review for #1201 still saw `Find today's AI news` complete quickly but lead with March-May 2026 stories with no up-front stale-only disclosure, now tracked in #1202.

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
