# About Mu

**Your personal home server for the everyday internet — not another attention farm.**

## What is Mu?

News, mail, search, weather, video, markets — the everyday internet, handled by one agent you just talk to. Ask Mu for anything you'd normally open ten tabs and five apps to do, and get an answer instead.

It checks your mail, looks up prices, searches the web, reads the news, and gives you a personalised answer. Each of those is a real service, and the agent operates them on your behalf. The AI remembers your preferences, surfaces contextual suggestions, and learns what you care about over time. And because Mu is open and self-hostable, you can run the whole stack yourself instead of renting each piece from a different platform.

Technology should serve people — not use them.

## Why Mu Exists

The big platforms have a service for everything — and they own it. Every one monetises your attention: infinite scroll keeps you hooked, algorithms decide what you see, ads follow you everywhere, and your data gets mined and sold. The thing at the centre of your life isn't yours.

Mu was built on a different principle: **own your services, and pay for the tools, not with your attention.**

## How it works

Open Mu and you see a prompt. Below it, contextual suggestions based on your state — unread emails, market movements, news. Ask a question or tap a suggestion. The AI checks your services, composes an answer, and shows it inline.

Below the AI, cards give you an at-a-glance overview. Cards are configurable — show or hide what you care about.

## What's included

- **AI Agent** — Ask anything. A go-micro agent that calls, checks, fetches, and synthesises across all the services below. Remembers your preferences.
- **News** — Headlines from RSS feeds, chronological, with AI summaries
- **Markets** — Live crypto, futures, commodity, and currency prices
- **Weather** — Forecasts and conditions
- **Video** — YouTube without ads, algorithms, or shorts
- **Web** — Search the web without tracking
- **Places** — Search places and nearby results with configured providers and open-data fallbacks
- **Blog** — Microblogging with daily AI-generated digests
- **Chat** — Conversational AI with session history
- **Mail** — Private messaging and email
- **Apps** — Build and use small, useful tools — any app can be pinned as a home card
- **Reminder** — A daily Islamic reminder surfaced as a home card and MCP tool
- **Stream** — Public event feed for agents and tools

## What we don't do

- **No ads** — we don't sell your attention
- **No tracking** — we don't profile you
- **No algorithmic ranking** — chronological, transparent
- **No infinite scroll** — there's always an end
- **No push notifications** — you come when you want

## Technology

Mu runs as a single Go binary, built on [Go Micro](https://go-micro.dev) — an agent harness and service framework. Every capability (news, markets, weather, mail, search, video…) is a go-micro service; the assistant is a go-micro agent that calls them; `/mcp` is served by go-micro's MCP gateway. One runtime, one binary — and a real-world reference app that dogfoods the framework. Self-host on your own server. Open source under AGPL-3.0.

Supports Anthropic Claude, Atlas Cloud (DeepSeek, Qwen), or local models via any OpenAI-compatible API (Ollama, vLLM, llama.cpp).

When you pay for tools, incentives are aligned. We build the tools, you use them. That's it.

---

**Try it** at [micro.mu](https://micro.mu) — or self-host from [github.com/micro/mu](https://github.com/micro/mu)
