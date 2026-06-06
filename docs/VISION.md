# Vision

**Your personal AI — not another dashboard.**

## The Problem

The tools people use every day — news, search, email, chat, markets — are scattered across dozens of platforms, each competing for your time and data. Every platform monetises your attention. Infinite scroll keeps you hooked. Algorithms decide what you see.

The internet became addictive. And no single place brings it all together without the noise.

## What Mu Is

Mu is a personal AI platform. Instead of browsing separate apps, you ask one AI that has access to all your services. It checks your mail, looks up prices, searches the web, reads the news, and gives you a personalised answer.

The AI remembers what you care about. It surfaces relevant information before you ask. Over time, it learns your preferences and becomes more useful.

Technology should serve people — not use them. When you pay for tools, incentives are aligned. We build the tools, you use them. That's it.

## Design Choices

**AI-first.** The home screen is a prompt, not a dashboard. Ask what you need, get an answer. Cards are secondary — browse when you want depth.

**Contextual.** The AI knows your state: unread mail, market movements, your preferences. Suggestions are generated from your data, not an algorithm.

**Memory.** The AI remembers what you tell it across sessions. "I'm interested in AI and crypto" shapes every future response.

**Chronological feeds.** No algorithm decides what you see. News is sorted by time. You choose what to read.

**Finite content.** No infinite scroll. You see what's there and move on.

**No ads, no tracking.** Revenue comes from usage credits, not attention.

**Single binary.** One Go binary, no external dependencies. Self-host your own instance.

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
| **Blog** | Microblogging with daily AI-generated digests |
| **Chat** | Conversational AI with session history |
| **Mail** | Private messaging and email |
| **Apps** | Build and use small web tools — pin any app as a home card |
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
