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
| **Chat** | XMPP over WebSocket, `jabber:client` | working, single instance |
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

- **Instances cannot talk to each other.** `service/chat` speaks `jabber:client`
  only. No S2S, nothing on 5269. Self-hosting works; federation does not exist,
  so this is one city rather than a network.
- **No voice or video.** No SIP, no Jingle, no TURN.
- **Fiat still arrives through Stripe**, with Stripe's toll.
- **The upstream rent is still paid.** Model vendors, Google, Twilio, all billed
  per request. We are one layer inside the structure, reselling with a stated
  markup — not outside it.
- **The human surface does not scale the way the machine surface does.** 32 of
  34 services hand-build a page; 15 render an at-a-glance card. Uniformity
  underneath produces neither.

## The order of work

1. **Make the protocols actually work.** XMPP first — federation is what turns
   one server into a network, and nothing else on this list matters as much.
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
