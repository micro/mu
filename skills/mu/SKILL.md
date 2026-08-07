---
name: mu
description: Use when working with a Mu instance over MCP — a server giving agents mail, calendar, contacts, tasks, files, records, news, markets, places, web search and app hosting behind one endpoint. Covers connecting and authenticating, which calls need an account, which cost credits, and the two capabilities agents usually miss: your own email address, and storage that survives the session.
---

# Working with Mu

Mu is one MCP endpoint carrying every tool an instance offers. There is no
second API and nothing to integrate per tool: `tools/list` is authoritative,
always current, and the right thing to read before assuming what exists.

The reference instance is `https://micro.mu`. Mu is a single self-hostable
binary, so substitute whatever host you were pointed at — everything here holds
for any instance.

## Before the first call

Three facts decide whether your first calls work.

**Scope the connection.** Every tool definition goes to the model on every turn.
An instance carries dozens across ~30 services, and a session about news does
not need a qibla compass in its context. Name what you want:

```
https://micro.mu/mcp?tools=news,mail,web
```

Takes service names (`news`), tool names (`web_search`), or the label the UI
uses (`search` for `web`). Unknown names are ignored rather than refused.

This changes what is **listed**, not what may be **called** — it is a context
decision, not a permission one. Permissions are the next two.

**Some tools need an account.** Anything holding one person's data or spending
their money refuses an anonymous caller: `mail`, `contacts`, `events`, `tasks`,
`files`, `db`, `index`, `wallet`, `saved`. They answer

```json
{"error":"authentication required"}
```

Note the shape — a plain body, not a JSON-RPC `error` object, so a client that
only inspects `result` will read it as a successful empty answer. Check for it.

**Some calls cost credits.** A call costs when it costs the instance money: a
model call, or a paid third party (web search, image generation, places). Calls
that only touch the instance's own storage — news, markets, weather, blog,
video, your own records — are included. Prices are per tool and live at `/tools`.

See `reference/connecting.md` for authentication, including how a client gets a
token without anyone pasting one.

## The two things agents miss

Most of Mu's surface is what you would guess. These two are not, and they change
what an agent can be:

**You can have your own email address.** Mu runs a real SMTP server with DKIM,
and `mail_address` returns a plus-address you can hand out:
`owner+research@instance`. Mail sent there lands in the owner's inbox tagged for
you, and `mail_inbox` with the same tag returns only your own. An agent that can
be *written to* is a different kind of thing from one that can only be called —
it can be sent a receipt, a form, a reply, and woken by any of them.

Inbound is deliberately strict: replies to mail that was sent, addresses the
account has written to, and known product domains. Everything else is refused at
the door. So an address you hand to a stranger will not receive until you have
written to them first.

**You have storage that outlives the session.** `db_*` is a per-account record
store — `db_create`, `db_get`, `db_list`, `db_delete` over named collections,
with a `public` flag per record. `files_*` holds bytes and hands back a URL.
Nothing about either is temporary. An agent that keeps notes across runs, or
builds up a dataset over weeks, does it here rather than re-deriving it.

## Asking Mu rather than calling it

`agent_ask` hands a whole task to the instance's own agent: it plans, calls
several tools, and returns a written answer. Worth it when the task needs
several calls whose shape you cannot predict.

It is the wrong tool when you already know which tool answers — you would be
spending a model call to reach something you could have called directly. It is
also the wrong tool for looking something up cheaply; for that, search the
instance's own content first (`index_search`, `news_search`) before paying for
`web_search`.

## Reference

- `reference/connecting.md` — endpoint, authentication, scoping, costs
- `reference/tools.md` — the surface by service, and what each is for
- `reference/recipes.md` — combinations worth knowing

## What Mu does not do

Mu declares only the `tools` capability. There are no MCP resources or prompts,
so do not probe for them.

There is no `signup` or `login` tool, deliberately: creating an account is how a
caller comes to exist, not something a caller can be granted. It happens at the
web boundary where a person is present.

Skills that bundle executable scripts have nothing to run them here — Mu's agent
has no shell. Code belongs in an app (`apps_create`), which the instance hosts
and runs in the browser sandbox.
