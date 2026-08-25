# Mu

**Work with Agents.** Make one, give it an address, hand it a job. It picks the
work up while you are elsewhere and answers on the conversation you asked in.
Behind it is the everyday internet — news, mail, search, weather, markets,
video, storage — as tools it can call, paid per request in USDC over x402, with
no account in the way.

That line is a promise rather than a category, which means it can be false. It
is true only while work is something you can hand over and collect:
`service/tasks` holds the work, `agent/work` runs it, the answer returns to the
thread it was asked on. If that stops being true the sentence has to change.
Underneath, structurally, this is a personal server with a messenger at the
front. When the emphasis moves, move the sentence after the first one.

**An address is the smallest interface there is** — no SDK, no OAuth, no
protocol to adopt, nothing on the other side — so a person, another agent, a
form or a cron job can all write to one. Every provider ships an MCP server;
none of them ship an agent that is permanently reachable and remembers. That is
what makes an agent something you have rather than something you visit.

The inbox is not email. It is `internal/thread` — every conversation this
account has had, on whichever client it arrived, in one record read at
`/inbox`. A new channel joins that record rather than starting a second.

**Removing the barrier is the product.** An agent wanting news, mail, search,
weather, markets, places and somewhere to keep records otherwise needs six
providers: six signups, six cards, six tokens to rotate. Mu is one balance and
one protocol. Sometimes that means running the thing ourselves, sometimes
paying a provider so the caller need not hold that relationship — both are
legitimate, and the test is whether the caller is spared an account, not whether
we wrote the backend. An earlier line said *real tools, not wrappers*, which
made "did we build it" the measure and capped breadth at what one team can
operate. Breadth behind one account is the value.

**Keep the signed-in app intact.** It is not legacy, it is the proof the tools
work: every capability had to render a page, which is why the services are
coherent enough for tools to be derived from them.

## The access model

Four layers. They are not different capabilities — it is one catalogue all the
way down. What changes is how much the consumer already knows, and therefore
how much we supply.

| We supply | They supply | Layer | Consumer |
|---|---|---|---|
| state | the exact call | **services** | a program somebody wrote |
| a described menu | the goal and the reasoning | **tools** | somebody else's agent |
| the reasoning too | the goal | **an agent** | a person, or anything that can send a message |
| the occasion too | a policy, once | **initiative** | nobody — it acts |

**A layer is what you reach. A carrier is how you get there, and it is a
detail.** Services have one carrier (`/api/v1/<service>/<method>`), tools have
one (`/mcp`), and the agent has several — the web app, `mu agent`, mail, XMPP,
`POST /agent/<name>`. Listing carriers beside layers is what makes the model
read as inconsistent when it is a gradient with an uneven fan-out. Which
protocol carries you is the caller's business, which is the only way "an address
is the smallest interface" means anything.

MCP is a real rung rather than the API in a different envelope: the same
services underneath, plus self-description, because a model has to *choose* and
choosing needs a menu.

The fourth rung is built and has no surface. `service/tasks`, `agent/work` and
`event.WorkForAgent` run work nobody is present for, and nothing renders it.
Outbound is the same gap seen from the other side — mail leaving, an x402
payment to another server — and `X402_SERVERS` is read by a client no tool
exposes. Inbound has three good rungs; outbound has none.

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
gofmt -l .              # formatting — CI gates on this and nothing above catches it
```

CI runs `gofmt -l .` and fails on any output. None of build, test or vet look at
formatting, so a change can pass everything run by hand and fail the first thing
CI does.

The main branch is `main`.

## What a service is

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
  not `home/` or `admin/`.
- Nothing registers a tool by hand. A tool with nowhere to come from is a
  service that has not been written yet.
- One directory per service, named for the service. `internal/service` is the
  runtime that hosts them, not a service.
- Every service has a page: service name == directory == route == nav label ==
  tool prefix. A capability with no page is one a person cannot find, and the
  last attempt at "is a service, but hide it" was a boolean whose deletion was
  the fix.
- A service is named for a **domain** (a noun), never an action. Tool names are
  `service_method`, so an action-named service leaves its main method nothing to
  be called but the same word — which is how `search.Search` produced
  `search_search`. Methods returning the current set of something are `List`.

Enforced by `TestEverySpecLivesUnderService`, `TestNothingThatUsesToolsIsAService`,
`TestEveryDirectoryUnderServiceIsAService`, `TestAServicePageIsAtItsOwnName`,
`TestNoMethodRepeatsItsService`.

**A balance is not a service.** How an account pays is the same shelf as
changing your email or rotating a token, so it lives in `account/` with no Spec
and no tools. What a service needs to know about money is `internal/quota`,
which holds prices and deliberately does not know what a balance is.

`service/wallet` *is* a service by the definition above — a key on Base that
answers questions about state. The ledger you top up with a card is the
account's. Those were one word writing one file and destroying each other's
data.

**The agent is not a service**, for the same reason stated backwards: it is the
thing that reads the catalogue. `agent_ask` and `agent_list` were tools; an MCP
client calling `agent_ask` already holds every tool this agent holds.

*(That reasoning is sound for a client that connects directly and weaker for one
that does not — another instance wanting an answer from your agent, with your
memory and your account, is not asking for your tools. Worth revisiting.)*

## Clients, and the record between them

Three ways in — the web page, the CLI, and mail — plus XMPP, and the agent is
the same one behind all of them. What differs is protocol and nothing else, so a
client's whole job is to translate: parse what arrived, hand it over, render
what comes back, name an agent where the address or the command already chose
one (`agent+news@`, a slash command).

**Everything else belongs to `agent.Ask`.** Finding the conversation a message
continues, handing the agent what was already said, running it, writing the turn
down, noticing anything worth remembering. Every client wrote that for itself
once and it drifted five ways — history in a map in memory, lost on restart;
runs unrecorded; memory extracted on the web alone, so an agent remembered what
you typed in a browser and forgot everything you said anywhere else.

**There is one agent loop, and no fallback behind it.** `runNative` in
`agent/native.go` is the whole of it: the model does native tool-calling over
the registered services, and a run that fails returns an error. It used to fall
through to a hand-rolled plan/execute/synthesize pipeline — one model asked for
a JSON array of tool calls, another asked to write prose around the results —
kept as a safety net after go-micro's agent replaced it.

A fallback that behaves differently from the thing it replaces does not make
failure softer, it makes it quieter. That one had its own tool catalogue
maintained by hand, its own system prompts, no conversation history and no
user-defined agent applied; when a run errored the person got an answer from a
different agent and no way to tell. It hurt most where nobody was watching — a
scheduled task whose model call timed out came back composed as the generalist,
from a list of tools that had drifted from the registry.

So: a new door calls `agent.Ask` or `agent.QueryWithOpts`, and when the agent
cannot answer it says so. Three copies of that pipeline existed at the end, one
per door, which is the other half of the argument. `test/destructive_test.go`
holds the safety property that moved with it — the model is handed a filtered
tool list *and* every dispatched call is checked again, both in `native.go`.

Three words that are not the same thing:

- **History** — the messages of one conversation. Per thread, persisted.
- **Memory** — durable facts about an account, across every client. `notes`.
- **Context** — what is assembled for one run out of both, plus live tool data.
  Assembled fresh each time, never stored.

### The system of record is not a service

`internal/thread` holds what was said: a message, on a thread, on a client, for
an account. It is written on every turn whether or not anybody asks.

**A service is something a caller may choose to use. A system of record is not a
choice.** The core of the product must not sit behind a decision an agent takes,
because an agent that forgot to call it would simply stop remembering. It keeps
the company of the others nobody chooses: `internal/quota`, `internal/x402`,
`internal/auth`.

Reading is a different question and a service over it is welcome, because
searching your own past *is* something an agent decides to do. That service is
`service/recall`, and the test for whether it is built in the right place:
**delete the service and nothing breaks.** Clients still record, the agent still
gets its history, the pages still render; you lose only the ability to go
looking on purpose.

One thing recall does own, and it is worth knowing why: deleting an account.
Nothing else in the catalogue knows the record exists — it is written by the
machinery, so no service claimed it and no deletion hook cleared it.

Messages, not events. An event log that accepts anything has no schema and
cannot be queried a year later, and there are already two event-shaped things:
`service/stream` is the public timeline, `internal/usage` the counters.

### A thread is not a workflow

`agent.Flow` is a **workflow** record: the steps a task took, the tools it
called, whether it failed. That is *how an answer was produced*.
`internal/thread` is *what was said*.

They were one struct for a while, which is how a workflow record came to stand
in as conversation history — and they have different lifetimes. A workflow
record is debugging and may expire; a message is memory and should not. One
eviction limit governed both, which is why it was wrong for each.

## Layering

The top level is the product — `home/`, `agent/`, `service/`, `admin/`,
`account/`. Each is a staple: it owns something nothing else owns, and a user
can name it. Underneath is `internal/`, which is everything with no name a user
would recognise.

**The product may import `internal/`. `internal/` may never import the
product.** The two exceptions are the programs: `internal/server` and
`internal/cli` assemble everything. Enforced by
`TestInternalNeverImportsTheProduct`.

The top level is the sidebar, and that is the test for whether something belongs
there — a user can name it and click it. It is why `tool/` is 280 lines sitting
beside packages twenty times its size: small is what a good derivation looks
like, not evidence it belongs in `internal/`. A nav item can be a *view* rather
than a directory; what decides is ownership. `tool/` owns the derivation from
`service.Spec` to `api.Tool`, so it is a directory. Usage owns nothing, so it is
not.

**Services never import each other.** A sideways import makes two services one
unit — read together, changed together, moved together — and the catalogue stops
being a list of independent things. Whatever they share goes in `internal/`,
never in a non-service directory under `service/`, because "one directory per
service" is only checkable while it is true. Enforced by
`TestServicesDoNotImportEachOther`, whose allowlist is empty and should stay
that way.

Both that test and `TestInternalNeverImportsTheProduct` once stopped at the
first directory level — a glob ending at the closing quote — so a package one
level deeper was invisible to the rule about it, for a year. Both walk the
subtree now. **A rule you can get out of by making a subdirectory is not a
rule.**

**An agent may import a service; a service may never import an agent.** This one
has a reason rather than a convention behind it: a service answers a question
about state, an agent decides which question to ask, and a service calling an
agent is asking the model what its own answer should be. Enforced by
`TestNoNewServiceCallsAnAgent`, which asserts zero.

Four things ask an agent for work: a chat message, an email arriving, a task
assigned, a schedule firing. Three of the four used to reach upward through a
function variable filled in at boot. They are one fact now —
`event.WorkForAgent`, published by whichever service holds the record,
subscribed by `agent/work`, which knows where the answer goes because a task
keeps its result and a standing instruction is mailed.

**A function variable is an import the compiler cannot see.** Every rule above
is checked by reading import statements, and every edge they forbid has been
made anyway, as an exported `func` variable a service declares and
`internal/server/hooks.go` fills in at boot. The import is gone; the dependency
is not; the test passes.

So the hooks are counted, classified and pinned by `test/service_hooks_test.go`.
Not forbidden — some are the cheapest honest answer today, and a test failing on
the whole list would be deleted within a week. What is asserted is that the
number does not go up quietly, and that the ledger matches the tree in both
directions. It did not: the doc said four hooks, there were nine, and one of the
four it named had already been deleted. Prefer a plain downward import; when you
cannot have one, add the hook and know it cost something.

**What a service may know is the runtime, not the product.** A service is a
building block and it also runs inside something, so it knows the platform:
`internal/data` to store, `internal/auth` for who is calling, `internal/quota`
for what an operation costs, `internal/event` to say something happened.

What it may not know is a *product requirement* — that agents exist, that there
is a roster, that an inbox has a switcher, that an app's author gets paid. Those
are decisions made above it, and a building block that encodes one stops being
reusable by anything that decides differently. Metering is the useful edge case:
what something costs is a fact a service declares (`Cost` on its Endpoint); who
may afford it is a judgement made at the door.

A service never imports `account/`. `account/` fills in the half quota cannot
answer, from its own `init`, because quota sits underneath it. Enforced by
`TestNoServiceImportsTheAccount`. This does not touch `service/wallet`, which
holds a key rather than a balance — and a sideways import from it would be
caught like any other.

## Naming what a package exports

**The package is the first word.** `markets.Prices()`, not
`markets.GetMarketPrices()`. The call site is where the name is read and the
import path already said the noun. Never repeat the package: `x402.Enabled()`,
not `x402.X402Enabled()`.

**A question is an adjective, a participle, or a noun phrase describing its
argument. Never a bare verb.** `ai.Configured()`, `quota.Metered()`,
`sms.OptedOut()`, `mail.Reachable()`, `setup.Needed()`; the noun-phrase form
classifies what was passed in — `service.DestructiveTool(name)`,
`api.ToolDispatch(path)`. `Is`/`Has`/`Can` are fine and not required, because an
adjective already reads as a question. What breaks it is a verb, because a verb
is an instruction and the caller is asking rather than telling: `usage.Skip(path)`
looked like a command and was a question, so it is `usage.Skipped`. Third person
is the other way to say a question — `app.SendsJSON(r)`, `app.WantsJSON(r)`.

**An action is a verb**, and says what it changes: `blog.CreatePost`,
`account.ChargeAppUse`, `mail.SendMessageTo`.

**An HTTP handler ends in `Handler`.** A service's own page is just `Handler`;
anything more specific says which page — `blog.PostHandler`.

**Drop `Get` where the bare noun is free.** `mail.ConfiguredDomain()`,
`auth.OnlineCount()`, `quota.OperationCost(op)`, `data.ByID(id)`.

It stays where **`X` is the type**: `auth.Account` is a struct, so
`auth.Account(id)` would not compile and would read like a conversion. Same for
`news.GetFeed`, `apps.GetApp`. Inventing `AccountByID` to avoid three letters is
a worse name. `Get` alone — `settings.Get("KEY")`, `blob.Get(id)` — is
idiomatic Go and was never the same thing.

A name may still stutter at the end — `quota.CheckQuota`, `agent.CreateAgent`.
Nineteen do, and the fix is the same fix; some are not stutters at all
(`social.TruthSocial` is a proper noun) and each needs looking at. A method on a
type may repeat nothing and say everything: `(*Verified).Settle()` reads off its
receiver, and these rules are about package-level names.

Enforced by `TestExportedNamesDoNotStutter` and `TestAPredicateIsNotAnInstruction`.

## The go-micro relationship

go-micro is the substrate Mu is built on, and it has three roles that must stay
separate.

**One-way dependency.** Mu depends on go-micro. go-micro must never depend on
Mu. It changes when *framework users* need something, never because Mu does. If
Mu needs a capability go-micro lacks, it goes in Mu's own code, or Mu vendors or
forks. A PR to `micro/go-micro` whose justification is "Mu needs this" is the
failure mode.

This is a policy, not an architecture. It is cheap to hold and it targets a
specific failure already lived through: a business built *depending on*
go-micro gave the framework two masters — a library wants stability and
generality, a runtime wants opinionated constraints — and the engineering budget
went on reconciling them instead of shipping either. Mu pins a released version;
forking is permitted and expected.

**Not in the product surface.** A user should never meet the words *go-micro*,
*microservice* or *framework* in the UI, in onboarding, in marketing copy, or in
anything the agent says about itself — including system prompts and the public
status page. If a user-visible string mentions them, that is a bug. The word
*service* is fine; it is ordinary English. What is banned is implementation
vocabulary, not the concept. The README and developer docs are exempt: they are
the funnel, and they are facts. Developer-facing, say it; customer-facing,
don't.

## Pricing

A credit is charged when an operation costs us something to run: a model call,
or a paid third party. Operations that only touch this instance's own storage
are free. `quota.json` at the top of the repo is the one price list — the gate
reads it and every cost table renders from it, and it carries the reasoning for
each price in the note beside it. `main.go` embeds it and calls `quota.Load`, so
the package that answers what something costs does not decide where the answer
comes from. A service says *which* operation it charges (`Cost:
quota.OpWebSearch`); what that costs is an operator's decision, so it is data.

Abuse control is `auth.CheckPostRate`, not the credit charge. Credits price real
cost; rate limits stop bots.

## Conventions

- No external dependencies for crypto — secp256k1, RLP and ECDSA are pure Go in
  `service/wallet/evm.go`
- Settings via `internal/settings/` — env vars first, stored values after
- Background loops are goroutines started in `Load()` or `main.go`
- **An embedded file lives with the package that owns it**, never in a shared
  `embed/` or `assets/` directory: `service/news/feeds.json`,
  `service/flights/airports.json`, `internal/app/html/*`, `quota.json` beside
  `main.go`. Same rule as one directory per service — a central pile separates
  an asset from the only code that reads it, and nothing then says which of the
  two to delete. The `//go:embed` sits in the file that uses the bytes
- `docs/` holds one file, `INSTALL.md`, served at `/install`. There were nine.
  The eight that went described what the code already says, in files nothing
  fails when they go stale — the architecture doc kept a hand-written table of
  the registry *with a test to stop it drifting*, which is the admission that it
  was a copy. What is registered is `/services` and `/tools`. What things cost
  is `quota.json`. The layering is `test/layering_test.go`, which fails when it
  is wrong rather than being read when it is not. Install survives because it
  holds what the code cannot: ports, DNS records, decisions about a machine this
  repository never sees
