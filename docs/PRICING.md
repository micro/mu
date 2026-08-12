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
| Subscriptions on offer | **£5/month → 500 credits**, **£10/month → 1,200 credits** |
| `daily_quota: 100` in quota.json | **dead** — declared, documented, configurable, read by nothing |
| Operations priced at 0 | 15 of 27 |
| Charging path | one gateway, since `127a326` |

Two of those need saying plainly.

**The subscriptions are not a product.** £5 buys 500 credits, which is £5 of
credits. The tier sells nothing that topping up does not, except at £10 where it
sells a 20% discount. A subscription that is a payment method rather than a
product has no reason to exist, and no reason for anybody to pick it.

**A new account cannot do anything that costs money.** It starts at zero. The
only reason the product appears to work is that most things are priced at zero,
which is a different fact and a fragile one — the moment pricing is applied
properly, a new account hits a wall on its first search.

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

## One meter, not two

A free tier can be built two ways and they are not equally good.

**As a second mechanism** — a monthly quota of calls per operation, the way
Google does it, sitting alongside credits. This is honest about provider
economics and it is what the machinery now supports (`free` per operation in
quota.json). It also means somebody has two numbers to understand, two things
that can run out, and two explanations on the wallet page.

**As one meter** — everything is priced in credits, and the free tier is a
monthly grant of them. One number. It runs out in one way. "You have 100 credits
this month" needs no further explanation, and "top up" is the same action
whether you are a free user or a paying one.

**Take the second.** The per-operation allowance stays in the code because an
operator running their own instance may want Google's shape, and because it is
the right tool for a provider free tier that we are passing through. It is off
by default and it is not the story anybody is told.

So: *a credit is the unit of everything*. The free tier is credits that arrive
monthly and do not accumulate.

Note the wording, because it was wrong in an earlier draft of this thinking:
"100 free credits" sounds like a balance, and a balance that never renews is a
wall with a delay on it. It has to be **100 credits a month**, refilled, or the
free tier is a trial pretending to be a tier.

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

The free tier exists so nobody meets a paywall before they have seen the thing
work. The paid tiers exist because we are paying providers per call and would
like to make money.

| | Free | Pro — £20/mo | Scale — £100/mo |
|---|---|---|---|
| Credits each month | 100 | 2,500 | 15,000 |
| What that is worth | £1 | £25 | £150 |
| Mail out, SMS, WhatsApp | no | yes | yes |
| Agents | 1 | 3 | 10 |
| Card needed | no | yes | yes |

Plus the two that are not tiers at all and should stay: **top up any amount** at
1 credit = 1p with no subscription, and **x402**, which is per-call payment in
USDC with no account whatsoever — the thing nobody else offers and the reason an
agent can use this without a human present.

Why these numbers:

- **£20 rather than £5.** £5 was competing with nothing and signalling a toy.
  £20 is roughly what one provider's minimum plan costs, for all of them.
- **The credits are worth more than the price at every paid tier.** That is the
  product: the subscription is not a way to buy credits, it is a discount that
  gets better as you commit, plus the things money unlocks. £5→500 was 1:1 and
  therefore pointless.
- **The channels are the paid line, not a credit count.** Mail, SMS and WhatsApp
  are where abuse lives and where a card on file is worth more than the money —
  it is an identity. That is the honest reason they sit behind the paid tier,
  and it should be said that way rather than dressed as a feature gate.
- **Agents, not seats.** The unit somebody scales is how many agents they run,
  which is also what costs us — each one is a token, a rate limit and a share of
  the number's reputation.

## What the pricing page shows

The worry that prompted this: if everything is priced, the page becomes a wall
of numbers and the product looks expensive.

It does not, because **most things stay free and should**. An operation costs a
credit when it costs *us* something — a model call, or a third party we are
billed for. Anything that only touches this instance's own storage is 0, and
charging for it would tax exactly the behaviour we want more of. Fifteen of
twenty-seven operations are already at 0, and the gateway passes those through
without even asking who is calling.

So the page is two lists, not one table:

**Included, unmetered** — news, notes, docs, files, contacts, events, blog,
social, stream, chat, video, the Quran and hadith, the CLI, every read of
anything you already own.

**Metered** — search, weather, places, routes, images, the agent, and the three
channels. With the price, and what it costs us, which is already in the `note`
field of every line in quota.json and is worth showing: *0.38p to Google, we
charge 2.*

An agent needs the same list as JSON, which `/pricing` already serves.

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

In order, each one landable on its own:

1. **Split `mail.Send`** into `mail_send` and `mail_email`. Removes the first of
   the three variable-price special cases and makes the never-free mark
   expressible.
2. **Units on the gateway.** `Cost × units`, defaulting to 1. Removes the other
   two, and `sms` and `whatsapp` stop charging themselves.
3. **Never-free operations.** A flag in quota.json, enforced in the gate, with a
   refusal that says what to do.
4. **The monthly grant.** 100 credits, refilled, replacing the dead
   `DailyQuota`.
5. **The tiers**, and the pricing page as two lists.
6. **Stage 2 of the gateway** — pages call endpoints — which is where the
   `pageCharged` ledger in `test/charging_test.go` empties out.

Do 1 to 4 before 5, because the tiers are a promise and the machinery should be
able to keep it first.
