# Mu

**A personal agent.** It has an email address; write to it and it answers — in the thread, remembering the last one. Behind it is the everyday internet — news, mail, search, weather, markets, video, storage — as tools it can call over MCP and REST, paid per request in USDC via x402, with no account in the way.

The line has moved before — *building blocks for life*, *tools for agents*, *an inbox for agents* — and this one is meant to stop moving, because it is a category rather than a claim. The others were arguments about why Mu matters, and an argument has to be remade every time the emphasis shifts. "A personal agent" is what it *is*, in a category a reader already holds, and the proof follows it rather than replacing it: it has an address, you can email it, and there are a hundred-odd tools behind it. Underneath, structurally, this is a personal server with a messenger at the front — that is the thesis. Do not rewrite the first sentence when the emphasis moves; move the sentence after it.

The lead was *tools for agents* and the tools are still what is behind it. What changed is which fact goes first: every provider ships an MCP server now, and none of them ship an agent that is permanently reachable and remembers. **An address is the smallest interface there is** — no SDK, no OAuth, no protocol to adopt, nothing on the other side — so a person, another agent, a form or a cron job can all write to one. That is what makes an agent something you have rather than something you visit.

The inbox is not email. It is `internal/thread` — every conversation this account has had with an agent, on whichever client it arrived, in one record read at `/inbox`. Email is the channel we lead with because it is the only one that needs nothing from anybody to start using; mail and the web write to the same record, and a new one joins it rather than starting a second.

The claim underneath is still *one account instead of a hundred*. An agent that wants news, mail, search, weather, markets, places and somewhere to keep records otherwise needs six or seven providers: six signups, six cards on file, six tokens to rotate. Mu is one balance and one protocol, and for an agent paying per request over x402, no signup at all. **Removing the barrier is the product.** Sometimes that means running the thing ourselves; sometimes it means paying a provider so the caller doesn't have to hold that relationship. Both are legitimate; the test is whether the caller is spared an account, not whether we wrote the backend. Which half of that a given service falls on is an implementation detail and never the pitch — running our own SMTP server kept getting written down as though it were the differentiator, and it is not. It is good plumbing for messaging, and an inbox an agent can use.

This replaces an earlier line — *real tools, not wrappers* — which was wrong in a way that cost us. It made "did we build the backend" the measure, which caps breadth at what one team can operate, and breadth concentrated behind one account is the whole value. Depth still matters where depth is what removes the barrier. It is not the point on its own.

Two doors onto one set of services. An agent calls `/mcp`; a person signs in and gets the home screen — cards per service, agent inline, apps, balance. Nothing is built twice, and a new service appears in both at once. **Keep the signed-in app intact** — it is not legacy, it is the proof the tools work.

Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable — run an instance and anyone paying to call your tools pays you.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain is a package under `service/`, one directory per service
- **Agents** — `agent/micro/` contains specialised micro-agents per domain, routed by keyword + LLM. `agent/<name>/` is an agent that writes into the service of the same name: `agent/blog` composes the daily opinion by asking the registry what exists rather than naming services in code, `agent/social` surfaces breaking stories, `agent/digest` writes the daily briefing. The service stores; the agent decides what is worth storing
- **Channels** — Mail (`client/mail/`). There were three more — Discord, Telegram and a Meta WhatsApp bot — and they are deleted: 2,100 lines and three third-party APIs carrying no traffic, against `agent.Ask` at 362. A client is meant to be a thin translation and Discord had grown its own identity store, its own usage tracking and a model call of its own
- **Protocols** — MCP server at `/mcp`, an agent endpoint at `POST /agent/<name>`, REST at `/api/v1/`, x402 crypto payments. There was an A2A door at `/a2a`; it ran a generic account-less orchestration that was nobody's agent, so it was deleted rather than reconciled with the other three. Everything upstream of the mux that a tool door needs — wallet signature, auth challenge, payment gate — asks `api.ToolDispatch(path)` rather than naming a path, because a second door otherwise starts out unpriced
- **AI** — `internal/ai/` supports Anthropic Claude, Atlas Cloud (DeepSeek), OpenRouter, and local models (Ollama)
- **Config** — `internal/settings/` for live-reloadable settings, admin UI at `/admin/env`

## Key Packages

| Package | Purpose |
|---------|---------|
| `agent/` | Main agent pipeline (plan → execute → synthesise) |
| `inbox/` | The agentic inbox: what arrived, and the agent that works on it. A staple by the usual test — you can name it and click it — and it grew inside `agent/` on the reasonable-looking grounds that both render conversations. They are not the same: the chat is a room you talk in, the inbox is where things turn up whether or not you are in it. Not the mail service (`service/mail` is the MTA and the store) and not the record (`internal/thread` is), so deleting it loses the pages and nothing else. It may not import `agent/`; where it needs the workflow record the agent hands it over — see `inbox.Tools` |
| `agent/micro/` | Multi-agent system — registry, router, executor, orchestrator |
| `agent/blog/` | Writes the daily opinion. Reads news, markets, video, prayer and the web, and calls `blog.CreatePost` — the five imports the blog service used to carry |
| `agent/social/` | Decides which headlines are worth surfacing, and calls `social.SurfaceBreaking` |
| `service/news/` | RSS feed aggregation, sentiment tagging |
| `service/markets/` | Crypto, stocks, futures, commodities, currencies via CoinGecko/Yahoo |
| `service/mail/` | SMTP server, DKIM, inbound filtering |
| `service/blog/` | Microblogging with AI-generated daily digests |
| `internal/ai/` | LLM abstraction — Anthropic, Atlas Cloud, OpenRouter, local models |
| `internal/api/` | MCP server, tool registry, and the HTTP API. `/api/v1/<service>/<method>` turns a path into a tool name and hands it to the same `ExecuteTool` `/mcp` calls, so it has no auth story or price table of its own; `/api` is its reference, derived from the specs. Not a second agent door — `/mcp` is that, and the two pages link to each other. Two deliberate differences: a destructive method refuses `GET`, and a `POST` resting on the session cookie needs `X-CSRF-Token` (`auth.StrictCSRF`, because `ValidCSRF` lets a request omit the token entirely for pages already in the wild) |
| `internal/app/` | Web UI framework, templates, middleware |
| `internal/auth/` | Account system, sessions, passkeys |
| `internal/notes/` | The store behind `service/notes` — a title, its text, and nothing that expires |
| `internal/thread/` | The system of record: what was said, to whom, on which conversation. Written on every turn from every client, by nobody's decision — see "Clients, and the record between them". Not a service, and not a workflow. `service/recall` is the read over it and `inbox/` is the pages over it — one list of conversations, whichever client each happened on. The record owns the value a client is stored under (`thread.WebClient`); what it is *called* in front of somebody is presentation and lives with `TimeAgo` in `internal/app` — there were two copies of that switch and they had already drifted, labelling the same conversation "Web" on one page and "Here" on another |
| `service/recall/` | Going looking in your own past on purpose: search what was said on any client, read a conversation back. Its page at `/recall` is a search box, not a second list — `/inbox` browses conversations, this searches every message in them. It owns none of what it reads, which is the point: delete it and the record is unaffected |
| `client/mail/` | Mail as a client: the shape a message arrives in, handed to the agent, and the answer turned back into a reply. `service/mail` is the capability underneath — the inbox, the address, the SMTP server |
| `internal/settings/` | Live-reloadable configuration |
| `home/` | Landing page, assistant, home dashboard, summary |
| `account/` | Who you are and what you can afford: sign-in, passkeys, tokens, Google, and the credit ledger with Stripe behind it. The balance is the first card on /account. Account furniture, not a service — no Spec and no tools |
| `service/wallet/` | A key of your own on Base: an address that holds USDC, and paying an x402-priced tool on another server with it. Capped per call and per day, because the agent reads text strangers wrote |
| `internal/x402/` | The toll on the door: price a request, write the challenge, verify and settle the payment. A protocol, not a capability — no page, no tool, no state |
| `internal/contacts/` | The address book itself. `service/contacts` is the tools, the page and the Google People bridge over it; `service/sms` asks it whether a number is known before texting a stranger |
| `internal/linkmeta/` | What a link looks like when you show it: the Open Graph tags behind a URL, cached on disk. News fills it, social reads it, and it belongs to neither — the files stay at `news/metadata/` because that is where every instance already has them |
| `internal/phone/` | Who a phone number belongs to: normalising it, and the proof that an account owns it. Under `service/sms`, and separate from it because ownership of a number is a fact about an account rather than about texting |
| `internal/twilio/` | The provider under `service/sms`: credentials, the send, and the webhook signature. It holds no opinion about what may be sent. Kept separate from the service so a second provider is an adapter rather than a rewrite |
| `internal/gtfs/` | Published transit timetables — the format almost every agency in the world uses, so `transit` speaks one thing and answers everywhere. Nothing is ever unpacked: Berlin is 75MB zipped and 590MB open, so the zip is streamed into a fixed-width array of departures, eight bytes each, seeked into rather than held. Times are the agency's own zone, which is the bug that empties a feed if you get it wrong |
| `internal/quota/` | What things cost and who may do them. The only thing a service knows about money — it holds prices, not balances. Prices are `quota.json` at the top level, not Go |
| `service/docs/` | The caller's own documents: a title and a markdown body. It was a record store wearing the word — the tool took a collection and a bag of JSON, so the page asked a person to type JSON. A service is named for a kind of thing somebody makes, never for how it is stored; the record store stays underneath as `internal/userdb`, reached by apps as `mu.db`, with no page and no tools |
| `service/files/` | Per-user file storage — keep a file, get a URL, read it back |
| `service/contacts/` | The caller's address book, so a name resolves to an address |
| `service/sms/` | A phone number: text somebody, read what they text back. Twilio. The rules about who you may text are the service, not decoration — see the package comment |
| `service/web/` | The open web: search it (`web.Search`), fetch a URL (`web.Fetch`). The Brave provider, the readability reader and the /search page live here too — they were `service/search`, a directory under `service/` that was not a service |
| `service/routes/` | Getting from one place to another: time with traffic, turn-by-turn, and which of several is nearest. It was one ETA inside `places`, whose own package comment gave the game away — "places could already tell an agent what is nearby and where it is, but not whether it is worth going". Those are two questions: `places` is the Places API's domain, this is the Routes API's. Not `routing`, which is what it does; a service is named for a domain. The page draws the route from the polyline Google already returned, so there is no map tile to buy |
| `service/stream/` | The console — this instance's own event timeline |
| `service/chat/` | Live discussion rooms attached to an item |
| `internal/user/` | A person on this instance: the face they show other people (/@username), whether they are here, and what they have decided about everybody else (saved, hidden, blocked, at /user). Not the account — that is `internal/auth`, which holds identity and credentials. It was two packages, `internal/profile` and `service/user`, and the second was a Spec over the caller's own preferences: a noun that is the asker and verbs nobody else can observe, which is the same shelf as the balance. The word `user` is free for the service that is missing — a directory of who exists here, people and agents alike |
| `docs/` | Three embedded pages — /about, /help, /install — plus the markdown the repository keeps for itself (`ARCHITECTURE.md`, `PRODUCT.md`, `PRICING.md`, `SECURITY.md`, `PRINCIPLES.md`, `USECASES.md`, `LISTING.md`), which is not served. `PRODUCT.md` is the one-pager: what Mu is, who arrives, and what Home is for. `PRICING.md` is what things cost, who pays, and how we make money — one meter not two, when a varying price is two endpoints and when it is units, and what must never be free. Check pricing changes against it. There was a `DIRECTION.md` holding the thesis and what to change next; the thesis is the top of this file now, and a roadmap kept in the repository was a second place for the positioning to live and disagree with itself. The /docs route is the Docs service |

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

## Clients, and the record between them

Three ways in — the web page, the CLI, and mail — and the agent is
the same one behind all of them. There were six; Discord, Telegram and a Meta
WhatsApp bot are deleted, and `/a2a` before them. What differs is protocol and nothing else, so a
client's whole job is to translate: parse what arrived, hand it over, render what
comes back, and name an agent where the address or the command already chose one
(`agent+news@`, a slash command).

**Everything else belongs to `agent.Ask`.** Finding the conversation a message
continues, handing the agent what was already said, running it, writing the turn
down, noticing anything worth remembering. Every client wrote that for itself
once and it had drifted five ways: three kept history in a map in memory, keyed
differently, lost on restart and unreadable by anything else; none of them
recorded that a run had happened, so a WhatsApp conversation left no trace; and
memory was extracted on the web alone, so an agent remembered what you typed in
a browser and forgot everything you said anywhere else.

Three words that are not the same thing:

- **History** — the messages of one conversation. Per thread, persisted.
- **Memory** — durable facts about an account, across every client. `notes`.
- **Context** — what is assembled for one run out of both, plus live tool data.
  Assembled fresh each time, never stored.

### The system of record is not a service

`internal/thread` holds what was said: a message, on a thread, on a client, for
an account. It is written on every turn whether or not anybody asks.

**A service is something a caller may choose to use. A system of record is not a
choice.** That is why this is substrate and not `service/threads` — the core of
the product must not sit behind a decision an agent takes, because an agent that
forgot to call it would simply stop remembering. It keeps the company of the
others nobody chooses: `internal/quota` for what things cost, `internal/x402`
for pricing a request, `internal/auth` for who you are.

Reading is a different question and a service over it is welcome, because
searching your own past *is* something an agent decides to do. That service is
`service/recall`. The test for whether it has been built in the right place:
**delete the service and nothing breaks.** Clients still record, the agent still
gets its history, the pages still render; you lose only the ability to go
looking on purpose. If deleting it breaks the product, the core is in the wrong
layer.

One thing recall does own, and it is worth knowing why: deleting an account.
Nothing else in the catalogue knows the record exists — it is written by the
machinery, so no service claimed it and no deletion hook cleared it, and
deleting your account left every conversation you had ever had on disk.

Messages, not events. An event log that accepts anything has no schema and
cannot be queried a year later, and there are already two event-shaped things —
`service/stream` is the instance's public timeline, `internal/usage` the
counters. If a second kind of event ever needs this treatment, a message becomes
a kind of event and the store does not change.

### A thread is not a workflow

`agent.Flow` is a **workflow** record, in go-micro's sense and the ordinary one:
the steps a task took, the tools it called, whether it failed. That is *how an
answer was produced*. `internal/thread` is *what was said*.

They were one struct for a while, which is how a workflow record came to stand in
as conversation history — and they have different lifetimes. A workflow record is
debugging and may expire; a message is memory and should not. One eviction limit
was governing both, which is why it was wrong for each.

## Layering

The top level is the product — `home/`, `agent/`, `service/`, `client/`,
`admin/`, `account/`. Each is a staple: it owns something nothing else owns, and
a user can name it. Underneath is `internal/`, which is everything with no name
a user would recognise.

**The product may import `internal/`. `internal/` may never import the product.**
The two exceptions are the programs: `internal/server` and `internal/cli`
assemble everything, so they import everything. Enforced by
`test/layering_test.go`.

The top level is the sidebar: Home, Account, Tools, Agents, Services. That is
the test for whether something belongs there — a user can name it and click it —
and it is why `tool/` is 280 lines of derivation sitting next to packages twenty
times its size. Small is what a good derivation looks like, not evidence it
belongs in `internal/`.

A nav item can still be a *view* rather than a directory, and a staple can have
more than one — Usage is a view of money, and Services and Tools are two lenses
on the same catalogue. What decides is ownership: `tool/` owns the derivation
from `service.Spec` to `api.Tool`, so it is a directory; Usage owns nothing, so
it is not.

When two packages genuinely need each other, the cycle is broken with a function
variable filled in by `internal/server/hooks.go`. That file is the ledger of
this layering debt: every entry is a cycle somebody could not avoid. Prefer a
plain downward import; when you cannot have one, add the hook and know it cost
something.

**Services never import each other.** Product may import `internal/`, and that
says nothing about sideways: `flights` imported `places` for a geocoder,
`whatsapp` imported `sms` for phone-number ownership, before both that service and
the email one were deleted. A sideways import makes two
services one unit — read together, changed together, moved together — and the
catalogue stops being a list of independent things. Whatever they share goes in
`internal/`, never in a non-service directory under `service/`, because "one
directory per service" is only checkable while it is true. Enforced by
`TestServicesDoNotImportEachOther`, whose allowlist is empty and should stay that
way. Both it and `TestInternalNeverImportsTheProduct` used to stop at the first
directory level — a glob of `service/<name>/*.go`, a pattern ending at the
closing quote — so a package one level deeper was invisible to the rule about
it. The since-deleted `internal/a2a` imported the micro agent and `service/news/digest` imported
markets and video, for a year, under tests whose whole subject was those edges.
Both now walk the subtree. A rule you can get out of by making a subdirectory is
not a rule.

**An agent may import a service; a service may never import an agent.** Same
rule one level up, and this one has a reason rather than a convention behind it:
a service answers a question about state, an agent decides which question to
ask, and a service calling an agent is asking the model what its own answer
should be. Not yet enforced — `tasks.RunAgent`, `events.RunAgent`,
`events.OnFireEvent` and `stream.AIReplyHook` are four hooks that exist to make
that direction compile. See "Where it leaks" in `docs/ARCHITECTURE.md`.

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
  `docs/ARCHITECTURE.md` for what is registered, which are account-scoped, and
  which are deliberately not exposed to the agent. Every service has a page:
  service name == directory == route == nav label == tool prefix, with no
  exceptions in it. A capability with no page is one a person cannot find, and
  the last attempt at "is a service, but hide it" was a boolean whose deletion
  was the fix
- A service is named for a **domain** (a noun), never an action. Tool names are
  derived as `service_method`, so an action-named service leaves its main method
  nothing to be called but the same word — that is how `search.Search` produced
  the tool name `search_search`. Methods returning the current set of something
  are all called `List`. Enforced by `TestNoMethodRepeatsItsService`

## Naming what a package exports

Services are named for domains and tools are `service_method`. The 1,300
exported functions underneath had no rule written down, so this is the one the
code already mostly follows, read off it rather than invented.

**The package is the first word.** `markets.Prices()`, not
`markets.GetMarketPrices()`. 187 exported names are a single word for this
reason — the call site is where the name is read, and the import path already
said the noun. Never repeat the package: `x402.Enabled()`, not
`x402.X402Enabled()`.

**A question is an adjective, a participle, or a noun phrase describing its
argument. Never a bare verb.** The family is large and consistent —
`ai.Configured()`, `quota.Metered()`, `sms.OptedOut()`, `mail.Reachable()`,
`tasks.Running()`, `setup.Needed()` — and the noun-phrase form classifies what
was passed in: `service.DestructiveTool(name)`, `api.ToolDispatch(path)`.
`Is`/`Has`/`Can` are fine and used about twenty times; they are not required,
because an adjective already reads as a question.

What breaks it is a verb, because a verb is an instruction and the caller is
asking rather than telling. `usage.Skip(path)` looked like a command to skip
something and was a question about whether a path is noise; `twilio.VerifyInbound()`
looked like it verified something and reported a setting. They are
`usage.Skipped` and `twilio.VerifiesInbound` now — third person is the other
way to say a question, as in `app.SendsJSON(r)` and `app.WantsJSON(r)`.

**An action is a verb**, and says what it changes: `blog.CreatePost`,
`account.ChargeAppUse`, `mail.SendMessageTo`.

**An HTTP handler ends in `Handler`.** 108 of them do. A service's own page is
just `Handler`; anything more specific says which page — `blog.PostHandler`,
`account.TokenHandler`.

**Drop `Get` where the bare noun is free.** `mail.ConfiguredDomain()`,
`auth.OnlineCount()`, `quota.OperationCost(op)`, `flag.Count()`, `data.ByID(id)`.
Fifty-one names lost the prefix that way.

It stays on sixteen, and the reason is a rule rather than an exception:
**`GetX` is right when `X` is the type.** `auth.Account` is a struct, so
`auth.Account(id)` would not compile and, if it did, would read like a
conversion. Same for `news.GetFeed`, `apps.GetApp`, `blog.GetPost`. There the
prefix is the thing distinguishing a lookup from the type it returns, and
inventing `AccountByID` to avoid three letters is a worse name, not a better
one.

`Get` alone — `settings.Get("KEY")`, `blob.Get(id)`, `(*Store).Get` — was never
the same thing and is idiomatic Go. Fifteen of those.

One thing this does not settle: **a name may also stutter at the end** —
`quota.CheckQuota`, `agent.CreateAgent`, `apps.SearchApps`, `weather.FetchWeather`.
Nineteen do. The rule is the same rule and the fix is the same fix; the reason
it is written down rather than done is that some of them are not stutters at all
(`social.TruthSocial` is a proper noun, `wallet.DeleteBaseWallet` names its type)
and each needs looking at.

And **a method on a type may repeat nothing and say everything** —
`(*Verified).Settle()` reads off its receiver, and the rules above are about
package-level names.

`TestExportedNamesDoNotStutter` holds the first rule.

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
