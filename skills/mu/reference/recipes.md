# Combinations worth knowing

Individually the tools are obvious. These are the joins that are not, and the
ones where reaching for the wrong tool costs money or gets the wrong answer.

## Give yourself an address, and be woken by it

This is the capability most agents never find, and it changes what an agent can
be: something that can be *written to* rather than only called.

An agent created on `/agents` is given one from its name — "Morning Briefer"
becomes `owner+morningbriefer@instance` — and it is shown on the agent's row.
Any caller can also mint one on demand:

```
mail_address {"tag": "research"}   → owner+research@instance
```

Hand that address out. Mail sent to it lands in the owner's inbox tagged
`research`, and:

```
mail_inbox {"tag": "research"}     → only your own mail, not the owner's
```

So a run can end by asking for something, and a later run can pick up the
answer. Receipts, forms, replies, confirmations, a human saying "yes go ahead" —
all arrive as mail, and none of them need a webhook or an open connection.

**The constraint that will bite you:** inbound is strict. Delivered mail is
limited to replies to messages the account sent, addresses the account has
written to, and known product domains. Everything else is refused at SMTP. So an
address handed to a stranger receives nothing until you have written to them
first. Send before you expect to receive.

## Keep state that outlives the run

`db_*` is a per-account record store, not a cache. Use it wherever you would
otherwise re-derive something or ask the user to repeat themselves.

```
db_create {"collection":"leads","data":{"name":"...","stage":"contacted"}}
db_list   {"collection":"leads","where":{"stage":"contacted"},"sort":"created","order":"desc","limit":20}
db_get    {"collection":"leads","id":"..."}
```

Records carry a `public` flag, so the same store holds working notes and things
meant to be read. `files_*` is the same idea for bytes, and `files_share`
returns a URL you can put in a message.

A useful pattern: keep a `runs` collection with what each invocation did. It
costs nothing, and it is the difference between an agent that repeats itself and
one that continues.

## Schedule something with a person

The whole join, in the order that avoids asking the user anything they have
already said:

```
contacts_find  {"query":"sam"}                      → resolve name to address
events_free    {"minutes":60,"from":"...","to":"..."}  → open slots, not a list to diff
events_create  {"title":"...","when":"2026-08-12T15:00:00+01:00"}
mail_send      {"to":"...","subject":"...","body":"..."}
```

`events_free` exists so you never compute availability yourself from
`events_list` — it returns the stretches that fit, within working hours
(`day_start`/`day_end`, defaulting 9–18). If the account has connected Google
Calendar, external busy periods are already merged in; you cannot tell from the
call, and should not try.

Send `when` as RFC3339 with an explicit offset. A bare local time is read as
UTC, which is silently wrong by whatever the user's offset is — the worst kind
of wrong for a deadline, because nothing looks broken until something is missed.

## Answer from what is already here before paying for the open web

Cost and latency both rise as you move down this list. Stop at the first that
answers:

1. `index_search` — the caller's own content: their posts, notes, saved things
2. `news_search` / `blog_list` — this instance's own aggregated content, free
3. `web_search` → `web_fetch` — the open web, and the metered door

"What did I write about X" is `index_search`, not `web_search`. Getting this
backwards is the most common way to spend credits on an answer that was already
local.

## Hand off work that outlasts the conversation

```
tasks_create {"title":"...","detail":"...","assignee":"agent","due":"..."}
```

`assignee` is `me` (default) or `agent`. Set it to `agent` and the instance's own
agent picks it up and works on it — this is the point of the field.

From the other side, an agent asking what to do next:

```
tasks_next                          → the next open task assigned to the agent
tasks_update {"id":"...","status":"doing"}
tasks_update {"id":"...","status":"done","result":"..."}
```

Put the outcome in `result`. It is what the person reads to find out what
happened, and what a later run reads to know it is finished.

## Publish something

```
files_put    {"name":"report.md","content":"..."}    → id
files_share  {"id":"...","public":true}              → URL
blog_create  {"title":"...","content":"..."}         → a post
stream_post  {"content":"..."}                       → the instance timeline
```

`blog_read` and `blog_delete` take a `title` as well as an `id`, resolved exact
→ unique prefix → unique substring. An ambiguous title returns nothing rather
than guessing — so a delete by title either hits the right post or does nothing,
never the wrong one.

## Ship a small program instead of describing one

Mu has no shell and no server-side execution, so an agent cannot run a script
here. What it can do is create an app, which the instance hosts and runs in the
browser sandbox:

```
apps_build  {"prompt":"a unit converter for temperature and weight"}
apps_create {"name":"...","slug":"...","description":"...","html":"..."}
apps_run    {"code":"..."}      → a throwaway sandbox, no app created
apps_fork   {"slug":"...","new_slug":"..."}
```

Apps can be priced and forked. `apps_run` is the right tool for "show me this
working once"; `apps_create` for something meant to persist.

## When to hand the whole thing over

`agent_ask` plans and calls tools on the instance side. It is worth it when the
task needs several calls whose shape you cannot predict, and wasteful when you
already know which tool answers — you are paying for a model call to reach a
tool you could have called directly.

There is no cheaper model-backed sibling. If the question is "what is already
here", that is a search, not a model call: `index_search` for the caller's own
content and `news_search` for the instance's, both free.
