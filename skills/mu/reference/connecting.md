# Connecting to a Mu instance

## Endpoint

`POST /mcp` — Streamable HTTP transport, JSON-RPC 2.0. One endpoint, every tool.

```json
{
  "mcpServers": {
    "mu": { "url": "https://micro.mu/mcp" }
  }
}
```

There is no key in that config because there is nothing to paste — see
authentication below.

`initialize` reports `{"capabilities":{"tools":{}}}`. Tools only: no resources,
no prompts, no sampling.

## Authentication

Calls carry a bearer token:

```
Authorization: Bearer YOUR_TOKEN
```

There are two ways a client comes to have one.

### Sign-in from the client (preferred)

Call without a token and the instance answers `HTTP 401` naming its
authorization server:

```
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer resource_metadata="https://micro.mu/.well-known/oauth-protected-resource"
```

A client implementing MCP authorization follows that to the metadata, registers
itself dynamically, opens a browser for the user to sign in, and stores the
token itself. Nothing is pasted and the token never enters a config file. Claude
Desktop and Cursor both do this.

Discovery documents:

- `/.well-known/oauth-protected-resource` — what this resource is, who authorizes
- `/.well-known/oauth-authorization-server` — endpoints, grant types, dynamic
  client registration

### Personal access token

For a client that does not speak the authorization spec, or for scripts: the
owner signs in at `/login`, creates a token at `/token`, and puts it in the
config.

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

### Tokens can be narrower than the account

A token minted at `/agents` carries a **scope**: the services that agent may
reach. Calls to anything outside it are refused even though the owner's account
is behind the credential. A token with no scope reaches everything the owner
can, which is a choice rather than a default.

This is the difference that matters: `?tools=` changes what is listed, a scoped
token changes what is permitted. If a caller must be *unable* to do something,
it needs a scoped token, not a filtered URL.

## Recognising a refusal

Two different refusals, and they mean different things.

**No account, on a tool that needs one.** `HTTP 401`, with the header that tells
a client where to sign in:

```
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer resource_metadata="https://micro.mu/.well-known/oauth-protected-resource"
{"error":"authentication required"}
```

This is the MCP authorization handshake, not an error to report to the user — a
client that implements it should follow the metadata and get a token. The body
is a plain object rather than a JSON-RPC `error`, because the refusal happens
before the JSON-RPC layer is reached.

**No account, on a metered tool.** `HTTP 200` with a JSON-RPC `error`, saying
the call is metered and there is nobody to charge. Do **not** start an OAuth
flow on this one: signing in is only one of the two answers, and the other is to
send an x402 payment.

So: check the status code first. 401 means get a credential; a JSON-RPC error
means read what it says.

## What needs an account

Refuse anonymous callers outright:

`mail_*`, `contacts_*`, `events_*`, `tasks_*`, `files_*`, `db_*`,
`wallet_*`, `saved_list`, `content_*`, and editing your own `apps_*` or `blog_*`.

Work anonymously:

`news_*`, `markets_list`, `weather_forecast`, `video_*`, `blog_list`,
`blog_read`, `stream_list`, `chat_rooms`, `chat_messages`, `apps_search`,
`apps_read`, `social_*`, `prayer_*`, `quran_search`, `images_search`.

`index_search` is the one that does both: anyone may call it and gets the
instance's public content; a caller with an account gets their own entries and
their own mail on top. So it is worth calling even unauthenticated, and worth
calling again once you have a token.

`mail_send` is the strict case: account-only whatever the price, so an
unaccountable caller cannot spend the instance's domain reputation.

## Costs

A call costs credits when it costs the instance money to run — a model call, or
a paid third party such as web search, image generation, or places. Operations
that only touch the instance's own storage are free, and that includes several
you might assume are metered: reading news, fetching a URL, writing records,
sending mail between local accounts.

**Do not carry a price list.** Every price is an instance setting
(`CREDIT_COST_*`), so a self-hosted Mu may charge differently from the reference
instance or nothing at all — a self-hosted instance with payments unconfigured
disables quotas entirely. A table copied into a prompt will be wrong somewhere.

Ask instead:

- `wallet_check` — what an operation would cost, before committing to it
- `wallet_balance` — credits remaining
- `/tools` — the live per-tool list for a human

Credits are prepaid against the account, and it is the same balance whether the
call comes from an agent or from a person using the web app.

The shape worth internalising rather than the numbers: **reading this instance
is cheap, reaching outside it is not.** If an answer can come from
`index_search` or `news_search` instead of `web_search`, it is both faster and
cheaper.

## Protocol

```bash
# initialize
curl -X POST https://micro.mu/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"example","version":"1.0"},"capabilities":{}}}'

# list tools
curl -X POST https://micro.mu/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# call one
curl -X POST https://micro.mu/mcp -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"news_search","arguments":{"query":"rates"}}}'
```

Mu also answers at `/api` and speaks A2A at `/a2a`, but MCP is the path
everything else is derived from and the one to reach for.
