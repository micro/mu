# Business model decision: sell the agent, meter the work

This note evaluates the three businesses Mu has tried to be and answers the
specific question raised by the agentic inbox: **where, if anywhere, does x402
belong?** It is a decision memo, not a price sheet. `PRICING.md` remains the
source of truth for current prices and limits.

## Decision

**Lead with a hosted agent that has its own address and inbox. Sell that hosted
product on a recurring plan, include usage at face value, and charge for
overages. Keep credits as the single internal usage meter. Keep x402 as the
accountless payment rail for programmatic tool calls, not as the way a person
pays for their inbox.**

In one line:

> Subscription buys the standing agent; credits buy the work it does; x402
> lets outside agents buy individual calls without becoming customers.

These are three layers of one model rather than three competing models.

## What the customer is buying

The durable product is not a server and not a bag of API calls. It is a
reachable delegate:

- it has a stable address;
- work can arrive while its owner is away;
- it remembers the thread;
- it can use the service catalogue to do the work; and
- the owner returns to an inbox containing outcomes, not an empty chat box.

That is recurring value even in a quiet month. Hosting, address continuity,
mail reputation, retention, scheduled or event-driven availability, and trust
are what justify a subscription. Model and provider usage still varies, so it
should not be hidden inside an unlimited promise.

The current inbox is not yet all of that. The repository itself says `/inbox`
is still a conversation log over `internal/thread`, while the actual mailbox is
`service/mail`. The business should be tested only after the product direction
already documented in `DIRECTION.md` lands: make the inbox the mailbox, with
the agent participating in it. Charging for an aspiration before that would
repeat the earlier plan mistakes.

## Evaluation of the three models

### 1. Personal home server subscription

**Verdict: keep self-hosting as distribution and trust, not as the primary
hosted offer.**

The home-server framing asks a customer to value infrastructure: installation,
updates, uptime, backups and remote access. The strongest users of a home
server are also the most able and inclined to self-host it, which fights a
subscription. Less technical users do not want a server at all; they want the
outcome. Support and heterogeneous home networks grow faster than recurring
revenue.

Self-hosting is still strategically useful. It makes the privacy and ownership
claim credible, creates an adoption path with no provider subsidy, and allows
operators to supply their own keys. It should remain the free/open deployment
option and an enterprise escape hatch, with paid support or managed deployment
considered only when customers ask for it.

### 2. Tools for agents, paid with credits

**Verdict: retain as the economic engine and developer channel, but do not rely
on it as the whole product.**

This model has the cleanest cost alignment. Each metered request can cover the
upstream provider, credits provide one comprehensible unit, and the catalogue
offers one relationship in place of many. It also already maps onto MCP, REST
and the common gateway.

Its weakness is demand and retention. A catalogue of tools is compared line by
line with direct providers and the growing number of MCP servers. Usage is
bursty, switching costs are low, and a dormant integration produces no revenue.
Breadth and account consolidation are real value, but they are more compelling
as capabilities of a delegate somebody depends on than as a catalogue somebody
occasionally calls.

Credits therefore remain the accounting primitive for:

- provider-backed operations;
- agent model calls and tool fan-out;
- signed-in customers' overages; and
- cost and margin reporting across every entry point.

They should not be presented as a second product or a game currency. One credit
must continue to have one price, included plan credits are prepayment rather
than a discount, and every run should show what it spent.

### 3. Agentic inbox subscription

**Verdict: strongest primary business, once the mailbox and asynchronous value
are real.**

The inbox gives Mu a recurring job and a natural return loop. An address is a
low-friction interface for people, software and other agents; messages arrive
without the owner opening Mu; and history compounds. This is meaningfully
different from visiting a chatbot or wiring one more tool server.

The offer should be packaged around things with recurring value rather than
arbitrary scarcity:

- number of continuously reachable agents or addresses;
- retention and searchable history;
- scheduled/event-driven work when it exists;
- channel and domain identity;
- collaboration, audit and administration for teams; and
- support or uptime commitments at the business tier.

Usage that creates variable cost remains metered. Avoid “unlimited AI,” avoid a
discounted credit bundle, and do not sell an abuse limit as though it were a
feature. A simple initial offer is one paid hosted plan for an individual and
one for an organisation, each including the same cash value of credits as the
monthly prepayment. Keep top-up available so a customer never has to change
plans merely because one month is busy.

## Does the inbox use x402?

### Yes at the machine boundary

Mu already implements x402 as HTTP payment middleware: an unauthenticated
caller receives a `402 Payment Required` challenge, signs a stablecoin payment,
retries the request, and receives the result and settlement receipt. That is a
good fit when an outside agent calls a priced tool over MCP or REST because the
caller is software, the unit is one request, and creating an account would be
pure friction.

Keep x402 for:

1. accountless MCP and REST calls to the service catalogue;
2. discovery and purchase of a capability by another agent;
3. later, paid A2A tasks where a price and deliverable are agreed before work;
4. operator-to-operator settlement in a federated or self-hosted network.

This is differentiated distribution: a customer can bring an agent that has a
wallet but no Mu account. It is also orthogonal to the hosted inbox plan. A
subscriber can use credits; a machine with no account can pay the equivalent
price over x402; both go through the same quota and operation definitions.

### No for ordinary inbox billing

Do **not** put a 402 challenge in front of reading mail, receiving ordinary
mail, opening the inbox UI, or every message sent to a person's agent. Those
flows need identity, spam controls, storage, refunds/support and predictable
billing. A card subscription plus a credit balance is much easier for a person
to understand than funding a wallet and approving a stablecoin transfer.

Receiving must not depend on the sender supporting x402. The address is the
smallest interface precisely because email, forms and cron jobs can already use
it. Requiring payment at that boundary would remove the central advantage.

There is a narrower future experiment: an agent may publish a **paid task
address** or A2A endpoint where an unknown machine prepays for a defined job.
That should be a separate, explicit door—not a toll on the normal mailbox. It
needs acceptance rules, refunds or failure semantics, a maximum workload, spam
policy, and a clear split between Mu and the agent owner before it becomes a
business model.

## Recommended revenue architecture

| Layer | Buyer | What they pay for | Payment mechanism |
|---|---|---|---|
| Hosted inbox | Person or organisation | Persistent agent, address, retention, administration and availability | Recurring card invoice |
| Agent work | Signed-in customer | Model calls, provider-backed tools and outbound operations | Credits, including plan prepayment and top-ups |
| Tool door | External software or agent | One MCP/REST operation without an account | x402 per call |
| Self-hosting | Operator | Their own infrastructure and provider accounts | Free software; optional support later |

The same operation must cost the same underlying number of credits regardless
of door. The payment mechanism may differ, but the price catalogue and usage
record must not.

## What to validate before changing prices

Do not choose final tiers from opinion. Instrument one end-to-end cohort and
answer these questions:

1. **Activation:** did the user create an agent address and receive a real
   external message?
2. **Delegation:** did the agent complete work, rather than merely answer a
   chat prompt?
3. **Return:** did something useful arrive while the owner was absent, and did
   they return within seven days?
4. **Economics:** what were model, tool, mail and support costs per active
   inbox, and what gross margin remained after included credits?
5. **Rail fit:** what share of tool revenue came from accounts, top-ups and
   x402, and how many x402 callers repeated?
6. **Willingness to pay:** do activated users prefer a recurring hosted agent,
   pure pay-as-you-go, or self-hosting?

The first useful pricing experiment is not three polished tiers. It is a single
monthly hosted-inbox offer with included credits at face value, top-up for
overages, and the existing x402 tool door left unchanged. Compare its activation,
four-week retention, gross margin and support load with pure pay-as-you-go.

## Risks and guardrails

- **The inbox is only a skin over chat.** Do not charge recurring rent until
  work genuinely arrives and gets handled asynchronously.
- **Email becomes the entire category.** Lead with an agent that has an address,
  not an AI email client; every channel should write to the same work record.
- **Included usage hides bad margins.** Keep credits visible internally and cap
  spend rather than promising unlimited work.
- **x402 becomes the pitch.** It is a rail, not the customer outcome. Mention it
  to developers and autonomous callers, not as a prerequisite for ordinary
  users.
- **Outbound abuse damages shared infrastructure.** Keep payment, trust and
  daily limits on external sends; isolate agent-mail reputation before raising
  volume.
- **Three payment stories confuse the buyer.** Present one appropriate choice
  per door: subscription/top-up to a person, x402 to an accountless agent, own
  keys to a self-hoster.

## The resulting positioning

**For people:** give your agent an address; work arrives, it handles what it
can, and you come back to the result.

**For developers and outside agents:** one tool endpoint replaces many provider
accounts; pay per call with an account balance or x402.

**For operators:** run the same network yourself, with your own providers and
payment address.

The inbox is the product, the catalogue is its capability and margin engine,
and x402 is the accountless route into that engine.
