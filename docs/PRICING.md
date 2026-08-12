# What things cost, who pays, and how we make money

One page, so the next pricing change can be checked against it instead of
relitigated. The companion to PRODUCT.md: that one says what Mu is and who
arrives, this one says what happens when they use it.

Written after finding that the answer to "what does a call cost" depended on
which of four code paths a service had been written against, and that most
priced operations were free in practice. The mechanism is fixed. What it should
charge is a product decision and was never written down.

## Where we actually are

Measured, not remembered.

| | |
|---|---|
| A new account starts with | **0 credits** |
| One credit is | **£0.01** |
| Subscriptions on offer | **£10 → 1,000**, **£20 → 2,000**, **£100 → 10,000** credits a month |
| `daily_quota: 100` in quota.json | **dead** — declared, documented, configurable, read by nothing |
| Operations priced at 0 | 15 of 27 |
| Charging path | one gateway, since `127a326` |

The plans were £5 → 500 and £10 → 1,200, and the second half of this document
was written to answer why nobody would buy either. £5 buys 500 credits, which is
£5 of credits: the tier sold nothing a top-up did not, except at £10 where it
sold a 20% discount on the one thing with a marginal cost. A subscription that
is a payment method rather than a product has no reason to exist.

They are now Personal, Pro and Premium, a credit is 1p on all three, and what a
plan sells is scale — agents, the channels that spend real money, and rate
limits. There is no free plan; see *No free plan* below. `/pricing` renders from
`wallet.SubscriptionPlans`, so the page that advertises a plan and the page that
charges for it cannot disagree again. Held by `wallet/plans_test.go`.

**A new account cannot do anything that costs money.** It starts at zero, and
deliberately. The only reason the product appears to work is that most things
are priced at zero, which is a different fact and a fragile one — the moment
pricing is applied properly, a new account hits a wall on its first search.
That wall is the decision point, and moving it is what a plan is for.

## The user we are pricing for

Not a consumer with an app subscription. Somebody wiring an agent to the
everyday internet, whose alternative is six or seven provider accounts: Brave
for search, Google for places and routes and weather, Twilio for a number, a
sending domain with DKIM, YouTube for video, somewhere to keep records. Six
signups, six cards, six tokens, six sets of terms.

That is what a tier is competing with, and it is why £5/month reads as a toy.
The comparison is not "is this cheaper than another app", it is "is this cheaper
than the afternoon I would spend signing up for six things, plus their
minimums".

## Price the work

The instinct to keep things free deserves stating so it can be argued with,
because it was the reasoning in quota.json and it is wrong.

It said: an operation costs a credit when it costs *us* something — a model
call, a third party billing us per request — and anything touching only this
instance's own storage is 0, because charging for it taxes the behaviour we
want more of. Fifteen of twenty-seven operations sit at 0 on that argument.

The flaw is that it counts only the marginal cost and none of the work. Feeds
are polled, articles fetched and de-duplicated, sentiment tagged, categories
maintained, an index kept, an inbox run with DKIM on a domain whose reputation
we defend. Somebody calling `news_list` is not touching free storage, they are
reading the output of a pipeline that runs whether or not they call it. You buy
clothes and they cost money. This is not a loss leader for something else — it
*is* the product.

**So every call through the tool surface costs at least one credit.** A base
unit, and more where a provider charges more. That is simpler to explain than a
list of exceptions, it makes "as usage scales we earn" true rather than
aspirational, and it removes the question a reader currently has to ask about
every single tool.

One honest cost of this: it prices the cheap sticky operations — keeping a note,
writing a document — which are exactly what makes somebody stay. The answer is
not to make them free for ever, it is to make the free grant big enough that the
habit forms before the meter bites. That is a number to tune, not a principle to
abandon.

## Where the meter is, and where it is not

This is the line that decides everything else, and it is not "which service" —
it is **which door**.

**Every MCP and API call is metered.** That is the product. Somebody wiring an
agent to this is doing it because one account replaces seven, and they expect
per-call pricing because that is what every provider they are replacing does.

**The signed-in web app is included.** Reading Home, opening a card, browsing
news, looking at a route: no credits.

Three reasons, and the third is the one that settles it:

1. **You cannot see it coming.** In a UI you do not know which click costs
   money. A page that silently debits on render is a page nobody trusts, and
   defending against it means reading the pricing table before every click.
2. **The app is the proof, not the product.** PRODUCT.md is explicit that the
   signed-in app exists to demonstrate the tools work. Charging for the
   demonstration is charging for the advert.
3. **Nobody expects it.** Arriving at something called *tools for agents*,
   seeing pricing in credits per tool call and per agent — the honest answer to
   "do I expect clicking a page to cost?" is no. Pricing that surprises people
   is a support burden and a reason to leave.

Two exceptions, both because they are *actions* rather than views, and both
things a person deliberately clicks a button to do:

- **The agent.** Asking it something is a model call and is charged wherever it
  is asked from.
- **The three channels.** Sending mail out, a text, or opening a WhatsApp
  conversation costs real money and is charged from the UI too. These are
  button clicks with a confirmation, so the charge is never a surprise.

Apps get the same treatment from the other side: an app somebody builds is free
to *serve*, because rendering HTML costs nothing and static hosting has been
free for a decade. What is not free is an app calling our services through the
SDK — that is the tool surface again, charged to whoever runs the app, and it is
what lets somebody build a business on top and pay us as it grows.

## No free plan

This section used to design one. It concluded with 500 credits on signup and
100 a month after, and it was wrong in a way worth keeping on the page rather
than deleting, because the argument for it is the one that will be made again.

The argument was: pricing every call puts a paywall in front of somebody who has
not yet seen the product work, and that is the failure mode that kills adoption.
True. The error was in what it costs to answer it. **Every priced call here
spends somebody else's money** — Atlas for inference, Brave for search, Google
for places and routes, Twilio for a text. There is no margin to give away
because there is no marginal cost of zero to give away *from*. A free tier is
therefore not a marketing spend with a known ceiling; it is a bill that scales
with exactly the accounts that use it most, and 500 credits on signup is £5 of
provider spend for every address that can receive a confirmation email.

That is different from the businesses this reasoning was borrowed from. Google
can give away ten thousand Routes calls because Google owns the routes. We do
not own anything underneath.

**The free option is self-hosting, and it is a real one.** The binary is AGPL,
it takes your provider keys rather than ours, and an instance with no Stripe and
no x402 configured cannot charge anybody — so nothing is metered and every tool
is free. Somebody who wants this for nothing can have all of it for nothing.
What is sold at micro.mu is not the software. It is not having to hold thirty
accounts.

What remains true from the old argument is that the wall should be somewhere
useful. That is what the ordering constraint at the end of this document is
about: meter the tool door before pricing everything, so the app a person is
looking at is included, and the wall is at the point where an *agent* starts
spending — which is the point where somebody can already see what they are
buying.

## One meter, not two

Given that, the free-tier question narrows to how an *operator* — anyone running
their own instance, us included — grants usage. Two ways, and they are not
equally good.

**As a second mechanism** — a monthly quota of calls per operation, the way
Google does it, sitting alongside credits. This is honest about provider
economics and it is what the machinery now supports (`free` per operation in
quota.json). It also means somebody has two numbers to understand, two things
that can run out, and two explanations on the wallet page.

**As one meter** — everything is priced in credits, and the free tier is a
monthly grant of them. One number. It runs out in one way. "You have 100 credits
this month" needs no further explanation, and "top up" is the same action
whether you are a free user or a paying one.

**Take the second.** The per-operation allowance stays in the code — `free` per
operation in quota.json, `internal/quota/allowance.go` — because an operator may
want Google's shape, and because it is the right tool for passing a provider's
own free tier through. Every entry is zero on this instance, so it is inert
here, and it is not the story anybody is told.

So: *a credit is the unit of everything*. A plan is credits that arrive monthly
and do not accumulate, and there is no plan where they arrive for free.

## The thing that must not be free

Some operations cost real money the moment they are called and cannot be
undone. Sending an email leaves this instance under our domain and our DKIM
signature. Sending a text costs the carrier's fee and spends the reputation of a
phone number every account on the instance shares. Starting a WhatsApp
conversation does the same.

These have been abused before, on this product, at scale, and the reason was
exactly this: they were reachable on free usage. A free allowance on an
operation like that is not a marketing decision, it is an open relay.

**So an operation may be marked as never free.** It is not payable from the
monthly grant and it is not payable by an account that has never paid. Mail out,
SMS and WhatsApp are the three today. The test is not "is it expensive" — an
image costs more than a text — it is *does calling it spend something we cannot
get back, in a way that harms other users of this instance*. Reputation and
deliverability are shared; a credit balance is not.

This is the one place where two buckets exist rather than one, and it is
invisible until you try: the wallet shows a single balance, and the refusal on a
never-free operation says what to do about it.

## Dynamic pricing, and when it is two endpoints instead

Three operations do not have a constant price, and they are not the same case.

**`mail.Send` charges one thing locally and another externally.** That is not
dynamic pricing, it is two operations wearing one name. Sending a message to
somebody on this instance and sending an email to the outside world differ in
price, in permission, in abuse profile and in what goes wrong — one is a row in
a database, the other is our domain's reputation. An agent choosing a tool
should be able to read what it costs before calling it, and a price that depends
on an argument cannot be shown in the catalogue.

> **Split it.** `mail_send` for a message to somebody here, `mail_email` for a
> real email leaving the instance. Two endpoints, two flat prices, and the
> never-free mark lands on exactly one of them.

**`sms.Send` is priced per 160-character segment.** That is genuinely dynamic
and splitting it is absurd — there is no such thing as a three-segment endpoint.
The price is constant and the *quantity* varies.

**`whatsapp.Send` charges only when a message opens a conversation.** Also
quantity: one, or zero for a reply inside the window.

> **One mechanism covers both:** the gateway charges `Cost × units`, where units
> defaults to 1 and a handler may report a different count. A text of three
> segments reports 3. A WhatsApp reply reports 0. Nothing else changes, the
> price stays a constant an agent can read, and the single system of record
> survives.

The rule that falls out, and the one to apply to the next case:

- The **price** differs → two endpoints.
- The **quantity** differs → one endpoint, units.

## What the tiers should be

A subscription must not sell credits at a discount. £10 buying 1,200 credits is
selling £12 for £10 on the one thing that costs us money, which loses more the
more successful it is. Credits are bought at face value, always: **1 credit = 1p,
subscription or not.**

So what does a tier sell? **Scale.** The thing somebody outgrows is not how much
they spend — they can top up — it is how many agents they may run, how hard they
may hit it, and whether the real-world channels are open to them. Those cost us
in ways credits do not: an agent is a token to keep, a rate limit to hold, a
share of a phone number's reputation.

| | Free | Pro — £20/mo | Scale — £100/mo |
|---|---|---|---|
| Credits included | 100/month | £20 of credits, at face value | £100 of credits, at face value |
| To get started | 500 once | — | — |
| Agents | 1 | 5 | 25 |
| Mail out, SMS, WhatsApp | no | yes | yes |
| Rate limit | modest | higher | highest |
| Card needed | no | yes | yes |

Everything above what a tier includes is bought at the same 1p. There is no
tier at which a credit is cheaper, and the pricing page should say so — it is
the sentence that stops anybody wondering whether they are on the wrong plan.

Plus the two that are not tiers and should stay: **top up any amount** with no
subscription, and **x402**, per-call payment in USDC with no account at all —
the thing nobody else offers and the reason an agent can use this with no human
present.

Why these shapes:

- **£20 rather than £5.** £5 competed with nothing and signalled a toy. £20 is
  about what one provider's entry plan costs, for all of them.
- **The included credits are a prepayment, not a discount.** £20 a month gets
  £20 of usage. The subscription is worth having for the agents, the channels
  and the limits, and those cannot be bought à la carte.
- **The channels are the paid line, and the honest reason is identity.** A card
  on file is worth more to us than the money there: it is the thing that makes
  spam expensive. That should be said plainly rather than dressed up as a
  feature gate.
- **Agents, not seats.** The unit somebody scales is how many agents they run,
  which is also the unit that costs us.

## Getting off the ground

The failure mode at one end is giving everything away; at the other it is a
paywall before anybody has seen it work. Pricing every call makes the second
easy to walk into, and *No free plan* above rules out buying our way out of it
with a grant.

So it is bought with sequencing instead. A visitor sees the tool catalogue,
the prices with what each one costs us beside it, and the app itself — none of
which is metered — before they are asked for anything. What they cannot do
without paying is have an agent spend our providers' money, which is the one
thing that has a bill attached.

The number to instrument is not the size of a grant, it is where people stop:
how far a new account gets before it hits the wall, and whether the thing they
were trying to do was visible enough by then to be worth £10. That is a
measurement, and it should be taken before anyone argues about the tiers again.

## What the pricing page shows

The worry that prompted this: if everything is priced, the page becomes a wall
of numbers and the product looks expensive.

It does the opposite. A page that has to explain *which* calls are free is
harder to read than one that says:

> **Every tool call costs 1 credit. These cost more:** search 2, places 5,
> routes 2, directions 3, the agent 7, an image 15, a text 10 per segment.
>
> **A credit is 1p.** The web app is included — you are only charged for what
> your agents call.

That is the whole thing. One rule and a short list of exceptions, rather than
twenty-seven rows a reader has to scan to find out whether the thing they want
is on the free half.

Worth showing beside each price: **what it costs us.** That is already in the
`note` field of every line in quota.json — *0.38p to Google, we charge 2* — and
it turns a price list into an argument for the product. Nobody who sees that
thinks they are being gouged, and it makes the "one account instead of seven"
claim concrete.

An agent needs the same as JSON, which `/pricing` already serves.

## Performance

Asked, so measured rather than assumed.

| | |
|---|---|
| A service call over the RPC | **~85µs**, 188 allocations |
| The gateway's own overhead | **below the noise floor** — the priced path benchmarked *faster* than the free one, so the difference is smaller than the variance of the call it wraps |

The gateway is a map lookup and a string scan. Nothing that goes in it later —
rate limits, moderation, audit — should be assumed as cheap; each one gets
measured against this before it lands.

The number that matters for the work still to do is the 85µs. A page calling a
service function directly pays nothing for the hop. A page calling its endpoint
pays an RPC. For a page making a handful of calls that is under a millisecond
and worth it for one charging path — but it is a decision to make with the
figure in hand.

## What this means for the code

In order, each landable on its own:

1. **Split `mail.Send`** into `mail_send` and `mail_email`. Removes the first of
   the three variable-price cases and makes the never-free mark expressible.
2. **Units on the gateway.** `Cost × units`, defaulting to 1. Removes the other
   two, and `sms` and `whatsapp` stop charging themselves.
3. **Never-free operations.** A flag in quota.json, enforced in the gate, with a
   refusal that says what to do about it.
4. ~~**`Account.Plan`.**~~ Done. The webhook carried the plan id in its
   metadata and threw it away, so nothing in the product had ever read a plan —
   which meant "1 agent", "5 agents", "25 agents" and "higher rate limits" were
   sold on the pricing page against no field, no counter and a rate limit that
   varied by account age. `Account.Plan` is set from the webhook;
   `SubscriptionPlan` carries `Agents` and `PostsPerHour`; `CreateAgent`
   enforces the count and `postLimitFor` reads the rate, both through function
   variables filled in by `hooks.go` because `auth` and `agent` sit below
   `wallet`. The pricing card renders those same two numbers, so it cannot
   claim what is not enforced. Held by `test/plans_are_real_test.go`.

   **Still open: the channels.** "Send mail, SMS and WhatsApp" was the fourth
   claim on the Pro card and it is off the card rather than enforced, because
   unlike the other three it has no obvious right answer — whether £10 buys the
   ability to send an email at all is a decision nobody has made, and gating it
   would take something away from every account that can do it today.
5. **Meter the tool door, include the app.** The gateway already knows the
   difference is not visible to it, so the door has to say: a call arriving from
   a page is marked as included, everything else is charged. That is one flag on
   the context, set where identity is already stamped.
6. **Price the currently-free operations at 1**, once 4 and 5 are in — not
   before, or a new account meets a wall on its first call.
7. **The tiers**, and the pricing page as one rule plus exceptions.
8. **Stage 2 of the gateway** — pages call endpoints — which empties the
   `pageCharged` ledger in `test/charging_test.go`.

Note the ordering constraint: **5 before 6.** Metering every operation while the
web app is still charged would make browsing cost money, which is the outcome
this document exists to prevent. With no signup grant to soften it, 5 is the
only thing standing between a new visitor and a paywall on the first page they
look at, so it is not optional and it is not last.

Instrument before tuning: how far a new account gets before it hits the wall,
and whether it comes back. Every number on the pricing page is a guess until
that exists.
