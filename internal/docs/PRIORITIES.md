# Priorities

The ranked product work queue for Mu's autonomous loop. The **product-review**
pass (the *product agent*) owns this file: each run it turns the North Star
([THESIS.md](THESIS.md)) plus a hands-on look at the live product into a single
ordered list — highest-value first — and links each item to a tracking issue. The
hourly **continuous-improvement** pass works the **top item whose issue is still
open**. So the product agent decides *what*, and the increment loop *builds* it.

**Reading / editing.** An item is done when its linked issue closes (the increment
that builds it adds `Closes #<issue>`). Roadmap phase (Now → Next → Later in
THESIS.md) is the primary ordering — and this phase is **Now: seamlessness**, so
refinements that make the existing product work better rank above new surface
area. The human can reorder this list — or the issues — at any time to redirect
the loop; direction always wins.

**Off-limits to the loop** (the product agent proposes these as notes, never as
queue items the loop can auto-merge): brand/positioning copy, pricing, breaking
public-contract changes (MCP/A2A/REST/webhooks/env vars), architectural rewrites,
and publishing marketing content. Those go to the human.

## Work queue (ranked)

> Seeded from the North Star. The product-review pass files a tracking issue for
> each unlinked item on its first run (`gh issue create --label codex`) and links
> it here; until then the continuous-improvement loop falls back to picking the
> highest-value item itself.

1. **Harden the core ask → answer loop end to end.** A guest and a signed-in user
   should get a correct, well-formatted, fast answer on web and on the chat
   clients. Add integration/smoke coverage of the ask → tool → answer path across
   the core services (weather, news, markets, mail, search) so regressions in the
   most important flow are caught.
2. **Every service degrades gracefully.** Audit each home card and each
   agent-callable service for the provider-down case — no dead cards, no silent
   failures, a clear "unavailable" instead. One service per increment.
3. **First-run experience.** A new visitor understands what Mu is and gets value
   from one prompt without an account — tighten the guest landing, suggestions,
   and the sign-up moment (when the free limit is hit) for clarity, not friction.
4. **Answer formatting quality.** Rendered answers (news, markets, weather) look
   right everywhere they appear — web (guest + signed-in), Discord, Telegram —
   with consistent spacing, headings, and links.

_Seeded by Claude Code from the North Star; thereafter maintained by the
product-review pass._
