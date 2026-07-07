# Operator charter — Mu

The OPERATIONS lens of Mu's autonomous loop (alongside product, marketing, and
security; see `internal/docs/CONTINUOUS_IMPROVEMENT.md`). Product decides what to
build, the increment loop builds it, security guards the invariants — the
operator **runs and releases** it, and feeds what it learns in production back
into development.

Nothing in this document is novel. It is ordinary continuous delivery and SRE:
canary releases, the four golden signals, SLOs and error budgets, synthetic
monitoring, incident-to-backlog, and upstreaming a fix to a dependency. The only
change from a normal software org is the **actor**: an agent fills the role a
release engineer or on-call SRE would, following the same process a competent
team already follows. We are automating the human aspects of dev and ops — not
inventing a new paradigm.

## Where this sits: the app runs; the library is developed against it

Two codebases, two different jobs — this is the standard library/application
split:

- **go-micro** is a *library*. You develop a library against tests and
  conformance. You do **not** put deployment, canarying, or production-watching
  *inside* it — a framework must not depend on one consumer's deploy target,
  metrics backend, or traffic. It stays general and host-agnostic, and only ever
  *receives* changes as reviewed pull requests.
- **Mu** is the *application/deployment* built on that library. It is the only
  place real usage signal exists — actual latency, errors, cost, traffic shapes,
  provider flakiness.
- **The operator** is the release/SRE function that owns *deploy → observe →
  decide*. It belongs with the application, not the library. It communicates with
  both repos through the two normal channels only: **pull requests** to change
  things, **metrics/logs** to observe them.

(The organism/genome analogy is a nice shorthand — Mu is the running organism,
go-micro the genome — but the engineering is just: run the app, learn from
production, send changes back as PRs.)

Consequence — signal flows one way, changes flow back as PRs: you learn in the
running app; you never let the app mutate the library at runtime. When production
reveals a *library* deficiency, it is **upstreamed** the ordinary way a
downstream team upstreams a framework bug: reproduce it, generalize the fix so it
helps *any* go-micro user (not a Mu-specific patch), open a reviewed PR. A change
motivated by Mu's specific phenotype that only helps Mu stays in the Mu repo.

## The fitness function: SLOs and the four golden signals

Replace "did CI pass" (a correctness test) with "did the deployed change stay
within our service objectives" (a runtime test). This is textbook SRE.

Mu already exposes `/status` (`internal/app/status.go`) — per-service up/down plus
client-measured latency. The operator needs one level up: an aggregated,
server-side **digest** of the four golden signals, per service and per agent tool:

- **Latency** — p50/p95/p99 per tool/endpoint.
- **Traffic** — requests/sec, task and tool-call counts.
- **Errors** — error rate by tool and by upstream provider.
- **Saturation** — memory, CPU, goroutines (go-micro already emits OTel; this is
  distilling it, not adding it).

Plus two Mu-specific signals: **cost per task** (LLM + paid tool spend) and
**provider degradations** (which upstreams are slow/down).

These become **SLOs with error budgets** (e.g. p99 of the agent answer < N s at
99.5%, tool error rate < 1%). "Did the canary stay within budget over the bake
window" is the promotion test.

Interface: a machine-readable digest the operator reads — extend `/status` into an
`/internal/health` (auth-gated) endpoint or a written report file. Keep it a
**digest**, not a raw trace firehose; the operator reasons over golden signals,
not spans.

## The pipeline: deploy → observe → decide (progressive delivery)

Morph `deploy.yml` into `operator.yml`. Today `deploy.yml` is push-to-main → SSH →
`git pull` → `go install` → `systemctl restart` on a persistent host with
zero-downtime socket activation. The persistent target already exists — good.
Progressive delivery adds three steps around it:

1. **Gate** on CI (unchanged — correctness first).
2. **Deploy to a canary** beside stable: a second systemd unit / socket on the
   same host (or a bounded % of traffic), not a straight restart of stable.
3. **Observe** for a bake window: run the exerciser and read the digest at *both*
   canary and stable.
4. **Decide**: within SLO / error budget → **promote** the canary to stable;
   breach → **auto-rollback** to last-known-good. The rollback path is a
   **threshold, not agent judgment** — deterministic and fast.

This is a canary release with automated rollback. Keep last-known-good hot for
instant revert.

## Synthetic monitoring: the exerciser

Standard black-box / synthetic monitoring, and non-optional here: early real
traffic is thin, so live signal alone is too weak to judge a release. Run a fixed
battery of **representative tasks** continuously against running Mu (and against
the canary during a release): a guest brief, a mail read, a market query, a
search, a paid tool call. Two purposes:

- Always-on signal feeding the digest, even before there are users.
- The comparison harness for canary vs stable during a release.

This is the "live stress test" — a smoke/synthetic suite exercising the real
deployment, the same way an ops team runs synthetic probes against production.

Built: `internal/exercise` + the `mu exercise` command. It drives a fixed battery
of representative requests at a target (`mu exercise --target <url>`), repeats for
latency percentiles (`--runs N`), optionally exercises the agent/LLM path
(`--deep`), and prints a JSON report that exits non-zero on any failure — so CI or
the operator can gate, and a canary-vs-stable check is a diff of two reports. The
default battery is cheap public GETs (no LLM cost); `--deep` adds a guest agent
query (token cost, rate-limited) for occasional canary checks.

Note — same primitive, two consumers: this synthetic load driver is the same idea
go-micro's own loop needs for an in-harness operator (below). One drives Mu's HTTP
endpoints; the other would drive go-micro RPCs. Reuse the concept, not necessarily
the code.

## Ops → dev handoff: incidents become backlog

The operator is on-call. When the digest shows a regression, or the exerciser
starts failing, it does exactly what on-call does: open a ticket with evidence —
which signal moved, which service/tool, and since which change/deploy — and drop
it into the existing product→increment queue (`internal/docs/PRIORITIES.md`).
That is the production signal entering development, through the ordinary
incident-to-backlog path, not anything exotic. A short postmortem note on
recurring regressions keeps the queue honest.

## Upstreaming to go-micro

When the evidence points at the *library* rather than the app, follow the normal
downstream→upstream contribution path:

1. Reproduce against go-micro directly (isolate from Mu).
2. **Generalize** — a fix that helps any go-micro user, not a Mu-shaped hack.
3. Open a reviewed PR to go-micro with the evidence; a maintainer (human or the
   go-micro architect loop) reviews it against that repo's tests/conformance.

Start this faculty as *"file an evidenced issue,"* and graduate to *"open a PR"*
only once digest→hypothesis quality is proven. **Germline writes are always
human-reviewed** — you do not autonomously change a library other projects depend
on. This is the single most important discipline: production experience may
*inform* the library, but only disciplined, generalized, reviewed changes are
allowed to land in it.

## Two operators, at two altitudes

The persistent-deployment requirement above is an *application* property, not a
*library* one. Mu (the app) needs a live phenotype because its fitness — real
traffic, cost, emergent failures over days — is not reproducible. go-micro (the
library) is different: its runtime fitness — throughput, RPC p50/p99, allocs/op,
goroutine leaks, memory growth under sustained load, races under concurrency — is
largely **reproducible**, so it can be measured in an **ephemeral harness** with
no deployment at all. That is the right place for a go-micro operator:

- **go-micro operator (in-harness):** a bench/soak/race regression gate in
  go-micro's own loop — spin up example services + a synthetic load driver in CI,
  measure the golden signals against the previous commit's baseline, and gate or
  file a regression. No persistent target needed; the harness *is* the
  environment. Fitness = bench deltas, not just "tests pass."
- **Mu operator (over deployment):** this document — canary + live golden signals
  over the running organism, for the emergent signal a harness can't reproduce.

They connect: **Mu is go-micro's live soak test.** The `/internal/health` digest
is, from go-micro's point of view, a second and richer fitness signal — the
dogfooding organism running the genome in production, feeding regressions upstream
via the reviewed-PR channel above. The harness catches reproducible regressions
pre-merge; Mu catches emergent ones post-deploy. Two fitness functions, one at
each altitude, neither leaking into the other.

## Guardrails — the most dangerous role, same discipline as security

Autonomous deploy earns the treatment we gave the security lens:

- **Canary-only** — never straight to stable.
- **Automated rollback** on SLO / error-budget breach; the rollback trigger is a
  threshold, never an agent's judgment call.
- **Human gate / change freeze** on: public contracts (MCP/A2A/REST/webhooks/env
  vars), schema or data migrations, spend/pricing changes, and any go-micro
  (library) write.
- **Blast radius** — one change per bake window; bounded canary traffic;
  keep-last-known-good for instant revert.
- **Auditable** — every change is a git commit / PR; secrets stay server-side and
  never surface in a digest, log, or report.
- When a signal is **ambiguous, prefer rollback** over a forward-fix.

## Phased rollout — build the signal first

The operator is blind without the fitness signal, so build that before any
automation:

1. **Runtime fitness report** — DONE (`internal/metrics`, `/internal/health`):
   golden signals per tool and LLM caller, saturation, cost, gated to an admin
   session or `HEALTH_TOKEN` bearer.
2. **Exerciser** — DONE (`internal/exercise`, `mu exercise`): synthetic battery
   with latency percentiles and a pass/fail report for canary comparison.
3. **`operator.yml` v0** — canary deploy + bake + compare + promote/rollback +
   report. No self-modification yet; just safe releases with a measured decision.
4. **Incident faculty** — digest/exerciser regressions → issues into
   `PRIORITIES.md`.
5. **Upstream faculty (human-gated)** — library-attributed findings → evidenced
   go-micro issues, then (once proven) PRs.

Steps 1–3 are the buildable core and make releases safe on their own merits.
Step 4 closes the loop from production back into development. Step 5 lets
production experience improve the library — carefully, and with a human in the
loop.

## Autonomy boundary

Within its SLO guardrails the operator may promote or roll back a canary on its
own — that *is* the job, and it is bounded by thresholds and last-known-good.
Everything beyond that — public-contract changes, migrations, spend, and library
(germline) writes — is human-gated, exactly as in `internal/docs/SECURITY.md`.
The operator's remit is to keep the running system healthy and to route what it
learns back into the loop, not to unilaterally reshape contracts or dependencies.
