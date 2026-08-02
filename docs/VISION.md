# Vision

**Tools for agents.**

## The Problem

An agent is only as useful as what it can reach. Today that is either a chat box
with no hands, or a proxy to an API somebody else owns. The tools an agent gets
handed are mostly thin wrappers — reselling access to a service the wrapper does
not run, with no margin and nothing to defend.

Meanwhile the everyday internet — news, search, mail, markets, video — is owned
by a handful of platforms that monetise attention rather than serve the caller.
Their APIs are priced and rate-limited for their benefit, and an agent calling
them is a second-class customer of a product built for someone else.

## What Mu Is

Mu is the everyday internet as tools an agent can call, run by whoever operates
the instance.

It runs the things it exposes. `mail_inbox` reads a real inbox behind an SMTP
server with DKIM. `web_search` queries an index it maintains. `db_create` writes
to real storage. Not a catalogue of other people's products — this instance's
own capabilities, exposed over MCP and REST, paid per request in stablecoin with
no account in the way.

The previous generation of this idea sold APIs to developers, who have to find
you, choose you, integrate you and keep choosing you. An agent does not shop: it
is handed a tool list and uses what is there. That inverts the funnel, and x402
removes the signup between wanting and paying.

There is also an app, because the operator is a person. Signed in, the same
services render as a home screen — headlines, prices, weather, unread mail —
with the agent inline. One set of services, two doors. A new service appears in
both at once, and the tools are used by a human every day before any agent calls
them.

## Design Choices

**Tools first.** Every capability is a tool before it is a page. The MCP endpoint is not a side door onto the app — the app is one caller among several.

**AI-first for people.** Signed in, the home screen is a prompt, not a dashboard. Ask what you need, get an answer. Cards are secondary — browse when you want depth.

**Contextual.** The AI knows your state: unread mail, market movements, your preferences. Suggestions are generated from your data, not an algorithm.

**Memory.** The AI remembers what you tell it across sessions. "I'm interested in AI and crypto" shapes every future response.

**Chronological feeds.** No algorithm decides what you see. News is sorted by time. You choose what to read.

**Finite content.** No infinite scroll. You see what's there and move on.

**No ads, no tracking.** Revenue comes from usage credits, not attention.

**Services, not features.** Every capability is a real [go-micro](https://go-micro.dev) service — news, mail, weather, markets and the rest — discoverable and callable over REST, MCP, A2A or the CLI. The agent is a go-micro agent that operates them. You own the whole stack, not a bundle of UI features.

**Single binary.** One Go binary built on go-micro, no external dependencies. Services run in-process today; the same handlers can be split across machines later by swapping the registry, with no code changes. Self-host your own instance.

**Local models.** Self-hosters can use Ollama or any OpenAI-compatible server. No cloud dependency required.

## What's included

| Service | What it does |
|---------|-------------|
| **AI Agent** | Ask anything — searches, checks, fetches across all services. Remembers preferences. |
| **News** | Headlines from RSS feeds, chronological, with AI summaries |
| **Markets** | Live crypto, futures, commodity, and currency prices |
| **Weather** | Forecasts and conditions |
| **Video** | YouTube without ads, algorithms, or shorts |
| **Web** | Search the web without tracking |
| **Places** | Search places and nearby results with configured providers and open-data fallbacks |
| **Blog** | Microblogging with daily AI-generated digests |
| **Chat** | Conversational AI with session history |
| **Mail** | Private messaging and email |
| **Apps** | Build and use small web tools — pin any app as a home card |
| **Reminder** | Daily Islamic reminder surfaced on the home screen and through tools |
| **Stream** | Public event feed for agents and tools |

## For Developers

Every service is available via REST API and MCP. Connect Claude Desktop, Cursor, or any MCP-compatible client:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

30+ tools. Pay per-request with USDC via [x402](https://x402.org). First 10 calls free per wallet.

The CLI (`mu news`, `mu agent "..."`) gives command-line access to every tool.

---

*Mu is open source under [AGPL-3.0](https://github.com/micro/mu/blob/main/LICENSE).*
