# Mu

**Tools for agents.** The everyday internet — news, mail, search, weather, markets, video, storage — as tools an agent can call over MCP and REST, paid per request in USDC via x402, with no account in the way.

The claim that matters is *one account instead of a hundred*. An agent that wants news, mail, search, weather, markets, places and somewhere to keep records otherwise needs six or seven providers: six signups, six cards on file, six tokens to rotate. Mu is one balance and one protocol, and for an agent paying per request over x402, no signup at all. **Removing the barrier is the product.** Sometimes that means running the thing ourselves — the SMTP server with DKIM is real, because a sending domain is not something you can casually acquire. Sometimes it means paying a provider so the caller doesn't have to hold that relationship. Both are legitimate; the test is whether the caller is spared an account, not whether we wrote the backend.

This replaces an earlier line — *real tools, not wrappers* — which was wrong in a way that cost us. It made "did we build the backend" the measure, which caps breadth at what one team can operate, and breadth concentrated behind one account is the whole value. Depth still matters where depth is what removes the barrier. It is not the point on its own.

Two doors onto one set of services. An agent calls `/mcp`; a person signs in and gets the home screen — cards per service, agent inline, apps, balance. Nothing is built twice, and a new service appears in both at once. **Keep the signed-in app intact** — it is not legacy, it is the proof the tools work.

Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable — run an instance and anyone paying to call your tools pays you.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain is a package under `service/`, one directory per service
- **Agents** — `agent/micro/` contains specialised micro-agents per domain, routed by keyword + LLM. `agent/<name>/` is an agent that writes into the service of the same name: `agent/blog` composes the daily opinion by asking the registry what exists rather than naming services in code, `agent/social` surfaces breaking stories. The service stores; the agent decides what is worth storing
- **Channels** — Discord (`client/discord/`), Telegram (`client/telegram/`), WhatsApp (`client/whatsapp/`)
- **Protocols** — MCP server at `/mcp`, A2A at `/a2a`, x402 crypto payments
- **AI** — `internal/ai/` supports Anthropic Claude, Atlas Cloud (DeepSeek), OpenRouter, and local models (Ollama)
- **Config** — `internal/settings/` for live-reloadable settings, admin UI at `/admin/env`

## Key Packages

| Package | Purpose |
|---------|---------|
| `agent/` | Main agent pipeline (plan → execute → synthesise) |
| `agent/micro/` | Multi-agent system — registry, router, executor, orchestrator |
| `agent/blog/` | Writes the daily opinion. Reads news, markets, video, prayer and the web, and calls `blog.CreatePost` — the five imports the blog service used to carry |
| `agent/social/` | Decides which headlines are worth surfacing, and calls `social.SurfaceBreaking` |
| `service/news/` | RSS feed aggregation, sentiment tagging |
| `service/markets/` | Crypto, stocks, futures, commodities, currencies via CoinGecko/Yahoo |
| `service/mail/` | SMTP server, DKIM, inbound filtering |
| `service/blog/` | Microblogging with AI-generated daily digests |
| `internal/ai/` | LLM abstraction — Anthropic, Atlas Cloud, OpenRouter, local models |
| `internal/api/` | MCP server, tool registry |
| `internal/app/` | Web UI framework, templates, middleware |
| `internal/auth/` | Account system, sessions, passkeys |
| `internal/notes/` | The store behind `service/notes` — a title, its text, and nothing that expires |
| `internal/settings/` | Live-reloadable configuration |
| `home/` | Landing page, assistant, home dashboard, summary |
| `client/discord/` | Discord bot with slash commands, embeds, briefings |
| `client/telegram/` | Telegram bot with commands and groups |
| `client/whatsapp/` | WhatsApp Business API integration |
| `account/` | Who you are and what you can afford: sign-in, passkeys, tokens, Google, and the credit ledger with Stripe behind it. The balance is the first card on /account. Account furniture, not a service — no Spec and no tools |
| `service/wallet/` | A key of your own on Base: an address that holds USDC, and paying an x402-priced tool on another server with it. Capped per call and per day, because the agent reads text strangers wrote |
| `internal/x402/` | The toll on the door: price a request, write the challenge, verify and settle the payment. A protocol, not a capability — no page, no tool, no state |
| `internal/contacts/` | The address book itself. `service/contacts` is the tools, the page and the Google People bridge over it; `service/sms` asks it whether a number is known before texting a stranger |
| `internal/linkmeta/` | What a link looks like when you show it: the Open Graph tags behind a URL, cached on disk. News fills it, social reads it, and it belongs to neither — the files stay at `news/metadata/` because that is where every instance already has them |
| `internal/phone/` | Who a phone number belongs to: normalising it, and the proof that an account owns it. Shared by `service/sms` and `service/whatsapp`, because a number is a number whichever message it carries |
| `internal/twilio/` | The provider under `service/sms` and `service/whatsapp`: credentials, the send, and the webhook signature. It holds no opinion about what may be sent |
| `internal/gtfs/` | Published transit timetables — the format almost every agency in the world uses, so `transit` speaks one thing and answers everywhere. Nothing is ever unpacked: Berlin is 75MB zipped and 590MB open, so the zip is streamed into a fixed-width array of departures, eight bytes each, seeked into rather than held. Times are the agency's own zone, which is the bug that empties a feed if you get it wrong |
| `internal/quota/` | What things cost and who may do them. The only thing a service knows about money — it holds prices, not balances. Prices are `quota.json` at the top level, not Go |
| `service/docs/` | The caller's own documents: a title and a markdown body. It was a record store wearing the word — the tool took a collection and a bag of JSON, so the page asked a person to type JSON. A service is named for a kind of thing somebody makes, never for how it is stored; the record store stays underneath as `internal/userdb`, reached by apps as `mu.db`, with no page and no tools |
| `service/files/` | Per-user file storage — keep a file, get a URL, read it back |
| `service/contacts/` | The caller's address book, so a name resolves to an address (headless) |
| `service/whatsapp/` | Reply to people on WhatsApp, through Twilio. Bounded by Meta's 24-hour window, so it answers rather than initiates. The Meta bot in `client/whatsapp/` is the other half — a door, not a capability |
| `service/sms/` | A phone number: text somebody, read what they text back. Twilio. The rules about who you may text are the service, not decoration — see the package comment |
| `service/web/` | The open web: search it (`web.Search`), fetch a URL (`web.Fetch`). The Brave provider, the readability reader and the /search page live here too — they were `service/search`, a directory under `service/` that was not a service |
| `service/routes/` | Getting from one place to another: time with traffic, turn-by-turn, and which of several is nearest. It was one ETA inside `places`, whose own package comment gave the game away — "places could already tell an agent what is nearby and where it is, but not whether it is worth going". Those are two questions: `places` is the Places API's domain, this is the Routes API's. Not `routing`, which is what it does; a service is named for a domain. The page draws the route from the polyline Google already returned, so there is no map tile to buy |
| `service/stream/` | The console — this instance's own event timeline |
| `service/chat/` | Live discussion rooms attached to an item |
| `internal/profile/` | The public face of an account: the page at /@username, and who is online |
| `docs/` | Three embedded pages — /about, /help, /install — plus the markdown the repository keeps for itself (`ARCHITECTURE.md`, `PRODUCT.md`, `PRICING.md`, `SECURITY.md`, `PRINCIPLES.md`, `USECASES.md`, `LISTING.md`), which is not served. `PRODUCT.md` is the one-pager: what Mu is, who arrives, and what Home is for. `PRICING.md` is its companion: what things cost, who pays, and how we make money — one meter not two, when a varying price is two endpoints and when it is units, and what must never be free. Check changes against both. The /docs route is the Docs service |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
gofmt -l .              # formatting — CI gates on this and nothing above catches it
```

CI runs `gofmt -l .` and fails on any output. None of build, test or vet
look at formatting, so a change can pass everything run by hand and fail the
first thing CI does — which is how it stayed red across several commits.

## What a service is

Three sentences, and most of the arguments this repo has had with itself follow
from them:

**Services are the fundamental building block. Tools are derived from services.
Agents use tools.**

A service answers a question about state: request in, response out,
deterministic given the data, callable by anything. That shape is exactly what
makes a tool derivable from it. An agent takes a goal and decides which
questions to ask — it consumes the catalogue, so it cannot be in it.

- Every `service.Spec` lives under `service/`. One exception would be one too
  many: the moment a Spec lives elsewhere, "what is a service" stops being
  checkable and starts being something you have to remember.
- Nothing that consumes tools declares a Spec — not `agent/`, not `account/`,
  not `home/`, `client/` or `admin/`.
- Nothing registers a tool by hand. A tool with nowhere to come from is a
  service that has not been written yet.

Enforced by `test/services_test.go`.

**A balance is not a service.** How an account pays is the same shelf as
changing your email or rotating a token, so it lives in `account/` with the rest
of the account and has no Spec, no tools and no entry in the catalogue. It was
in `/services` once under the name Wallet, and the fix was a boolean called
`Staple` meaning "is a service, but hide it": that flag was the error made
legible, and deleting it was the actual fix. What a service needs to know about
money is still `internal/quota`.

That word `wallet` used to mean two things — a ledger you top up with a card,
and a key that signs — in one directory, writing one file, destroying each
other's data. They are separated now. The ledger is the account's; the key is
`service/wallet`, which *is* a service by the definition above: it answers
questions about state and an agent can derive tools from it.

**The agent is not a service** either, for the same reason stated the other way
round: it is the thing that reads the catalogue. `agent_ask` and `agent_list`
were tools; an MCP client calling `agent_ask` already holds every tool this
agent holds, and listing your agents is a page, which exists.

## Layering

The top level is the product — `home/`, `agent/`, `service/`, `client/`,
`admin/`, `account/`. Each is a staple: it owns something nothing else owns, and
a user can name it. Underneath is `internal/`, which is everything with no name
a user would recognise.

**The product may import `internal/`. `internal/` may never import the product.**
The two exceptions are the programs: `internal/server` and `internal/cli`
assemble everything, so they import everything. Enforced by
`test/layering_test.go`.

A nav item is a *view* of a staple, and a staple can have more than one — Tools
and Services are both `service/`, Account and Usage are both money. A view owns
nothing, which is why Tools has no directory and should not get one.

When two packages genuinely need each other, the cycle is broken with a function
variable filled in by `internal/server/hooks.go`. That file is the ledger of
this layering debt: every entry is a cycle somebody could not avoid. Prefer a
plain downward import; when you cannot have one, add the hook and know it cost
something.

**Services never import each other.** Product may import `internal/`, and that
says nothing about sideways: `flights` imported `places` for a geocoder,
`whatsapp` imported `sms` for phone-number ownership. A sideways import makes two
services one unit — read together, changed together, moved together — and the
catalogue stops being a list of independent things. Whatever they share goes in
`internal/`, never in a non-service directory under `service/`, because "one
directory per service" is only checkable while it is true. Enforced by
`TestServicesDoNotImportEachOther`, whose allowlist is empty and should stay that
way.

A service never imports `account/`. What a service needs to know about money is
`internal/quota` — what an operation costs and whether this caller may do it.
Quota holds prices and does not know what a balance is; `account/` fills in the
half quota cannot answer, from its own `init`, because quota sits underneath it.
Enforced by `TestNoServiceImportsTheAccount`.

This rule does not touch `service/wallet`, which holds a key rather than a
balance — and a sideways import from it to `account/` would be caught here like
any other.

## Conventions

- No external dependencies for crypto (secp256k1, RLP, ECDSA implemented in pure Go in `service/wallet/evm.go`)
- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- All client integrations follow the same pattern: auto-create accounts, conversation history, public/private mode
- The main branch is `main`
- One directory per service under `service/`, named for the service — see
  "What a service is" above. `internal/service` is the runtime core that hosts
  them, not a service itself. See
  `docs/ARCHITECTURE.md` for what is registered, which are headless, which
  are account-scoped, and which are deliberately not exposed to the agent
- A service is named for a **domain** (a noun), never an action. Tool names are
  derived as `service_method`, so an action-named service leaves its main method
  nothing to be called but the same word — that is how `search.Search` produced
  the tool name `search_search`. Methods returning the current set of something
  are all called `List`. Enforced by `TestNoMethodRepeatsItsService`

## The go-micro relationship

go-micro is the substrate Mu is built on. It has three roles, and they must stay separate.

**One-way dependency.** Mu depends on go-micro. go-micro must never depend on Mu.
go-micro changes when *framework users* need something — never because Mu needs
something. If Mu needs a capability go-micro lacks, it goes in Mu's own code, or
Mu vendors or forks. A PR to `micro/go-micro` whose justification is "Mu needs
this" is the failure mode: reject it and put the code in `micro/mu`.

This is a policy, not an architecture. It is cheap to hold, and it targets a
specific failure already lived through once: in 2019 a business was built that
*depended on* go-micro, so the framework had two masters — a library wants
stability and generality, a runtime wants opinionated constraints — and the
engineering budget went on reconciling them instead of shipping either.

Mu pins a released go-micro version. Forking is permitted and expected.

**Not in the product surface.** A Mu user should never meet the words *go-micro*,
*microservice*, or *framework* in the UI, in onboarding, in marketing copy, or in
anything the agent says about itself. If a user-visible string mentions them,
that is a bug. This covers the agent's system prompts and the public status page,
which are easy to forget.

The word *service* is fine — it is ordinary English, and the internal convention
(service name == route == nav label == tool prefix) is good and invisible to
users. What is banned is implementation vocabulary, not the concept.

The README and developer docs are exempt: they are the funnel, and they are
facts. The distinction is developer-facing (say it) vs customer-facing (don't).

## Pricing

A credit is charged when an operation costs us something to run: a model call,
or a paid third party (Atlas Cloud for inference and images, Brave for web
search, Google for places). Operations that only touch this instance's own
storage are free — see `quota.json` at the top of the repo, which is the one
price list: the gate reads it and every cost table renders from it. `main.go`
embeds it and calls `quota.Load`; the package that answers what something costs
does not decide where the answer comes from. A service says *which* operation
it charges (`Cost: quota.OpWebSearch` on its Endpoint); what that operation
costs is an operator's decision, so it is data.

Abuse control is `auth.CheckPostRate`, not the credit charge. Keep the two jobs
separate: credits price real cost, rate limits stop bots.
