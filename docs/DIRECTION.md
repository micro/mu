# Direction

Where the product is going, how the code is shaped to follow it, and what is
currently in the way. `PRODUCT.md` says what Mu is today; `ARCHITECTURE.md` says
how it is built. This is the one that says why, and what changes next.

## 1. The thesis, and the fact that it moves

Mu started as a personal home server: your mail, your news, your notes, your
machine. It is now **tools for agents**: the everyday internet as things an
agent can call, one account instead of a hundred, paid per request. It may well
become **agents** — you come for a thing that does the work, and the tools are
what it is standing on.

Those are not three products and not three rewrites. They are the same asset
with a different consumer in front of it:

| Thesis | The asset | Who consumes it |
|---|---|---|
| Personal home server | a catalogue of capabilities behind one account | you, through a web page |
| Tools for agents | the same catalogue | your agent, through MCP |
| Agents | the same catalogue | our agent, on your behalf, and you consume the agent |

The catalogue is the invariant. Every pivot so far has added a consumer and left
the services alone, which is exactly why neither pivot needed a rewrite — the
home server's news reader and the agent's `news_list` are the same service with
two doors on it.

**So the architectural bet is: keep the catalogue stable, let the consumer be a
layer.** It is already the shape of the tree — services answer, tools are derived
from services, agents use tools — and the value of naming it is that it gives a
cheap test for whatever comes next:

> Does this add a consumer, or does it change what a service is?

Adding a consumer is cheap and has been done three times. Changing what a
service is costs the catalogue, and the catalogue is the product.

### What agents does to the thesis, and the rule that protects it

"Tools for agents" is a neutral position. Anyone's agent can call; we are
infrastructure and we are not competing with the thing calling us. Shipping our
own agents changes that — we become a competitor to some of our own callers.

That is survivable, and it is survivable on one condition, which should be a
rule rather than an intention:

> **Our agents get no private door.** Everything `agent/` can do, an MCP client
> holding a token can do. If the assistant can reach a capability that is not in
> the catalogue, the thesis has quietly changed and nobody decided to change it.

This is testable, and it is close to true today — the agent runs tools through
`api.RunPlannedAs`, which is `api.ExecuteTool` with two refusals in front of it,
and the refusals only ever *remove* things. The place it is not true is
`agent/blog`, which calls `service.CallDynamic` directly. That is fine for now
and worth watching.

## 2. The top level is the product

The rule, stated plainly: **a top-level directory is something a user can name
and click.** The sidebar is Home, Tools, Agents, Services, with Account below and
Admin below that for operators. The tree is:

```
home/      → /home       the first screen: ask, and the live cards
tool/      → /tools      the catalogue, agent lens
agent/     → /agents     what acts for you
service/   → /services   the catalogue, person lens
account/   → /account    who you are and what you can afford
admin/     → /admin      operator surface, one user
client/    → —           Discord, Telegram, WhatsApp
internal/  → —           everything with no name a user would recognise
```

Five nav items, five directories, in the same order. That is not a coincidence
worth admiring, it is the constraint working — and it is why `tool/` is 280
lines sitting beside packages twenty times its size, and why `internal/gtfs` is
1,590 lines that nobody will ever click.

Two entries need saying out loud so the rule stays checkable rather than
remembered:

- **`admin/` is a nav item** — for one user. Conditional, not absent.
- **`client/` is the rule inverted.** A Discord bot is a nav item in Discord.
  The door is somewhere else, which is the whole point of it.

Everything else at the top level (`docs/`, `examples/`, `scripts/`, `test/`) is
not product and does not ship in the binary's surface.

### The gap this exposes

`agent/` is a top-level directory and `/agents` is a nav item, and the page
"lists and revokes; it does not create" — its own words. You click Agents and
get a table of tokens. Home has the conversation; Agents has the inventory.

That is the clearest signal in the repo about where the product goes next, and
it was found by reading the directory names against the nav, which is the method
working. See §7.

## 3. How a surface actually reaches a capability

This is the open question — reach into the package, or go over a protocol? The
answer today is better than it looks, and the reason is worth knowing before
changing anything.

| Surface | Path to the capability |
|---|---|
| `/mcp` — external agents | `api.ExecuteTool` → `tool/derive.go` → `service.Call` (go-micro RPC) |
| `mu <service> <method>` — CLI | HTTP → `/mcp` → the above |
| `/a2a` — other agents | `agent/a2a` → `agent/micro` → `api.RunPlannedAs` → the above |
| Apps — `mu.service(name, method, args)` | `POST /apps/{slug}/sdk/service` → `service.CallDynamic` → RPC |
| `agent/blog` | `service.CallDynamic` directly |
| **Web pages** — `/news`, `/mail`, `/home` | **Go import, direct call, HTML string** |
| **`admin/`, `client/`** | **Go import** |

**There is already one spine.** `service.Call` over the in-process go-micro
registry is how a tool runs, how an app calls, how the CLI works and how another
agent's request is served. MCP is a *derived* door onto it. The app SDK is a
derived door onto it. Nobody built a second implementation of "read the news".

The exception is the server-rendered HTML. `home/` is the interesting case
because it is mostly *not* an exception: it draws its cards from
`service.Cards()`, read off the registry, so a service that ships tomorrow can
appear on the home screen with nobody editing `home/`. It reaches directly into
a service in exactly four places — headlines, unread count, one app lookup, the
support address.

So the honest summary is: **one spine, six doors, and the HTML is the one that
walks around the back.**

### Is that wrong?

Partly, and the trade is real in both directions.

In favour of the direct call: a page rendered from an in-process function call
has no round trip, no JSON encode/decode, no client-side framework, and no
loading state. Pages arrive complete. That is a feature, not laziness, and it is
most of why the thing feels fast.

Against it: a page and a tool are two readings of the same data, and they drift
in the small ways — one filters, one does not; one caps at 20, one at 50. And a
consumer that is not in this binary — a desktop app, a native mobile client,
somebody else's front end — can reuse none of it.

The question "if you built a desktop app, would everything go through an API?"
has a concrete answer here: **yes, and the API already exists** — it is
`{service, method, args}` with identity from the session and scoped services
refused, and it is currently addressed as though it belonged to apps.

## 4. Architecture: what is good, leave it alone

- **Spec-derived everything.** One `service.Spec` produces the nav entry, the
  route, the tool name, the card, the price, the permission and the docs. Adding
  a service is one declaration, and every surface picks it up. This is the best
  thing in the codebase and most of the leverage in it.
- **One spine, already.** Tools, apps, the CLI and A2A all reach capabilities
  the same way. It did not have to turn out like that.
- **The layering rules, now enforced at the right depth.** Services do not
  import each other, `internal/` does not import the product, and both tests
  walk subtrees rather than stopping at the first directory.
- **`internal/` is shallow.** The busiest package has 53 importers and the
  deepest imports six of its siblings. There is no tangle underneath.
- **Prices are data.** `quota.json` at the top of the repo, not Go.
- **Single binary, self-hostable.** This is the home-server ancestry, and it is
  an asset rather than baggage: "one account instead of a hundred" is only
  attractive if the one account is not a company you cannot leave.

## 5. Architecture: what to change, in order

**1. Promote the app SDK's service door to a first-class API.**
`POST /apps/{slug}/sdk/service` already takes `{service, method, args}`, binds
identity from the session, strips any `account_id` the caller supplies, and
refuses account-scoped services. That is the API — it is just addressed as if it
belonged to apps, and it is deliberately crippled (no scoped services) for a
reason that applies to sandboxed apps and not to a signed-in desktop client.

Lift it to a route of its own (`/api/v1/<service>/<method>`; note `/api` itself
is currently the API documentation page), derive it from Specs like every other
door, and let apps be one caller of it rather than its owner. This is mostly a
route move and a permission split, and it is what makes "everything works
against an API" true without touching a line of UI.

It also needs the same treatment `/mcp` has for payment: x402 sits at the door,
not on the spine, so a new door starts unpriced unless it is wired.

**2. Let page handlers use the spine, opportunistically.**
Not a UI rewrite — that is the expensive answer to a cheap question. A page
handler calling `service.Call` and rendering the result is the same speed
(in-process), the same code path as the tool, and one implementation instead of
two. Do it when touching a page anyway; do not schedule a migration.

**3. Fix rule 4 — a service must not import an agent.**
Four hooks (`tasks.RunAgent`, `events.RunAgent`, `events.OnFireEvent`,
`stream.AIReplyHook`) invert the one direction that has a reason rather than a
convention behind it. Do this *with* the agents UI in §7, because that work
revisits tasks, events and console replies anyway, and doing it separately means
touching three live background loops twice.

**4. Put argument types in the Spec.**
The Spec says what a method does, costs, needs and is called; it does not say
what it *takes*. So the CLI guesses argument types, fails, fetches `tools/list`
and re-types — `coerceToSchema` is a workaround for a hole in the declaration.
Every other consumer will hit the same hole. The handler signature already has
the types; the Spec should carry them.

**5. Do not add a UI framework yet.**
Reactive UI has a real cost — a build step, a template scaffold, a loading state
on every card — and the current pages are fast and complete. The thing that
makes a desktop app or a third-party front end possible is change 1, not a
rewrite. If a rewrite ever happens it should happen *because* an API-driven
client already exists and proved the API, not in order to have one.

## 6. Product: what is good

- **"One account instead of a hundred"** is the claim, and it survives the
  pivots. It was true for a home server and it is true for agents.
- **Two doors on one set of services.** Nothing is built twice, and the
  signed-in app is the proof the tools work rather than legacy to be tolerated.
- **Home as the demonstration.** The cards show what the instance knows, and the
  input above them answers using it. That is the pitch, running.
- **Paid per request, no signup.** The barrier is the product, and removing it
  is the thing being sold.

## 7. Product: what to change

**1. Agents becomes a surface, not an inventory.**
`/agents` lists tokens. It should list *your agents*, and clicking one should
open a conversation with that agent — its scope, its memory, its history, its
runs. Everything needed is already built: per-account agents, the `agent/micro`
registry, scopes, tokens, and an orchestrator. What is missing is the
conversation view and per-agent history.

`mu agent` in the CLI is the right place to iterate first. A CLI can change
daily without a design, and what survives a week of using it is what deserves a
page. Ship the loop there, then surface it.

**2. Decide what "agents" means for the neutrality claim** — see the rule in §1.
This is a positioning decision, not an engineering one, and it should be made
deliberately rather than discovered later.

**3. Keep the two lenses honest.** Services and Tools are the same catalogue
seen by a person and by a model. As the catalogue grows, the person's lens needs
grouping and the model's needs pruning, and they will want to diverge. They
should not diverge in *content*.

## 8. What keeps this true

Rules that are not enforced become comments. These already exist:

- `test/services_test.go` — every Spec is under `service/`, every directory
  under `service/` is a service, nothing registers a tool by hand
- `test/layering_test.go` — the import rules, walking subtrees
- `test/destructive_test.go` — nothing under `agent/` reaches the raw tool door
  with a name it did not write

Worth adding as the above lands:

- **No private door for our own agents** — every capability `agent/` reaches is
  in the catalogue
- **Every nav item has a directory and every product directory is a nav item**,
  with `client/` named as the one deliberate exception. The mapping in §2 is the
  method for reading this codebase, and it is worth one test to keep it real.
