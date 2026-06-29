# Marketing & Evangelism

The charter for Mu's **marketing-review** pass (the *marketing agent*) — the
public-story half of the autonomous loop. Where the product agent decides what to
build and the increment loop builds it, this pass makes sure the outside world can
understand Mu, that the public surface stays coherent with what actually shipped,
and that there is a steady supply of things worth writing about — grounded in real
shipped work and real usage, not hype.

North Star: [THESIS.md](THESIS.md). Voice and values: `docs/PRINCIPLES.md`.

## Where content publishes: the live blog (dogfood)

Mu markets itself **on Mu**. Evangelism content is posted to Mu's own blog by an
in-process loop (`blog/evangelism.go`) running on the live platform — the system
account writes a short, canon-grounded piece about Mu on a low cadence, the same
way the opinion and digest loops already post. Actual content, actual usage. No
external token, no staging: it runs wherever Mu runs.

That loop is grounded strictly in a `muFacts` block and a set of `angles` so it
states only what is true and never invents features. Keeping those in lockstep
with the canon (README, VISION, PRINCIPLES, THESIS) is the marketing pass's main
ongoing code job.

## What the marketing pass does each run

1. **Coherence.** Audit the public surface — `README.md`, the docs under `docs/`
   (served at `/docs`), and the landing/home copy (`home/landing.go`,
   `home/home.go`, `home/pricing.go`, `home/assistant.go`) — for places where
   they contradict each other, are stale, or describe behaviour that has since
   changed. Cross-check against the code and recently merged PRs.
2. **Keep the live voice accurate.** Check `blog/evangelism.go` — do `muFacts`
   and `angles` still match the canon and recently shipped work? If the product
   changed (new capability, changed positioning), update them so the live posts
   stay true. This is a safe, CI-verifiable code change and may auto-merge.
3. **Find the story.** From recently shipped work and how the live product is
   actually used (micro.mu), identify 1-3 genuinely true, concrete things worth
   saying — either as a new evangelism angle (added to `evangelism.go`) or, for
   long-form, a Markdown draft under `internal/docs/blog/` for a human to post.
4. **Report.** Post a concise findings comment on the dispatch issue: what is
   aligned, what drifted, what was fixed/updated, and any angles or drafts added.

## Voice

- Plain and concrete. Show the thing working; quote real output.
- Honest about limitations. We assist, we don't oversell. No dark patterns in
  copy any more than in product.
- Lead with what a person gets, not with technology — though the go-micro /
  ownership story is a real differentiator and worth telling on its own.

## Autonomy boundary

- **Auto-merge (safe):** factual-alignment and crispness fixes to README/docs;
  and keeping `blog/evangelism.go` (`muFacts`, `angles`) accurate to the canon —
  these make the live voice more correct without changing positioning. Verify
  build/test when code is touched.
- **The live evangelism loop publishes autonomously** to Mu's own blog, like the
  opinion/digest loops — that is intended (it is grounded in canon and low
  cadence). It can be turned off entirely with `EVANGELISM=off`.
- **Human only (never auto-merge):** brand/positioning copy and taglines; pricing;
  and long-form marketing drafts under `internal/docs/blog/` (surfaced in the
  report for a human to post). The human owns positioning and anything off-blog.
