# What Mu is, who arrives, and what they do

One page, so a change can be checked against it instead of relitigated. If
something in the product contradicts this, one of the two is wrong and it is
worth saying which before building.

Written after a day in which the sidebar was rearranged five times. Each round
was individually reasonable. Together it was churn, because there was no written
answer to "what is this and who arrives how".

## The product in one sentence

**Mu is tools for agents: the everyday internet — news, mail, calendar, search,
weather, markets, places, storage — as tools an agent can call over MCP, paid per
request, with no account in the way.**

The claim that matters is *one account instead of a hundred*.

An agent that wants news, mail, search, weather, markets, places and somewhere
to keep records needs six or seven providers to get there: six signups, six
cards on file, six tokens to rotate, six sets of terms. Mu is one balance and
one protocol, and for an agent paying per request over x402, no signup at all.
Removing that barrier is the product.

Sometimes removing it means running the thing ourselves. The SMTP server with
DKIM is real, and it has to be, because a sending domain with deliverability is
not something a caller can casually acquire — building it *is* the barrier
coming down. Sometimes it means paying a provider so the caller never holds
that relationship. Both count. The test is whether the caller is spared an
account, not whether we wrote the backend.

**On the line this replaces.** For a long time this said *real tools, not
wrappers*, and treated "we built the backend" as the thing that made Mu worth
using. That was wrong, and it was expensive: it made the wrong quality the
measure, and it caps the catalogue at whatever one team can operate. Breadth
concentrated behind a single account is not the cheap part of this product. It
is the valuable part. Depth is what you reach for when depth is what removes
the barrier.

## The three levels

This is the model the whole interface should teach, and the sidebar is where it
is taught:

| Level | What it is | Where |
|---|---|---|
| **Agents** | What acts for you. Named, scoped, each with its own token. | `/agents` |
| **Tools** | What an agent can call. Derived from services, never hand-written. | `/tools` |
| **Services** | The building blocks. One domain each; the things Mu actually runs. | `/services` |

An agent acts for you, tools are what it can call, services are what backs all
of it. Tools are derived from services. Only the bottom level lives in
`service/`.

Context was a fourth row for a while — a `/context` page holding what an agent
remembers and what it is watching right now. It is gone, and the reason is
worth keeping: it was argued against at the time on the grounds that context is
a property most services have rather than a kind of thing, which makes the page
a drawer for anything account-shaped. That is what it became. It grew a second
copy of the home screen, a card picker to justify the second copy, and a
checkbox asking the reader to opt in to the product working properly.

The half that was real is memory, and memory is a service. `memory_*` is how an
agent reads and writes it; the Memory card on `/account` is how a person does.
What an agent should know about right now is simply sent with the question —
see **Cards** below.

A feature that is interesting but not done well is worth removing rather than
keeping half-built. Three levels, each of which earns its row.

Two consequences worth holding:

- **An agent is not a service.** It consumes them. `agent/` sits beside
  `service/` in the repo and always has.
- **A service is a domain noun**, never an action and never a substrate. The
  substrate is how other things work: `internal/userdb` is the store that
  `tasks`, `files`, `contacts`, `images` and the agent roster all sit on, and it
  is not a service and never will be.

  But *the caller's own records* are a domain, the same way their files are.
  That distinction was got wrong once, in this document, which said storage was
  not a domain and therefore `db` could not be a service. Two things followed.
  Token scoping arrived afterwards and works per service, so a tool with no
  service behind it could be granted to nobody — `db_*` was unreachable by every
  scoped agent and had no box to tick on `/agents`. And the catalogue is derived
  from Specs, so the storage the docs told agents to rely on appeared nowhere a
  person could see it.

  The rule survives; the reading of it was too literal. `internal/userdb` is the
  substrate and stays internal. `service/db` is "your database" — a noun you can
  look at, grant, and scope.

### What earns a place in the sidebar

Only the levels above, plus Home, plus the account group. Everything else is a
service and lives in the catalogue — including Apps, which sat in the spine for
a while on the argument that it is half the product, and which has a Spec and a
catalogue tile like the other nineteen. Anyone who lives in a service pins it.

A sidebar that named four of nineteen services implied the other fifteen did not
exist. `/services` lists them all, and Home answers the at-a-glance question —
see below for what that means, which is not a wall of somebody else's content.

### What earns a place as a service

**Offer what the caller can't bring.**

Every agent that connects arrives already holding a text model — that is what
makes it an agent. Selling it text completion is selling someone their own coat.
It does not arrive with a calendar, an inbox, a search index, a places database
or an image generator, and those are worth running.

The test also settles the reverse: a service that merely wraps something the
caller already has, or could trivially get, is not worth the surface.

## Who arrives, and what happens next

The funnel, with the state of each step marked. **This doubles as the gap
list** — a step marked partial or missing is work, not aspiration.

| # | Step | State |
|---|---|---|
| 1 | Lands on `/` and understands it is tools for agents | ✅ |
| 2 | Sees what tools exist, and what each costs | ✅ `/tools`, priced per call |
| 3 | Creates an account | ✅ password, passkey, or Google |
| 4 | Connects their own agent — config file, or OAuth for Claude Desktop | ✅ both paths on `/tools` |
| 5 | Gets a credential scoped to what that agent should reach | ✅ `/agents` |
| 6 | Tests a call and sees it work | ⚠️ playground at `/mcp`; `/runs` now shows what ran, but nothing pushes "your agent called us" |
| 7 | Tops up so calls keep working | ✅ wallet, with a banner before it runs out |
| 8 | Discovers Mu can run agents too, and tries one | ⚠️ possible, but nothing on the connect path mentions it |
| 9 | Comes back because something happened while they were away | ✅ `/runs`, and Recent on Home |

Step 9 is closed. Every question already produced a Flow — the prompt, the
agent, each tool it ran, the answer, the error — persisted per account, and
`ListFlows` was served by nothing: the record of every run was written and never
shown. It is `/runs` now, with the last few on Home.

6 and 8 remain, and share the old shape: **the product does not tell you when
something worked.** A first call that succeeds is the moment somebody decides
this is real, and it still passes without a word.

## What Home is

The hardest question in the product, and the one the narrative kept failing:
the landing says *tools for agents*, you sign in, and you get a magazine.

**Home is your console: what your agents did, what is waiting, what it cost.**

Not the world's content. Under a tools-for-agents product the signed-in person
is the *operator* — they created the agents, they pay for the calls, and they
come back to find out what happened while they were elsewhere. That is a
dashboard, and it is the other half of `/agents`: agents is what you delegated,
Home is what came back.

`/` and `/home` are different on purpose and that is not the problem. `/` is the
public pitch for somebody deciding whether to point an agent here; `/home` is the
console for somebody who already did. The problem was that the console showed
headlines.

### So what are the cards?

Two things at once, and the second was discovered later.

They are each **a service demonstrating itself** — on the landing and on
`/services`, a live card is evidence, the difference between claiming a news
aggregator and showing one running. On Home it was the world's content where
your own should be: every card somebody else's day.

And they are **context for the model**. Each card is a materialised view over a
service, refreshed on a timer; rendered as text they are the payload an agent
wants before it starts guessing which tool to call. This document used to say
"not context for the model — nothing on the screen is fed to it", and that was
true when it was written and is not now.

So the rule is one sentence: **cards prove the tools are real, they live on
Home, and they go with the question.**

A card fed to a model and a card shown to a person must be the same card, and
the only way to guarantee that is for there to be one list. There is: the
instance's, in `cards.json`. No per-account selection, no picker, no toggle —
each of those was a way for the screen and the payload to disagree, and each of
them did.

What Home should carry, in order:

1. **In flight** — tasks running, agents working now. ⚠️ counts only.
2. **Waiting** — unread mail, events today, results you have not read. ⚠️ counts only.
3. **Recent** — what your agents actually did, and whether it worked. ✅ `/runs`,
   last five inline.
4. **Cost** — what it is running you, with the way to top up. ✅
5. **The agent input**, because asking it something is a thing you do from here. ✅
6. **The cards**, last, because they are the evidence rather than the point.

Ordering them this way is what answers the old objection — Home showing the
world's news under a tools-for-agents banner. The answer was never to move the
cards off Home; it was to put what is yours above them.

`/usage` is the ledger — calls and spend over time. Home is the front page of
your instance, not its accounts.

## Memory

Durable notes about the caller, kept across conversations. `memory_set`,
`memory_list` and `memory_delete` for an agent; the Memory card on `/account`
for a person, with a Forget button on every row.

It was invisible from both sides. Facts were extracted from conversations by a
background model call and injected into every prompt, and there was no way for
a person to see one or an agent to write one. An agent that cannot say
"remember this" gets told the same thing every session, which is the exact
failure persistent memory exists to prevent.

The rule that took longest to see: **a primitive an agent cannot call is a
dashboard.** Memory is a service for that reason — headless, so it gets tools
and scoping without claiming a domain it does not run.

## The two doors

One set of services, two ways in, and neither is a second-class citizen:

- An **agent** calls `/mcp`.
- A **person** signs in and gets the app.

Nothing is built twice. A new service appears in both at once because both are
derived from the same `service.Spec`. **Keep the signed-in app intact** — it is
not legacy, it is the proof the tools are real. A calendar you can open is
evidence that `events_free` is not a mock.

## What an agent is, exactly

A name, a scope, and optionally a token.

- **Name** — so it can be talked about.
- **Scope** — the services it may reach. Enforced at the MCP boundary against
  its token, not just displayed. Empty scope means "everything you can", which
  is a choice rather than a default.
- **Token** — only needed to call in from outside. An agent you only talk to
  here has nothing to authenticate, so it has no credential until you ask for
  one. Removing an agent revokes its token.

The same record whether you made it in the chat or on `/agents`, and the same
confinement whether you talk to it here or point Claude Desktop at it.

## Rules that have already been paid for

Each of these was learned by getting it wrong:

- **Ask for permission where it is earned, not at the door.** Nobody meets a
  calendar consent screen at signup; the ask appears when an answer was thinner
  for the lack of it. The acceptance rate is then a real signal.
- **Audit permissions in one place.** `/account` lists every grant. A permission
  you can only find on the page that happens to use it is one nobody reviews.
- **The console is public.** Anything written to `/stream` is published — no
  titles, no subjects, no senders, no account ids.
- **Scoped means enforced.** If a scope is not checked at the boundary, it is a
  label, and a label that looks like a control is worse than no control.
- **Never ship a control that does nothing.** A "runs here" option that stored a
  field nothing read, an address that receives no mail, a badge on one item and
  not its neighbour — each was a promise the product could not keep.
- **Credits price real cost** — a model call or a paid third party. Operations
  that only touch this instance's storage are free. Abuse control is rate
  limiting, not pricing.

## What this is not

- Not a personal home server with a service per sidebar row. That is what it was
  growing into, and the move away from it is the point of the three levels.
- Not a directory of APIs. m3o accumulated a hundred and none of them composed.
  The test for a new service is whether it lets the agent finish a job that
  needs *two* — "move my Thursday and tell the people in it" needs calendar,
  contacts and mail, and that sentence is the difference between a platform and
  a list.
- Not a place where implementation vocabulary reaches the user. No *go-micro*,
  no *microservice*, no *framework* in the UI, in what the agent says about
  itself, or in marketing copy. *Service* is ordinary English and is fine.
