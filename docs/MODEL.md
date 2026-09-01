# The model

What this is, in the terms it is built from. No metaphors that do not forbid
something.

## The parts

| | |
|---|---|
| **Service** | An atom. A fixed building block that answers a question not depending on who asked. The set is bounded and roughly complete at 34 — you do not add elements. |
| **Endpoint** | The quantum. One method, one typed request, one typed response. The unit of work, of billing, and of description. Nothing is made of half of one. |
| **Agent** | A selection over the registry plus words. An id, a subset of services, a prompt. No code, no deployment, unbounded in number. |
| **App** | A compound. Assembled from services, sandboxed, priced by its author. Unbounded, and never enters the tool list — a planet is not an element. |
| **Client** | A front door. Each one an open protocol somebody already owns a client for. |
| **Thread** | The causal record. Everything that happened, whichever door it came through. |

Services never call each other, and never call an agent. All interaction is a
call passing through the harness — atoms interact by emitting and absorbing,
not by touching. This is enforced, not documented: `TestServicesDoNotImportEachOther`,
`TestNoServiceImportsTheAccount`, `TestNoNewServiceCallsAnAgent` (zero, and it
stays zero), and a hook ledger that counts every function variable trying to
route around them.

The consequence is that adding a service costs the agent nothing: no import, no
wiring, no code change. `service.Register(Spec)` is the whole cost.

## The doors

Every client is a protocol with an installed base. Nothing here asks you to
install something we own.

| | | |
|---|---|---|
| **Mail** | SMTP in, IMAP out | working |
| **Web** | HTTP | working |
| **Agents** | MCP, or plain HTTP | working |
| **Terminal** | SSH | working |
| **Chat** | XMPP: C2S, WebSocket, and S2S with dialback | working, federating |
| **Money** | x402 on Base, and Stripe | working |
| **Voice** | SIP, Jingle for signalling | not started |

## The economics

- A credit is a cent. USDC is dollars. A hundred credits is one USDC, so there
  is no exchange rate anywhere in the system.
- Tools are priced at what they cost us plus assembly. The markup is published
  per operation in `quota.json`, including the multiple — the buyer knows what
  we know.
- Anything touching only this instance's own storage is free. There is no
  marginal cost, and charging for it taxes the behaviour the product wants.
- Talking to your agent is free.
- **Apps: the author keeps the whole price.** What an app costs to run is
  metered per operation where it happens. Taking a further percentage of the
  sale would be charging twice for one thing — once by cost and once by
  position.
- Two ways in that are not a card: convert USDC, or be sent credits by somebody
  who has them.

The rule underneath all of that: charge for what was done, never for permission
to do it. A fee that scales with somebody else's revenue while your cost does
not is rent, whatever its size.

## What is missing

Stated plainly, because a roadmap that only lists strengths is marketing.

- **Federation is new and unproven against real servers.** S2S with dialback is
  built in both directions and tested against its own logic; it has not been run
  against a Prosody or ejabberd deployment, which is the only test that counts.
  Presence does not cross servers yet — messages do.
- **No voice or video.** No SIP, no Jingle, no TURN.
- **Fiat still arrives through Stripe**, with Stripe's toll.
- **The upstream rent is still paid.** Model vendors, Google, Twilio, all billed
  per request. We are one layer inside the structure, reselling with a stated
  markup — not outside it.
- **The human surface does not scale the way the machine surface does.** 32 of
  34 services hand-build a page; 15 render an at-a-glance card. Uniformity
  underneath produces neither.

## Running the model on one host

This means **inference**, not training a foundation model. Training something
comparable to the models from Anthropic, Google, or OpenAI is a cluster-scale
research programme: data, pre-training, post-training, evaluations, safety, and
repeated failed runs. One machine can serve an open-weight model and adapt it;
it cannot reproduce that programme.

"Comparable" also needs a boundary. One host can be comparable for Mu's actual
distribution of short answers and tool calls. It will not match every frontier
model on long context, difficult coding, multimodal work, every language, and
peak concurrent traffic. The decision is therefore made against a replayable
Mu evaluation set, not a public leaderboard or a parameter count.

### What one host is

The serious version is an eight-GPU HGX-class server, not the CPU box running
the Go binary. Eight H200s provide 1,128 GB of aggregate GPU memory; each H200
has 141 GB of HBM3e and a 700 W maximum thermal design power. GPU memory is not
ordinary RAM: weights, the attention cache, and temporary working memory must
fit there, and crossing GPUs makes the interconnect part of the machine.

That is enough to serve a strong 70B-class dense model without severe
quantisation, or a much larger quantised or mixture-of-experts model. It is not
a promise of quality. A model that fits may still be bad at choosing Mu tools,
and a model that produces a good token in isolation may still have poor
throughput once several agents are running.

A smaller two- or four-GPU machine is a useful evaluation and low-traffic
deployment. It is not an honest basis for claiming frontier-comparable service.
An eight-GPU machine is still one failure domain: maintenance, a driver fault,
or a failed power supply takes all model calls down.

### Cost basis

These are planning numbers, in USD, before tax, dated September 2026. They are
deliberately ranges: a physical system is quote-priced, and power, rack, and
support vary more than a precise-looking estimate suggests.

| Cost | Own one eight-GPU host | Rent one eight-GPU host |
|---|---:|---:|
| GPUs, server, NVSwitch, CPUs, RAM, local storage | $250k-$400k once | included |
| Accelerator power | 5.6 kW maximum for eight H200s | included |
| Whole-host power and cooling budget | 8-10 kW | included |
| Electricity at $0.10-$0.25/kWh and PUE 1.2-1.5 | about $700-$2,700/month | included |
| Rack, network, spares, support | site-specific | included to the provider's terms |
| Current cloud reference | — | eight H100s at $3.99/GPU-hour: $31.92/hour, about $23.3k per 30-day month |

The cloud line is a reference rather than an H200 equivalence. Lambda publishes
that on-demand H100 SXM rate, while availability and reserved pricing change.
At full utilisation, owning can amortise below renting. At low or uncertain
utilisation, APIs win because idle GPUs cost nothing. Hardware also has an
opportunity cost: money is committed before the workload and useful life are
known.

API comparison must use a measured request shape. At Anthropic's published
Sonnet 5 price of $2 per million input tokens and $10 per million output tokens,
a run with 10,000 input and 2,000 output tokens costs $0.04 before caching or
tool-provider costs. $23.3k buys roughly 582,000 such runs in a month. If Mu is
well below that volume, renting an always-on eight-GPU host does not save model
cost; if it is above it and can keep the host busy, the calculation can reverse.
The same calculation must be repeated from Mu's usage log because input/output
ratio, cache hits, batch traffic, and model tier change the answer.

Sources for the assumptions: [NVIDIA H200 specifications](https://www.nvidia.com/en-us/data-center/h200/),
[AWS's eight-H200 instance memory and interconnect description](https://aws.amazon.com/ec2/instance-types/p5/),
[Lambda GPU Cloud pricing](https://lambda.ai/service/gpu-cloud), and
[Anthropic model pricing](https://docs.anthropic.com/en/docs/about-claude/pricing).

### Benefits

- Prompts, retrieved private records, tool results, and model outputs can remain
  on the operator's machine. This benefit disappears for any request routed to
  an upstream fallback.
- The weights, system prompt, decoding, retention, upgrade date, and availability
  policy are under one operator's control. A vendor cannot silently move an
  alias or withdraw a model.
- A busy, batchable workload turns a fixed machine into a predictable marginal
  cost, and local retrieval avoids sending large private contexts over the
  network.
- Mu can fine-tune or preference-train for its own tool grammar and failure
  modes rather than optimising for a general chat benchmark.

### Costs and disadvantages

- Quality is no longer bought as a service. Model selection, serving, drivers,
  quantisation, evaluation, abuse handling, upgrades, and incident response all
  become our work.
- Capacity is a ceiling. Interactive latency and background jobs compete for
  the same GPUs; a traffic spike queues rather than borrowing a vendor's fleet.
- One host has neither redundancy nor a maintenance window without downtime.
  A second host is the first real high-availability feature and almost doubles
  the fixed cost.
- Open weights provide control, not automatically permission for every use.
  Each model's licence, acceptable-use terms, and redistribution conditions
  still need review.
- Quantisation saves memory and often throughput cost, but can damage exactly
  the rare tool-choice and structured-output cases that matter. It must be
  evaluated, not assumed harmless.
- Privacy shifts rather than becoming automatic: logs, traces, swap, crash
  dumps, backups, administrators, and the tools called by the model remain data
  paths to secure.

### How to reach comparable product performance

1. **Define the target from production.** Build a redacted, consented replay set
   stratified by client, agent, tool, context length, language, and destructive
   action. Record answer quality, correct tool and arguments, unsupported claims,
   policy failures, time to first token, total latency, and cost. Keep destructive
   calls in a simulator.
2. **Rent before buying.** Run the candidate on one eight-GPU node for 30 days.
   Shadow traffic first, then opt-in traffic. This discovers memory, concurrency,
   and tokens-per-second requirements without making the hardware decision the
   model decision.
3. **Choose on the Mu set.** Test several current open-weight models at BF16/FP8
   and conservative quantisations. Reject any candidate that cannot reliably
   emit the native tool calls and schemas used by `runNative`, regardless of its
   leaderboard score.
4. **Optimise the serving path.** Use a production continuous-batching server,
   prefix/prompt caching, paged attention, bounded contexts, admission control,
   and separate interactive from batch queues. Quantise only after establishing
   the unquantised quality baseline. Speculative decoding is useful only when
   measurements show its draft model repays the extra memory.
5. **Train the narrow gap.** Curate failed Mu traces, correct them, and use
   supervised fine-tuning or preference optimisation for tool selection,
   argument construction, refusal, and house style. Do not try to pre-train a
   general model. Re-run held-out and destructive-action evaluations after every
   model, prompt, runtime, or quantisation change.
6. **Make the limit visible.** Set concurrency and latency objectives, queue
   background work, shed load cleanly, and return an error when the model is
   unavailable. An upstream escape hatch is a separate product mode because it
   changes the privacy claim; it must be explicit per operator and per account,
   never a silent fallback.

### Decision

Do not buy the host yet. First instrument input tokens, output tokens, cacheable
prefixes, concurrency, latency, and vendor spend by workload; then conduct the
rented-node evaluation above. Buy only when the candidate clears the Mu quality
and latency gates, observed steady demand can keep it usefully occupied, and
the three-year owned cost including power, support, labour, and a downtime plan
beats the measured API alternative.

That experiment buys the information the capital decision needs. If the local
model clears ordinary Mu work but not the hardest tail, the honest outcomes are
either a local-only product with a stated capability boundary or an explicit
hybrid mode with a weaker privacy boundary. Calling a silent frontier fallback
"self-hosted" would obtain the benchmark result by giving up the reason to run
the model.

## The order of work

1. **Prove federation against a real server.** The code is written; what is
   missing is a handshake with somebody else's deployment, and presence.
2. **Voice as the sixth protocol.** SIP and Jingle, signalling in the chat
   service where it belongs. TURN relay is the piece worth buying rather than
   hosting. Not voice *with an agent* — that is a different thing and not now.
3. **One good client.** A PWA that speaks all of it, so there is something to
   hand somebody who does not want six apps. Distribution without an app store,
   because nothing here needs one.
4. **The human surface.** Fewer bespoke pages, more services reducing to a card
   and a derived page.

## Why it might work

The valuable protocols were all open, and the companies that beat them did it by
closing a door they had been walking through. That is available to any operator
large enough, so intentions are not a defence — the defence is that leaving is
cheap. Portable identity, real export, your own domain. Email survived because a
domain moves.

Which is the actual claim: not that this is nicer, but that it is built so that
the usual move does not pay.
