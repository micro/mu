# Help

Mu is the everyday internet as tools an agent can call — news, mail, search,
weather, markets, video, files, notes — on one account, paid per request. One
balance instead of a hundred signups.

## Point an agent at it

One endpoint, `POST /mcp`, speaking [MCP](https://modelcontextprotocol.io).
There is no second API and nothing to integrate per tool.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

No key in that config, because there is nothing to paste: a client that speaks
OAuth signs in from the client and gets its own token. Works with Claude
Desktop, Cursor, or anything else that speaks MCP.

For anything that does not do the sign-in dance, make a token at
[/agents](/agents) and send it:

```
Authorization: Bearer YOUR_TOKEN
```

A token can be scoped to some services and not others, which is the control
that actually restricts what an agent can do.

Connecting for one job? Name the services you want and the rest stay out of the
list:

```
https://micro.mu/mcp?tools=news,web,mail
```

That changes what is **listed**, not what may be **called** — it is a context
problem, not a permission one. There is a picker on [/tools](/tools) that
builds the URL.

## Pay without an account

Every priced call answers `402 Payment Required` with an
[x402](https://x402.org) challenge: pay in USDC, retry, get your answer. No
signup, no card, no account. What each call costs is on [/plans](/plans).

Signed in, the same calls come out of your credit balance, which is on
[your account](/account).

## The command line

The same binary is a CLI. Every tool is a subcommand:

```bash
mu news_list
mu weather_forecast --location "London"
mu agent "what happened in markets today"
```

`mu --help` lists what is there. It reads the same catalogue the agent does, so
it never falls behind.

## What is here

[Tools](/tools) is what an agent can call, priced. [Services](/services) is the
same catalogue as things you can open and use yourself. Both are generated from
what this instance actually runs.

## Run your own

[Install](/install) — one Go binary, self-hostable. Anyone paying to call your
tools pays you.
