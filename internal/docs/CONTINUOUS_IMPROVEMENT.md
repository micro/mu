# Continuous Improvement

Mu runs an autonomous loop driven by Codex. Scheduled GitHub Actions each open a
fresh tracking issue and dispatch Codex with a role; Codex does the work and (for
safe changes) opens a PR with auto-merge, so safe changes land when CI is green.
All are gated on the `CODEX_TRIGGER_TOKEN` secret (a PAT for a user account Codex
follows — Codex ignores comments from the github-actions bot), and all align to
the North Star in [THESIS.md](THESIS.md).

## The agents

The shape mirrors go-micro's loop, recast for **Mu the product**:

| Pass | Role | Cadence | Output |
|------|------|---------|--------|
| **product-review** (`.github/workflows/product-review.yml`) | Head of product — *where to next, what to refine* | hourly `:59` | maintains the ranked queue in [PRIORITIES.md](PRIORITIES.md) + an assessment |
| **continuous-improvement** (`.github/workflows/continuous-improvement.yml`) | Builder — *ship one increment* | hourly `:29` | a PR that builds the top open item in PRIORITIES.md |
| **marketing-review** (`.github/workflows/marketing-review.yml`) | Marketing — *tell the story* | daily `07:00 UTC` | public-surface coherence fixes + blog drafts (per [MARKETING.md](MARKETING.md)) |
| **security-review** (`.github/workflows/security-review.yml`) | Security — *nothing like this again* | daily `03:00 UTC` | audits the agent tool/auth/wallet surface against [SECURITY.md](SECURITY.md); reports + files findings |
| **operator** (spec — [OPERATOR.md](OPERATOR.md)) | Operations/SRE — *run it, release it safely, learn from prod* | on deploy (planned) | canary deploy + golden-signal digest → promote/rollback; routes production signal back into the queue |

The security pass is the odd one out on autonomy: it **does not auto-merge**
code. It reports findings and files scoped issues; a clearly-correct, CI-verified
hardening may open a PR, but anything touching auth, wallet, or identity binding
is left for a human. Its job is to keep the core invariant true — the model can
never decide whose data is used or how the user's funds are spent.

The **operator** is the ops/SRE lens (design in [OPERATOR.md](OPERATOR.md), not
yet wired). Where the others evolve the source against synthetic tests, the
operator closes the loop on the *running* system: it turns `deploy.yml` into
progressive delivery (canary + golden-signal digest + automated rollback), runs a
synthetic exerciser for live signal, and feeds production regressions back into
the queue — the ordinary incident-to-backlog path. It also carries the disciplined
channel for upstreaming library-level findings to go-micro (generalized,
human-reviewed). Same autonomy boundary as security: it may promote/rollback a
canary within SLO guardrails, but contracts, migrations, spend, and any go-micro
write are human-gated.

So the **product agent decides what**, the **increment loop builds it**, and the
**marketing agent keeps the outside world coherent and supplied with things worth
writing about**. The product pass runs just before the increment pass each hour so
the builder always works against a fresh queue.

This phase's bias (see THESIS.md): **product-market fit through refinement** —
make what already exists work seamlessly before adding surface area.

## How a build increment works

1. The hourly action opens a fresh issue (unique → unique `codex/` branch).
2. It posts an `@codex` comment pointing at PRIORITIES.md.
3. Codex takes the highest-ranked item whose issue is still open (or falls back
   to the charter categories below), implements it, runs the checks, opens a PR.
4. GitHub auto-merge lands the PR when CI is green; `Closes #<issue>` removes the
   item from the queue.

## What Codex Should Work On

The queue in [PRIORITIES.md](PRIORITIES.md) is the source of truth — the increment
loop builds the top item whose issue is still open. The categories below are the
**fallback** ordering for when the queue is empty or fully closed: pick the single
highest-value item.

### 1. Bug Fixes
- Fix any failing tests
- Fix known issues in the issue tracker
- Fix error handling gaps

### 2. Test Coverage
- Add tests for untested packages
- Add edge case tests for critical paths (agent routing, trade execution, auth)

### 3. Code Quality
- Remove dead code and unused imports
- Fix inconsistencies (error handling patterns, logging)
- Simplify complex functions

### 4. Feature Improvements
- Improve agent routing accuracy
- Better error messages for users
- Performance improvements (caching, reducing API calls)
- Improve documentation

### 5. Infrastructure
- Improve CI pipeline
- Add linting rules
- Improve build times

## Rules

- **One concern per PR** — don't bundle unrelated changes
- **Don't touch brand/positioning** — no changes to taglines, descriptions, marketing copy
- **Don't break public API** — MCP tool names, A2A protocol, webhook endpoints stay stable
- **Don't change env var names** — existing deployments depend on them
- **Run all checks** — `go build ./...`, `go test ./...`, `go vet ./...`
- **Keep PRs small** — under 200 lines of change is ideal
- **Write clear commit messages** — explain what and why
