# MCP Server

Mu speaks [MCP](https://modelcontextprotocol.io) (Model Context Protocol) at a
single endpoint. That endpoint is the only way an agent talks to this instance —
there is no second API to choose between, and nothing to integrate per tool.

**Endpoint:** `POST /mcp` — Streamable HTTP transport, JSON-RPC 2.0.

## Connect

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

There is no key in that config because there is nothing to paste. Works with
Claude Desktop, Cursor, or anything else that speaks MCP.

## Teaching your agent to use this

Tool definitions say what each tool does. They cannot say how the tools compose,
which need an account, which cost money, or that a caller can have its own email
address and a store that outlives the session.

That layer ships as an [Agent
Skill](https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills)
in [`skills/mu`](https://github.com/micro/mu/tree/main/skills/mu). Drop it where
your agent reads skills from:

```bash
mkdir -p ~/.claude/skills && cp -r skills/mu ~/.claude/skills/
```

It is checked against the registry by tests, so it cannot quietly drift from
what this instance actually does.

## Scoping a connection

One endpoint carrying every tool is right for the server and wrong for a
session: the definitions go to the model on every turn whether or not any of
them could help, and a client connected for news does not need a qibla compass
in the list. Name the services you want:

```
https://micro.mu/mcp?tools=news,web,mail
```

`tools` takes service names (`news`), individual tool names (`web_search`), or
what the sidebar calls a service (`search` for `web`), comma-separated. An
unrecognised name is ignored rather than refused — a scope is a preference, and
failing the whole connection because somebody wrote `email` for `mail` would be
worse than giving them the rest.

Scoping changes what is **listed**, not what may be **called**. A tool left out
is still reachable by name, and the guards that matter — account, credits, rate
limits — are unchanged. This is about what an agent is asked to consider, which
is a context problem rather than a permission one; if you need a caller to be
unable to do something, that is what an account-scoped service and a token are
for.

There is a picker on [/tools](https://micro.mu/tools) that builds the URL.

## Authentication

Calls carry a bearer token:

```
Authorization: Bearer YOUR_TOKEN
```

Two ways your client comes to have one.

### Sign-in from the client (recommended)

Call without a token and you get an `HTTP 401` naming this instance's
authorization server:

```
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer resource_metadata="https://micro.mu/.well-known/oauth-protected-resource"
```

A client implementing [MCP
authorization](https://modelcontextprotocol.io/specification/basic/authorization)
follows that to the metadata, registers itself dynamically, opens a browser for
you to sign in, and stores the token itself. You paste nothing and the token
never passes through a config file. Claude Desktop and Cursor both do this.

The discovery documents are served at:

- `/.well-known/oauth-protected-resource` — what this resource is and who
  authorizes for it
- `/.well-known/oauth-authorization-server` — endpoints, grant types, and the
  dynamic client registration URL

### Personal access token

For a client that doesn't speak the authorization spec, or for scripts and the
CLI: sign in at `/login`, create a token at `/token`, and put it in the config.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp",
      "headers": { "Authorization": "Bearer YOUR_TOKEN" }
    }
  }
}
```

There is no `signup` or `login` tool, by design. Creating an account and
exchanging a password for a session are not capabilities a caller can be granted
— they are how a caller comes to exist, and they happen at the web boundary
where a person is present.

## What calls cost

A call costs credits when it costs the instance money to run: a model call, or a
paid third party. Everything that only touches this instance's own storage —
reading news, markets, weather, blogs, videos, your own records — is included.

Prices are per tool. The live list, always current, is at
[/tools](https://micro.mu/tools); each tool's page on
[/mcp](https://micro.mu/mcp) carries its schema, an example request and a
playground. There is no table of them here because a copied one goes stale.

Credits are prepaid against your account and topped up by card. It is the same
balance whether the call comes from your agent or from you using the app.

A few tools that touch your account — wallet, mail, editing your apps — always
need a signed-in caller, whatever the price. `mail_send` is the strict case: it
is account-only, so an unaccountable caller can't spend the domain's reputation.

## Protocol

### Initialize

```bash
curl -X POST https://micro.mu/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"example","version":"1.0"},"capabilities":{}}}'
```

### List tools

```bash
curl -X POST https://micro.mu/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
```

### Call a tool

```bash
curl -X POST https://micro.mu/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest AI news"}}}'
```

## Your data (`db_*`)

The `db_*` tools give an agent a personal database — the same collections and
private/public model as the app SDK's `mu.db`, scoped to your account. Store
records in named collections, keep them private or share them publicly, and
query with a `where` filter (`eq`, `ne`, `gt`/`gte`/`lt`/`lte`, `contains`, `in`,
`exists`). A collection is made on first write; there is no schema to declare.

It is a service like any other, which means two things worth knowing. You can
see what has been stored at [/db](https://micro.mu/db) — the same records, as a
page. And an agent token scoped at [/agents](https://micro.mu/agents) can be
granted `db` and nothing else, so the thing you hand a credential to can keep
records without also reaching your mail.

```bash
# Store a private record
curl -X POST https://micro.mu/mcp \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"db_create","arguments":{"collection":"tasks","data":{"title":"Ship it","done":false}}}}'

# List your open tasks
curl -X POST https://micro.mu/mcp \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"db_list","arguments":{"collection":"tasks","where":{"done":false}}}}'
```

The owner is bound from your session, so records are isolated per account —
another user can't read or change your private records, and no request carries an
account field to forge. Data written through the `db_*` tools lives in your
account's namespace, separate from any app's own data.

## Self-hosting

The MCP endpoint is available automatically at `/mcp` with no extra
configuration. Set `MU_DOMAIN` to your public hostname so the authorization
metadata advertises the right issuer — behind a proxy it is read from
`X-Forwarded-Host`, and falls back to the request host. See
[Configuration](ENVIRONMENT_VARIABLES.md).
