# What things cost, who pays, and how we make money

The model, what it excludes and why, and the arguments that were made and lost.
`quota.json` is the prices; this is the reasoning behind them. Check a pricing
change against both.

## The model

Three rules, and everything follows from them.

**A credit is a penny, and a credit is charged when a call costs this instance
money.** A model call to a provider we pay, or a third party billed per request:
Atlas for inference and images, Brave for web search, Google for places, routes
and weather, Twilio for SMS and WhatsApp. Nothing else is charged, because
nothing else has a marginal cost.

**Anything that only touches this instance's own storage is 0.** Notes,
documents, files, the blog, apps, the inbox, local mail, the record. 13 of 30
operations are 0. Charging for them taxed exactly the behaviour the product
wants more of, and the control they need is abuse control, which is
`auth.CheckPostRate` and not a price.

**A price is what a fetch costs.** A call answered from something the instance
already had — a forecast someone asked for half an hour ago, a stored tile — is
not charged. See `internal/service/meter.go`. The busier a shared instance is,
the less its callers pay, which is the only argument for shared infrastructure
that survives contact with a bill.

The agent is not priced at all. It is not a service, so it has no operation to
charge for; it is the thing you came for, and metering it meant a new account
could not ask a question until it had paid. What bounds it is a count of runs a
day.

## Where the money is

| | |
|---|---|
| A new account starts with | **0 credits** |
| One credit is | **£0.01** |
| Ways to pay | **top up** any amount at 1p a credit · **x402** per request, no account · **self-host**, nothing metered |
| Priced operations | **17 of 30** |
| Operations at 0 | **13 of 30** |
| Plans | **none** |
| Daily grant of credits | **none** |
| A call served from cache | **not charged** |
| Charging path | one gateway, `quota.Charge` |

The margin is the spread between what a provider bills and what the table says:
`web_search` is 2p against Brave's ~0.4p, `places_search` 5p against ~2.5p,
`text_summarise` 2p against ~0.1p. The notes in `quota.json` carry the
underlying cost for each, so the markup is visible rather than folklore.

## The one price that is not a cost

`mail_email` — your own address answering somebody outside — is 4p and capped
per day. Sending it costs this instance
nothing — it is our own SMTP server and our own domain — so by the rule above it
should be 0.

It is not, and the reason is worth stating plainly rather than hiding: what a
loop spends there is the sending domain's reputation, and no balance repairs
that. The same domain carries password resets and sign-in links for everybody on
the instance. `external_email`, `whatsapp_send` and `sms_send` are the other three that reach
a stranger, and those have a real provider bill as well — all three are Twilio.

The mailbox and the channel had one operation between them until now, so they
shared its daily cap: ten `email_send` calls left an account unable to answer
its own correspondence, and answering correspondence left it unable to send.
Two products competing for one budget is what one meter across two services
buys you.

The price and the cap do different jobs and both are wanted. A price stops
somebody who has to pay and does nothing about a loop; a cap stops the loop and
does nothing about somebody with a card. This is the one operation where the
price is a deterrent rather than a cost, and it is an exception on purpose. If a
second one is ever proposed, that is the moment to check whether the rule still
holds or whether it has quietly become "we charge for what we like".

An operator relaying through a paid smarthost — SES, Postmark — does have a
per-message bill, and `CREDIT_COST_EXTERNAL_EMAIL` is theirs to set.

## The mailbox

Worth its own line because it is the largest thing that is free, and because a
reader assumes otherwise.

Receiving costs nothing, however much arrives. Reading costs nothing: the
address opens in any mail client over IMAP, with an access token as the
password. Replying from that client costs what replying from the web costs,
because both go through `SendOut`/`ReplyOut` — one door, one gate, one charge.
Local mail between two accounts here is 0.

So an address on this instance, read and answered in Thunderbird all day, bills
nothing until a message is addressed outside.

## What must never be free

An operation may be marked as never payable from a promotional balance. The
reason is Twilio: SMS and WhatsApp were the two things a free tier made
reachable, and a stranger's mail can drive an agent. Anything that reaches a
person who did not ask — a text, a WhatsApp message, mail to an address outside
— is real money and real reputation and must be spent from a balance somebody
put money on.

## Rejected, and why

Kept because each will be proposed again, and the argument that killed it is the
answer.

**Plans.** Tried three times: two price tiers, then three columns, then columns
without the styling. Every version came apart on the same fact — a credit is a
penny and every operation costs what `quota.json` says, whoever is asking. A
subscription that buys the same credits at the same price is a payment method,
not a product; one that buys them cheaper is a second price list to keep in step
and an answer to "what does this cost" that begins "it depends". `/pricing` is a
price list and there is nothing on it to choose.

**A daily grant of credits.** There was one, and it cancelled itself out: the
grant existed to make the agent usable without paying, and the agent was the
thing being charged for. Deleting both was one change, not two. A grant is also
a bill that scales with the accounts that use it most.

**Metering every call.** Proposed as "every call through the tool surface costs
at least one credit", on the grounds that a list of exceptions is harder to
explain than a flat rule. It prices the cheap sticky operations — keeping a
note, writing a document — which are exactly what makes somebody stay, and it
charges for operations that cost nothing to serve. The exceptions are not a
list to memorise: they are one sentence, which is that we charge when somebody
bills us.

**Metering the web app separately from the API.** The door is not the unit. What
is charged is the operation, whoever asked for it, because the same tool called
two ways costing two amounts is two price lists again.

**A free tier.** There is none, and the free option is self-hosting, which is a
real one: the binary is AGPL, every price comes from one file, and an instance
you run charges what you set — including nothing, at which point callers pay
you rather than us.

## Where the price list lives

`quota.json` at the top of the repo, and nowhere else. `main.go` embeds it and
calls `quota.Load`, so the package that answers what something costs does not
decide where the answer comes from. A service names *which* operation it charges
— `Cost: quota.OpWebSearch` on its Endpoint — and what that operation costs is
an operator's decision, so it is data.

Every surface renders from it: the price on each entry at `/tools`, the table at
`/pricing`, the cost lines at `/usage`. Four hand-maintained tables drifted
before, and three of them had lost the most expensive operation in the product.
An operator can drop a `quota.json` in the data directory to override without
rebuilding.

`internal/quota` holds prices and does not know what a balance is. `account/`
fills in the half it cannot answer, from its own `init`, because quota sits
underneath it. A service never imports `account/`.

## Abuse control is not pricing

`auth.CheckPostRate` stops bots. The credit charge prices real cost. The daily
`limit` in `quota.json` bounds what reaches a stranger. Three mechanisms, three
jobs, and collapsing any two of them is how the allowance came to exist.
