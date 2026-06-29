# Mu — Thesis (North Star)

The single reference the autonomous loop aligns to. The **product** pass, the
**marketing** pass, and the **continuous-improvement** pass all derive their
work from this document and the canon it names. When they conflict, this wins;
when the human redirects, the human wins.

## Mission

**An agent for everyday.** The everyday internet — news, mail, search, weather,
video, markets — runs on services, and a handful of platforms own all of them
and sit at the centre of what you do. Mu is the alternative: one agent across all
the everyday things, each a real service, and open/self-hostable so a person can
run the whole stack themselves. No ads, no tracking, no algorithm — you pay for
the tools, not with your attention.

Built on [Go Micro](https://go-micro.dev): every capability is a go-micro
service, the assistant is a go-micro agent, the protocols (MCP, A2A) are its
gateways. Mu is also the flagship reference application that proves the framework.

## The relationship (don't blur it)

- **go-micro** is the substrate — the open runtime agents-as-services are built
  on. It is not productised directly (no hosted SaaS, no enterprise tier). Its
  roadmap lives in its own repo and its own loop.
- **Mu** is the product — the everyday agent, with real users and real revenue
  surfaces (credits, x402, self-host). This loop is about **Mu the product**.

## What "good" means right now: seamlessness, not surface area

The bias of this phase is **product-market fit through refinement**, not more
features. Mu already spans a wide surface (news, mail, markets, weather, search,
video, blog, social, agent, apps, wallet). The job is to make what already
exists work **seamlessly** — fast, correct, obvious, and reliable — for a real
person on their first visit and their hundredth.

Default to **refine over add**. A new capability needs to clear a high bar: it
either removes friction from the core loop (ask → answer) or it is something a
real user has actually reached for. When in doubt, make an existing thing work
better instead of adding a new thing.

The core loop to protect above all else: **a person asks Mu something and gets a
correct, well-formatted, fast answer** — as a guest and signed in, on web and on
the chat clients. Everything else is in service of that.

## Canon (read these; don't rely on this file alone)

- `README.md` — the product positioning and developer entry.
- `docs/VISION.md`, `docs/PRINCIPLES.md` — what Mu is and the values it refuses
  to violate (no ads/tracking/algorithm; assist, don't replace).
- `docs/WHITEPAPER.md`, `docs/SYSTEM_DESIGN.md` — the architecture and economics.
- The live product at **micro.mu** — exercise it; the lived experience is canon.

## Roadmap (Now → Next → Later)

Phase is the primary ordering for `PRIORITIES.md`; internal findings (friction,
bugs, rough edges) interleave by value.

### Now — seamlessness / PMF
- The core ask → answer loop is fast, correct, and well-formatted for **guests
  and signed-in users**, on web and on Discord/Telegram.
- Every service the agent can call actually works end to end and degrades
  gracefully when a provider is down (no dead cards, no silent failures).
- First-run experience: a new visitor understands what Mu is and gets value in
  one prompt, without an account.
- Reliability and clarity of errors over new capability.

### Next — depth where users already are
- Memory/personalisation that demonstrably improves repeat answers.
- The chat clients (Discord/Telegram/WhatsApp) at parity with web for the core loop.
- Streaming the answer token-by-token once the framework supports tool-aware
  streaming (tracked upstream as go-micro#3341).

### Later — reach
- "Run your own Mu": self-host/white-label as a real, documented path.
- Broader provider coverage and federation.

## Autonomy boundaries (what the loop may and may not do)

- **May**, unsupervised: refinements, bug fixes, test coverage, error-message and
  formatting polish, factual doc alignment, performance — anything that makes the
  existing product more seamless without changing its contract.
- **May**, on the live platform: Mu posts its own notes to its own
  blog via the in-process loop (`blog/notes.go`), grounded in canon and low
  cadence — the same autonomy the opinion/digest loops already have. Toggle with
  `NOTES=off`.
- **May not**, without the human: brand/positioning copy and taglines; breaking
  changes to public contracts (MCP tool names, A2A protocol, webhook/REST
  endpoints, env var names); pricing; large architectural rewrites; and
  publishing marketing content **off** Mu's own blog (long-form drafts are
  surfaced for the human, not auto-posted elsewhere).

When unsure, pick the smaller, safer, more reversible change — and keep `main`
green (`go build ./...`, `go test ./... -short`, `go vet ./...`).
