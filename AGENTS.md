# Mu

**A personal server: one Go binary you can self-host that carries a web app, an
HTTP API, a CLI, an MCP server and an installable PWA over the same 37 services
and 147 tools.** `go build ./...` produces it; nothing else has to be running.

It is not a framework or a set of libraries. It is a thing that runs, that you
sign into, and that other programs can call.

## What is in the binary

The runtime service contract is `/api/v1/<service>/<method>` and `/mcp`.
Both hosts expose the same service tools; x402 adds payment handling. Tokens
restrict permissions, never select a different catalogue.

Product clients use `/agent`, `/agent/<name>`, `/inbox` and `/work` with content
negotiation, not separate API routes. `/developers` documents those requests;
`/api` and `/tools` document the runtime service contract.

| Door | Where | For |
|---|---|---|
| Web app | `/` | a person, signed in |
| PWA | `manifest.webmanifest`, `mu.js` | the same app, installed on a phone |
| API | `/api/v1/<service>/<method>` | a program somebody wrote |
| MCP | `/mcp` | somebody else's agent, holding a token |
| CLI | `mu <service> <method>`, `mu ask` | a terminal, a script, a cron job |

And it speaks protocols people already have clients for, served here rather than
proxied: SMTP and submission (`service/mail/smtp.go`, `submission.go`), IMAP
(`service/mail/imap.go`), XMPP (`service/chat/xmpp.go`), SSH
(`service/shell/ssh.go`), HTTP. An address is the smallest interface there is —
no SDK, no OAuth, nothing to adopt — so a person, another agent, a form or a
cron job can all reach the same account.

## The three things the app is

**Services** are the building blocks: 37 of them, one directory each, each with
an API surface and a set of tools derived from the same Spec. A dedicated
page is optional: services are capabilities, not a requirement for more screens. Some run a
protocol here — mail, chat, files, shell. Others hold an account with an upstream
provider so the caller does not have to: news, markets, video, weather, places,
maps, web search. Both are legitimate and the test is the same — whether the
caller is spared a signup, not whether we wrote the backend.

**Agents** are defined one way: a name, a prompt, and a scoped set of those
tools. Talk to one interactively on the web, by mail, over XMPP or from the CLI;
or POST JSON to `/agent/<name>` with a product token, where Mu runs the
agent and manages its tools. Built-in agents also run without anybody present: the
digest, the brief, moderation and work.

**Inbox** owns the local view of incoming correspondence and agent conversations.
Mail, chat and SMS retain their own service records. Inbox projects committed
service events into `internal/thread`, caching content, previews and source
references alongside local read state. Recording arrivals does not depend on an
agent deciding to answer. Agent conversations are written directly by Agent.

**Home** is the signed-in personal overview: assistant prompt, brief, inbox and relevant context.

## Where this goes

Direction, not description — none of this paragraph is a claim about today.

The services become the building blocks for infrastructure, tools and external
services. MCP is how agents reach them. The agents are what turn reach into
intelligence: summarising, contextualising and acting on what is there, rather
than fetching it again each time somebody asks. The conversation becomes the focal point, with the agent using services and showing their results in place. Work owns delegated goals, execution and outcomes;
Inbox owns the messages and updates about them. A conversation alone is not
a work item. `/work` retains delegated task details; the assistant starts work
and delivers outcomes back to the conversation.
It composes those records without renaming the tasks or apps services.

**Removing the barrier is the product.** An agent wanting news, mail, search,
weather, markets, places and somewhere to keep records otherwise needs six
providers: six signups, six cards, six tokens to rotate. Mu is one balance and
one protocol. An earlier line said *real tools, not wrappers*, which made "did
we build it" the measure and capped breadth at what one team can operate.
Breadth behind one account is the value.

**Mu is the runtime; Micro is the assistant it hosts.** The consumer app uses
Micro consistently in its header, page titles and PWA. Mu names the runtime in
code and technical documentation. The five primary
product destinations are Home, Inbox, Agents, Work and Services. They use bottom
navigation on phones, a narrow rail on tablets and a left navigation on desktop.
Account and Admin stay in the account menu. Keep the styling sparse.

Home owns the authenticated overview, prompt, personal collections and pinned
service cards. Its short brief uses local facts; it does not repeat the full
scheduled daily brief. Agent owns the conversation UI and explicit new/resume
controls. Services are directly usable by people as well as callable by agents;
the directory is not admin-only. Each service retains its permissions. Apps are
small tools that can be opened or embedded, not additional primary destinations.
Public landing and informational pages belong to internal/server; `/` redirects
signed-in visitors to `/home`. Preserve protocols, API responses, authorisation,
mutations and shared links.

**Extend through stable patterns.** Compose existing service interfaces and
card renderers. Refresh external context in bounded background work and render
cached views instantly. Personal previews stay account-scoped. Do not restart
automatic model content generation as a side effect of restoring Home.

## What is true today, and what is not

The line above this file used to open with was "a network for humans, agents and
services", and one word in it is still ahead of the code. Kept here because a
promise that can be false is worth more than a category that cannot.

*Humans and agents* is true: an agent account is a user here like any other,
holds an address, and can be written to. *Services* is true: the registry is
what both reach through. *Network* is true of the code and not yet proved on the
wire — S2S with dialback is built in both directions (`service/chat/xmpp_s2s.go`)
and has never completed a handshake with somebody else's server. Mail federated
all along, because SMTP is SMTP and `service/mail` looks up MX records like
anything else; chat was the protocol that did not. The claim stands when a
message from here lands on a Prosody account and one comes back.

Background work now has a dedicated public surface.
`service/tasks`, top-level `work` and durable `tasks.started` events run work nobody is
present for; `/work` and the public Work operations expose its state and outcome. Outbound is the same gap from the other
side — mail leaving, an x402 payment to another server — and `X402_SERVERS` is
read by a client no tool exposes. Inbound has three good rungs; outbound has
none.

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
detail.** Product JSON and HTML share the owning resource URL; the agent also
answers through the web app, CLI, mail, XMPP and `POST /agent/<name>`. Listing carriers beside layers is what makes the model
read as inconsistent when it is a gradient with an uneven fan-out. Which
protocol carries you is the caller's business, which is the only way "an address
is the smallest interface" means anything.

Product resource handlers serve both people and programs. Service HTTP and
MCP expose the service registry with its existing scope checks.

## What may travel in a URL

**A URL may carry what a thing is called. Never what a person said.**

| Where | What belongs there | Examples |
|---|---|---|
| **path** | public nouns | `/chat/finance`, `/@asim`, `/news/…` |
| **query** | view state from a vocabulary we already published | `?page=2`, `?tab=sent`, `?view=grid`, `?id=` |
| **body** | anything typed, and every credential | search terms, agent prompts, tokens |

The query string is *inside* TLS — a middlebox sees the SNI hostname and nothing
else — so this is not about the wire. It is about the four places a URL comes to
rest, two of which we have closed and two of which we cannot:

- our own access log records `r.URL.Path`, not `RequestURI` (`internal/server/serve.go`), so a query never reaches it;
- every page carries `<meta name="referrer" content="no-referrer">`, so nothing leaves in a `Referer`;
- browser history syncs to an account, and pasted links go wherever links go;
- **whatever terminates TLS in front of us logs the full URI.** Caddy and nginx
  both do by default. This is a self-hosted product, so the normal install has
  a reverse proxy in front of it, and every inbox search would land in plaintext
  in `/var/log` on a box whose whole premise is that the data is the owner's. We
  cannot fix that from in here. We can decline to put the words there.

So a search over anything private is `POST /<thing>/search`, not `GET ?q=`. A
search over something public — news, images, the docs — may stay a GET, because
the query discloses nothing the reader did not bring, and a searchable public
page that cannot be linked to is worse.

### A name in the path is not a hole; an id that talks is

Guessable URLs are a feature: `/chat/finance` is how somebody tells somebody
else where to go. The mistake is never that the name is in the URL — it is
letting the URL *be* the authorisation. `service/chat/private.go` is the shape:
the id is public, and `Member` decides.

The separate problem is an id that is itself a disclosure. `dm_asim_henrik`
names two people and says they talk, and no amount of POST helps, because that
id is in the page, in an `href`, and in whatever gets pasted into a bug report.
An id may be derived (cheap, no lookup, both sides compute the same one) or it
may be opaque (says nothing, needs a stored mapping). It cannot be both, and for
anything private, opaque is the one that holds.

`TestNothingPrivateTravelsInAURL` holds this line. Its allowlist is the record
of every surface we decided is public enough to keep a GET.

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
questions to ask — it consumes service tools. It declares no service Spec; the public API may
expose agent operations without turning the agent into a service.

- Every `service.Spec` lives under `service/`. One exception would be one too
  many: the moment a Spec lives elsewhere, "what is a service" stops being
  checkable and starts being something you have to remember.
- Nothing that consumes tools declares a Spec — not `agent/`, not `account/`,
  not `home/` or `admin/`.
- Internal service tools are derived from Specs. Public outcome operations are
  assembled in internal/server from agent, work and inbox, and adapted
  through one HTTP/MCP dispatcher in internal/api. Keep those registries separate.
- Public capability scopes (`api:agent`, `api:work`, `api:inbox`) never grant raw
  service access. Service scopes never grant broader agent execution. Account
  identity comes from the credential, never request arguments.
- One directory per service, named for the service. `internal/service` is the
  runtime that hosts them, not a service.
- Every service is discoverable through the generated Services API/SDK reference.
  Go handlers own their HTML pages and use the shared `internal/app` shell.
  Existing service URLs remain available without requiring primary navigation.
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

### Conversations and service-owned records

Agent owns persistent conversations; `internal/thread` supplies their storage.
It also holds Inbox's derived correspondence view. Mail, chat and SMS own their
original messages and expose them through service interfaces. Cached copies
carry source references and are updated or invalidated by the Inbox consumer.
The cache is not an alternative source of truth for those services.

Pages read this local view. Never synchronously fetch source records, external
services or models when rendering Inbox or switching conversations. Build and
refresh views in the background; retain usable cached content while doing so.
The same instant-load requirement applies to Home and other product pages.
Start the HTTP listener before restoring stores and registering services. During
restoration, return a fast, retryable loading response; never run mutations on
partially loaded stores or show missing history as an empty account. Publish
readiness only after storage and hooks are initialized. Secondary projections
and reindexing continue in the background after that. Shutdown during loading
must not flush uninitialized stores over saved data.


`internal/event.Commit` persists a service mutation and its factual event in
one recoverable commit. Named consumers checkpoint only after saving their
results. Delivery is at least once; consumers must deduplicate by event or
source identity. Mail, SMS/WhatsApp and chat response consumers live under
`agent/`, read source messages through `service.Call`, and reserve durable
execution receipts before starting Agent. Trust is captured by authenticated
transport entry points and checked again by the consumer. Importing a record or
posting through a tool must not recursively start an agent. Work uses the same
reserve-before-execution rule; neither path blindly repeats model/tool actions
after a crash. Inbox's historical reconciliation runs independently of its live
consumer; source reads and projection writes are serialized to prevent stale
backfill from resurrecting a deleted message. External notification attempts are reserved before sending;
a crash during delivery may require manual review rather than automatic replay.
`Publish` remains the transient broker for existing integrations and hints;
not all older publications have migrated to the durable outbox.

Events carry resource identity, version and necessary arrival facts, rather
than becoming a second store of private message bodies. Background consumers
resolve contents via the service API. Home commands still call Agent directly.
`service/recall` provides optional search over conversations; removing recall
does not stop the Agent or Inbox from recording and displaying messages.
Stored conversation history is retained until explicitly deleted; bounded
model context must not silently evict it.

### A thread is not a workflow

`agent.Flow` is a **workflow** record: the steps a task took, the tools it
called, whether it failed. That is *how an answer was produced*.
`internal/thread` is *what was said*.

They were one struct for a while, which is how a workflow record came to stand
in as conversation history — and they have different lifetimes. A workflow
record is debugging and may expire; a message is memory and should not. One
eviction limit governed both, which is why it was wrong for each.

## Layering

The fixed product packages are `account/`, `admin/`, `home/`, `agent/`,
`inbox/`, `work/` and `service/`. They evolve deliberately and slowly; do not
add top-level packages or navigation for implementation infrastructure.
`main.go` is the front door; `internal/server` assembles the product.
Internal event delivery and persistence belong under `internal/`.

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

That test reads import paths, and `internal/ai` is not one of them — it is
substrate, and a service is entitled to know about the runtime. So the
forbidden act can be performed through a permitted import, and the rule passes.
**The line is whether the model is producing the answer the caller asked for,
or deciding what the answer should be.** Generating an image is the first: the
caller asked for an image, and the model is the implementation the way an HTTP
call to a provider would be. Composing a message in a chat room is the second —
nobody asked for those words, so the service decided both that there should be
a message and what it says. The first is a service using a tool. The second is
a service being an agent, and belongs in one: publish the fact on
`internal/event` and let an agent subscribe, as `service/mail` and `agent/mail`
do.

Counted rather than forbidden, in `test/service_models_test.go`, for the reason
`test/service_hooks_test.go` gives about hooks: a test that failed on all of
them would be deleted within a week, and the tool-shaped ones would be "fixed"
into something worse. The ledger records which are which so the judgement is
not re-derived from each call site, and pins the count so it cannot go up
quietly. Two are debt today — `service/blog` and `service/chat` — tracked in
#1469, and chat in #89 where moving it out also unblocks serving XMPP.

Services publish facts; subscribers own response policy. Mail and chat do not
invoke the agent. Tasks publish `tasks.started` and schedules publish
`events.due`; top-level `work` reads their current state through service APIs,
executes the requested instruction and delivers the outcome. Schedule invites
and device notifications have their own subscriber in the server composition.

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

## UI composition

The signed-in front door is Home. Its prompt calls Agent directly, while
service pages and small apps remain independently usable. Share navigation,
renderers and controls across the product; keep one stylesheet and one browser
script. Preserve backend APIs, protocols, account isolation, CSRF checks,
payments and stored conversations. Requests and credentials belong in POST
bodies, never URLs. Only opaque conversation IDs may be used to resume a session.
Public pages retain the shared website footer. Signed-in app pages omit it.
Keep mobile content near the top: a title, a compact search row and the content;
group secondary actions separately and collapse infrequent filters.

Personal daily brief schedules remain available. No other automatic model
content generation: no daily images, public digests, summaries, tagging, topic
suggestions, greetings or arrival triage. User-requested jobs remain explicit.

Remove obsolete page renderers when separating them from backend handlers.
The command shell has one stylesheet; service results share one renderer.
Backend tests and the browser layout gate must pass before merge. Inspect real
desktop and mobile screenshots of populated/empty command results, login,
signup and footer destinations alongside browser assertions.


### Shared service layout contract

Keep the product and its code Spartan: one clear home for each capability,
shared renderers and layout primitives, and no parallel implementations of the
same flow. Services has its own primary navigation entry; do not duplicate it
in Admin. Service destinations such as Apps belong in that directory. Prefer removing
obsolete code to adding another abstraction. Preserve authorisation and existing
URLs when consolidating implementations.

Service pages use `/mu.css` and `/mu.js` through the shared shell. Add shared
component rules there; do not introduce a second stylesheet, executable inline
scripts, or page-local spacing fixes. Bind behaviour with data attributes; use
inert JSON for server-provided state.

Use `collection-list` / `collection-item` for linked records, `record-card` for
vertical content, `metadata-row` for secondary labels and timestamps, and
`form-actions` for groups of controls. A `reading-row` is a horizontal media row,
not a wrapper for an article's title, metadata and paragraphs. Wrap separate
metadata values in elements so layout gaps can separate them. Use `search-bar`
and labelled fields for forms; never depend on whitespace between inline links.

The shared app canvas fills the space beside navigation on every page. Do not
add route-specific width fixes or constrain whole app pages to reading width.
Services, agents and app collections use the shared responsive card grid;
messages and tables use rows. Sections share borders, padding and spacing.
Keep only prose and ordinary forms narrow; editors and maps use the canvas.
The Micro brand sits above desktop sidebar navigation and returns to the
header on smaller screens. All grid/flex children must shrink and long content must wrap.
Mobile forms wrap, tables scroll or stack, previews fit their container, and
conversation composers remain reachable. Bound images and give SVG shapes
explicit fills and strokes. Keep optional references in a collapsed disclosure.

When changing a shared primitive, inspect its populated and empty uses across
services at narrow and wide widths, including long titles and action groups.
Record any unavailable browser verification explicitly; a successful build does
not establish that a page renders correctly.
