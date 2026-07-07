# The 100 building blocks of Mu

Mu is an evolutionary architecture: a small set of composable **service-genes**,
each a go-micro service that does one thing, assembled and evolved by agents. This
document identifies the 100 blocks that make up Mu.

## Principles

1. **Each block is a service.** One responsibility, a stable request/response
   contract, independently testable and replaceable. The agent layer routes,
   composes, and improves them; the blocks don't know who calls them.
2. **Reuse first — no new external senses.** Blocks are carved out of what Mu
   already has (the packages under the repo root and `internal/`) plus the
   internal AI layer. We do not add external APIs to add a capability; we
   recombine existing ones. Mu's current external inputs — RSS, CoinGecko/Yahoo,
   Brave, Stripe, Coinbase CDP, reminder.dev — are its fixed sensory organs, not
   to be multiplied.
3. **The genome is fine-grained; composition is emergent.** Today Mu ships ~10
   coarse services (`news, markets, mail, weather, blog, social, video, search,
   recall, apps`). The 100 below are the atomic decomposition those grow into —
   e.g. "news" becomes feed-registry + poller + reader + sentiment + digest.
4. **Agents manage and evolve blocks.** A block is a gene: agents can improve one
   in place (better ranker), recombine several (news + markets → a correlated
   brief), or grow a new one from existing parts. Evolution operates on this set.

## Layout

The 100 blocks in 14 layers, substrate → surface. Each line is `name — purpose
(builds on: existing package/data)`.

### 1. Substrate & state (1–9)
1. **accounts** — create, look up, tear down user accounts (auth)
2. **sessions** — issue and validate session tokens (auth)
3. **passkeys** — WebAuthn credential register/verify (auth)
4. **api-tokens** — PAT/bearer issuance for API + CLI (auth)
5. **settings** — live-reloadable config get/set (settings)
6. **memory** — per-user, scope-namespaced key/value memory (memory)
7. **scheduler** — cron/interval job runner for background loops (main loops)
8. **event-bus** — internal publish/subscribe (event)
9. **store** — versioned JSON persistence + snapshots/rollback (data, snapshot)

### 2. Ingestion — sources to raw items (10–16)
10. **feed-registry** — manage the set of RSS/source feeds (news)
11. **feed-poller** — scheduled fetch of registered feeds (news)
12. **web-fetch** — fetch a URL server-side, SSRF-guarded (search)
13. **reader** — readability article extraction (search)
14. **normalizer** — canonicalize items to one schema (news, data)
15. **deduper** — drop duplicates by URL/content hash (data)
16. **price-fetcher** — poll market prices from providers (markets)

### 3. Enrichment — raw to enriched (17–26)
17. **summarizer** — LLM summary of arbitrary text (ai)
18. **sentiment** — sentiment tagging (news, ai)
19. **tagger** — topic/keyword tags (ai)
20. **entity-extractor** — people/orgs/tickers/places from text (ai)
21. **classifier** — category/intent classification (ai)
22. **translator** — translate text (ai)
23. **embedder** — vector embeddings for items (ai, data)
24. **ranker** — relevance/importance scoring (data)
25. **clusterer** — group related items into topics (data)
26. **threader** — group messages/posts into conversations (mail, social)

### 4. Knowledge & recall (27–33)
27. **indexer** — write items into the owner-scoped corpus (data)
28. **fulltext-search** — keyword search over the corpus (search, data)
29. **vector-search** — semantic search over embeddings (data)
30. **recall** — unified cross-source recall for a user (recall)
31. **bookmarks** — save/unsave items (user)
32. **query-history** — per-user query/answer history (agent flows)
33. **owner-scope-guard** — enforce per-account scoping on reads/writes (data)

### 5. News (34–38)
34. **headlines** — latest headlines by topic (news)
35. **topics-trending** — topic taxonomy and what's trending (news)
36. **article-read** — fetch + extract a single article (news)
37. **news-search** — search the news corpus (news)
38. **news-digest** — compose a news brief (news, home)

### 6. Markets (39–44)
39. **prices** — live prices by category (markets)
40. **movers** — top gainers/losers (markets)
41. **watchlist** — per-user tracked symbols (markets, memory)
42. **convert** — currency/crypto conversion (markets)
43. **chart-series** — historical price series (markets)
44. **price-alerts** — threshold alerts (markets, scheduler)

### 7. Weather & places (45–48)
45. **forecast** — weather/conditions by coordinates (weather)
46. **geocode** — place name to coordinates (places)
47. **nearby** — places near a point (places)
48. **weather-digest** — a daily conditions brief (weather)

### 8. Mail (49–55)
49. **inbox** — list/read received mail (mail)
50. **mail-send** — send internal + external mail (mail)
51. **mail-threads** — thread grouping (mail)
52. **spam-filter** — inbound spam/abuse scoring + rules (mail)
53. **dkim-signer** — DKIM signing of outbound mail (mail)
54. **mail-summary** — summarize a message or thread (mail, ai)
55. **mail-notify** — new-mail notifications to channels (mail, clients)

### 9. Social, blog & video (56–63)
56. **social-feed** — public feed read (social)
57. **social-post** — create a post/status (social, user)
58. **social-thread** — conversation threads (social)
59. **blog-posts** — author/edit/delete posts (blog)
60. **blog-notes** — AI daily notes / opinion voice (blog, ai)
61. **video-latest** — latest videos (video)
62. **video-search** — search videos (video)
63. **activitypub** — federation in/out (blog)

### 10. Productivity & apps (64–70)
64. **reminders** — reminder content (reminder)
65. **lists** — user lists/trackers (apps)
66. **counters** — counter apps (apps)
67. **checklists** — checklist apps (apps)
68. **app-builder** — build a micro-app from a prompt (apps)
69. **app-runner** — sandboxed app execution (apps)
70. **app-directory** — search/read the app catalog (apps)

### 11. Wallet & commerce (71–79)
71. **credits-ledger** — credit balance + transactions (wallet)
72. **credit-transfer** — move credits between users (wallet)
73. **topup-stripe** — card top-up (wallet)
74. **base-wallet** — per-user USDC wallet on Base (wallet)
75. **x402-pay** — pay metered MCP tools from the wallet (wallet)
76. **usdc-convert** — sweep USDC into credits (wallet)
77. **spend-limits** — per-call/daily spend caps (wallet)
78. **quota-charge** — charge a metered operation (wallet)
79. **pricing-catalog** — per-tool/op price list (wallet, api)

### 12. Agent core (80–88)
80. **router** — route a query to the right domain/agent (agent/micro)
81. **planner** — plan the tool calls for a query (agent)
82. **executor** — run a tool call, record metrics (agent, api)
83. **synthesizer** — compose the final answer from tool results (agent)
84. **tool-registry** — register/look up callable tools (api)
85. **agent-registry** — built-in domain agents (agent/micro)
86. **user-agents** — user-defined agents CRUD (agent/micro)
87. **identity-injector** — bind the caller's account into tool calls (agent)
88. **flows** — persist and continue multi-turn conversations (agent)

### 13. Channels & protocols (89–95)
89. **web-ui** — the web app surface (internal/app, home)
90. **mcp-server** — the MCP endpoint (internal/api)
91. **a2a-server** — agent-to-agent protocol (internal/a2a)
92. **discord** — Discord bot channel (client/discord)
93. **telegram** — Telegram bot channel (client/telegram)
94. **whatsapp** — WhatsApp channel (client/whatsapp)
95. **event-stream** — the public event stream (stream)

### 14. Trust & safety (96–100)
96. **rate-limiter** — per-account/IP request limits (auth)
97. **content-moderation** — moderate user-generated content (flag, ai)
98. **flag-report** — flag/hide reported content (flag)
99. **block-mute** — user block/mute relationships (user, auth)
100. **access-control** — invites, bans, admin gates (auth, admin)

## How the set evolves

- **Improve in place** — a block gets a better implementation behind the same
  contract (e.g. a smarter `ranker` or `spam-filter`). Callers are unaffected.
- **Recombine** — an agent composes several blocks into a higher-order answer
  (`news-digest` + `market-digest` + `weather-digest` → a morning brief) without
  a new block.
- **Grow** — a genuinely new capability is a new block built from existing parts,
  not a new external dependency. If a block would need a new external API, that is
  a deliberate decision to add a sensory organ, made rarely and explicitly.

The 100 is a living genome: blocks merge when redundant and split when one is
doing two jobs. Keeping it near 100 is a forcing function for the right grain —
small enough to reason about and evolve, large enough to cover Mu.
