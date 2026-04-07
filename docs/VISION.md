# Vision

**Your personal dashboard. No ads. No tracking. No algorithm.**

## The Problem

The tools people use every day — news, search, email, chat, markets — are scattered across dozens of platforms, each competing for your time and data. Every platform monetises your attention. Infinite scroll keeps you hooked. Algorithms decide what you see. Ads follow you everywhere.

There's no single place that brings it all together without the noise.

## What Mu Is

Mu is a personal dashboard. News, markets, weather, mail, chat, AI — one screen, one account. Like iGoogle or Netvibes, but modern, self-hostable, and backed by an API that AI agents can use too.

Pay for the tools, not with your attention.

## Design Choices

**At a glance.** The home screen shows cards — a summary of each service. Headlines, prices, weather, your mail. Everything visible, details one tap away.

**Chronological feeds.** No algorithm decides what you see. News is sorted by time. You choose what to read.

**Finite content.** No infinite scroll. You see what's there and move on. The goal is to inform, not to keep you scrolling.

**No ads, no tracking.** Revenue comes from usage credits, not attention. There's no incentive to maximise screen time.

**Single binary.** One Go binary, no external dependencies. Self-host or use [mu.xyz](https://mu.xyz).

## What's on the dashboard

| Service | What it does |
|---------|-------------|
| **News** | Headlines from RSS feeds, chronological, with AI summaries |
| **Markets** | Live crypto, futures, commodity, and currency prices |
| **Weather** | Forecasts and conditions |
| **Video** | YouTube without ads, algorithms, or shorts |
| **Web** | Search the web without tracking |
| **Blog** | Microblogging with daily AI-generated digests |
| **Chat** | AI-powered conversation |
| **Mail** | Private messaging and email |
| **Agent** | AI assistant that searches, answers, and builds across every service |
| **Apps** | Build and use small web tools |

## For Developers

Every service is available via REST API and MCP. Connect Claude Desktop, Cursor, or any MCP-compatible client:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp"
    }
  }
}
```

AI agents can pay per-request with USDC through the [x402 protocol](https://x402.org). No API keys. No accounts. Just call and pay.

## Pricing

- **Browsing included** — news, blogs, videos, markets
- **20 credits per day** — covers search, chat, and AI features
- **Pay as you go** — 1 credit = 1p, top up via card
- **Crypto** — AI agents pay per-request via [x402](https://x402.org)
- **Self-host** — run your own instance, no restrictions

## Get Started

Visit [mu.xyz](https://mu.xyz) or self-host:

```
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```
