# Mu: A Network for Apps Without Ads, Algorithms, or Tracking

**Version 0.1 — March 2026**

**mu.xyz**

---

## Abstract

Mu is an open network of applications for everyday use — news, search, chat, video, email, markets, weather, and more — delivered without advertising, algorithmic manipulation, or user tracking. It runs as a single Go binary, is fully self-hostable, and provides a unified API via the Model Context Protocol (MCP) for both human users and autonomous AI agents.

This paper describes the architecture, economic model, payment protocols, and future direction of the Mu network, including the potential for a blockchain-based settlement layer that enables a permissionless marketplace of services.

---

## 1. The Problem

The internet's dominant platforms share a common business model: extract user attention, sell it to advertisers, and optimise for engagement above all else.

This creates a set of well-documented harms:

- **Algorithmic feeds** that prioritise engagement over truth or usefulness
- **Infinite scroll** and notification loops designed to maximise screen time
- **Surveillance** — behavioural profiling for ad targeting
- **Click and rage bait** to drive interaction metrics
- **Walled gardens** that lock users into proprietary ecosystems
- **Dark patterns** that manipulate users into sharing more data and spending more time

The fundamental misalignment is economic: platforms profit when users spend more time, not when they accomplish their goals. Every feature is evaluated by its effect on "engagement" — a euphemism for addiction.

The tools people use every day — news, search, email, chat, markets — are scattered across dozens of platforms, each competing for attention and data. There is no single place that brings them together without the noise.

### 1.1 The Agent Problem

A new class of user is emerging: autonomous AI agents that need access to real-world services. An agent that wants to search the web, check the weather, and send an email currently needs three different providers, three signups, three API keys, and three billing relationships.

There is no unified interface for agents to discover, access, and pay for services.

---

## 2. The Mu Network

Mu is a collection of apps built on a shared infrastructure. Each app does one thing well:

| Service | Function |
|---------|----------|
| **News** | RSS feed aggregation with AI summaries |
| **Video** | YouTube without ads, algorithms, or shorts |
| **Web** | Privacy-respecting web search (Brave) |
| **Blog** | Microblogging with ActivityPub federation |
| **Chat** | AI-powered conversation |
| **Mail** | Private messaging and external email (SMTP) |
| **Markets** | Live crypto, futures, and commodity prices |
| **Weather** | Forecasts and pollen data |
| **Places** | Location search and discovery |
| **Apps** | Build and run small web tools with AI |
| **Agent** | AI assistant that orchestrates all services |
| **Wallet** | Credit-based payment system |

### 2.1 Design Principles

Every design choice follows from a single question: **does this serve the user or the platform?**

- **Chronological feeds** — no algorithm decides what you see
- **Finite content** — no infinite scroll; content ends
- **No ads, no tracking** — revenue comes from usage, not attention
- **Pay for what you use** — no subscriptions that incentivise engagement
- **Single binary** — no external dependencies, easy to self-host
- **Open source** — AGPL-3.0, your server, your data

### 2.2 What We Exclude

Mu deliberately excludes features that drive addiction:

- No likes or follower counts
- No infinite scroll
- No push notifications
- No algorithmic ranking
- No engagement metrics visible to users
- No gamification of social interaction

---

## 3. Architecture

### 3.1 Layered Design

The system is structured in three layers:

```
Agents          — Autonomous processes that compose building blocks
Building Blocks — User-facing features (news, chat, blog, wallet, etc.)
Subsystems      — Fundamental infrastructure (app, api, ai, data, auth)
```

**Subsystems** provide primitives: rendering (app), API layer (api), LLM access (ai), storage (data), identity (auth), and administration (admin).

**Building blocks** are user-facing features. Each uses the subsystems: `app/` for rendering, `api/` for endpoints, `ai/` for intelligence, and `data/` for storage and events.

**Agents** compose building blocks autonomously. An agent can read news, check markets, generate analysis, and publish to the blog — all by orchestrating existing building blocks through MCP tools.

### 3.2 Single Binary

Mu compiles to a single Go binary with no external dependencies. All assets (HTML, CSS, JavaScript, icons) are embedded at build time. Storage uses JSON files on disk with optional SQLite and FTS5 for full-text search.

```
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```

This architectural simplicity provides:

- **Zero configuration** deployment
- **No distributed systems complexity**
- **Sub-second cold start**
- **Simple backup** — copy the data directory

### 3.3 Event-Driven Communication

Building blocks communicate via an internal event bus, avoiding tight coupling:

```
News publishes article → Blog subscribes and generates digest
Chat references article → News refreshes HN comments
Agent calls tool → Wallet checks quota
```

### 3.4 Progressive Web App

The frontend is a server-rendered PWA. No JavaScript framework, no client-side routing, no build step. HTML is rendered on the server with Go templates. The PWA manifest enables mobile installation without an app store.

---

## 4. Model Context Protocol (MCP)

Every service in Mu is exposed as an MCP tool at a single endpoint: `POST /mcp`.

MCP is a JSON-RPC 2.0 protocol that standardises how AI agents interact with tools. Mu implements the MCP server specification, exposing 30+ tools:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp"
    }
  }
}
```

### 4.1 Tool Categories

| Category | Tools |
|----------|-------|
| Information | news, video, markets, weather, social |
| Search | news_search, video_search, web_search, social_search, places_search |
| Communication | mail_send, blog_create, chat |
| Utility | web_fetch, places_nearby, reminder |
| Account | wallet_balance, wallet_topup, wallet_transfer |

### 4.2 Why MCP

MCP provides a standard interface that any AI client can use — Claude Desktop, Cursor, custom agents, or other MCP-compatible clients. This means Mu's services are accessible to the entire ecosystem of AI tools without custom integration.

The protocol also enables **composability**: agents can chain multiple tool calls in a single workflow, orchestrating searches, data retrieval, and content creation across all services.

---

## 5. Economic Model

### 5.1 Credit System

Mu uses a simple credit-based system:

- **1 credit = £0.01 GBP** (1 penny)
- Credits are stored as integers to avoid floating-point errors
- Credits never expire
- No subscriptions — pay as you go

### 5.2 Free Tier

Every registered user receives **20 free queries per day**, resetting at midnight UTC. This covers casual use of search, chat, and AI features. Browsing (reading news, blogs, videos, markets) is always free.

### 5.3 Pricing

Costs reflect actual infrastructure expenses:

| Operation | Credits | Cost |
|-----------|---------|------|
| News search | 1 | £0.01 |
| Video search | 2 | £0.02 |
| Chat query | 3 | £0.03 |
| Web search | 5 | £0.05 |
| Places search | 5 | £0.05 |
| External email | 4 | £0.04 |
| Weather forecast | 1 | £0.01 |

### 5.4 Why No Subscriptions

Unlimited tiers create misaligned incentives. If users pay a flat fee, the platform is incentivised to maximise engagement so users feel they are getting value. Pay-as-you-go keeps incentives aligned: we want to build efficient tools that solve problems quickly, not sticky products that maximise screen time.

### 5.5 Self-Hosting

Users who want unlimited free access can self-host. The code is open source (AGPL-3.0). When payments are not configured, all quotas are disabled.

---

## 6. Payment Protocols

Mu supports two payment methods, optimised for different use cases.

### 6.1 Card Payments (Stripe)

Traditional card payments for human users:

1. User selects an amount (£1–£500)
2. Redirected to Stripe Checkout
3. Credits added automatically via webhook
4. Rate: 1 credit = 1p, no bonuses or tiers

### 6.2 Crypto Payments (x402)

The [x402 protocol](https://x402.org) enables account-free, per-request payments with stablecoins. Designed for AI agents and programmatic clients.

**Flow:**

```
1. Client sends API request without payment
2. Server returns HTTP 402 with X-PAYMENT-REQUIRED header
3. Client signs payment on-chain (USDC/EURC on Base)
4. Client retries with X-PAYMENT header containing proof
5. Server verifies via facilitator, settles, serves response
```

**Pricing:** 1 credit = $0.01 USD. A web search (5 credits) costs $0.05 per request.

**Why x402:**

| | Card (Stripe) | Crypto (x402) |
|---|---|---|
| Account required | Yes | No |
| Payment model | Pre-pay credits | Pay per request |
| Settlement | Instant (webhook) | On-chain (~seconds) |
| Currency | GBP | USDC/EURC |
| Best for | Human users | AI agents |

x402 is HTTP-native — the payment is part of the request/response cycle. No signup, no API keys, no onboarding. The agent's wallet is its identity.

### 6.3 Wallet Transfers

Users can transfer credits to other users on the network. This enables:

- **Peer-to-peer payments** between Mu users
- **Tipping** content creators for blog posts
- **Service payments** where one user pays another for work
- **Gift credits** to friends or family

Transfers are instant, recorded in both parties' transaction history, and non-reversible.

---

## 7. Services Marketplace

### 7.1 Concept

Mu's architecture is extensible. Any developer can build an MCP-compatible service and register it in the Mu marketplace. Users discover and pay for services through the existing credit system.

Examples: recipe extraction, flight tracking, translation, price comparison, legal document summarisation.

### 7.2 Architecture

```
Developer runs MCP server → Registers in Mu marketplace
User requests service → Mu proxies MCP call → Provider responds
Credits deducted → Provider credited (70/30 split)
```

The Mu agent can discover and suggest marketplace services contextually: "I don't have a built-in tool for extracting recipes, but there's a marketplace service that can help (2 credits)."

### 7.3 Direct Settlement (x402)

Providers can run their own x402-enabled MCP servers. Agents pay them directly on-chain — no intermediary, no platform cut on direct calls. Mu's value shifts from payment intermediary to **discovery and trust**: which services are reliable and worth paying for.

---

## 8. Federation

Mu supports ActivityPub for decentralised social networking. Blog posts are published as ActivityPub objects, discoverable via WebFinger. Users on Mastodon, Threads, and other federated platforms can follow Mu authors and interact with their posts.

This means Mu blogs are not walled gardens — content is part of the open social web.

---

## 9. Toward a Blockchain Layer

### 9.1 Current State

Today, Mu runs as a single-server application with in-memory or JSON/SQLite storage. Payments are processed via Stripe (centralised) or x402 (on-chain, but only for API payments). The wallet is a centralised ledger.

### 9.2 The Case for Decentralisation

Several aspects of Mu would benefit from a decentralised settlement layer:

**Wallet as on-chain state.** Credits currently exist in a JSON file on one server. If credits were tokens on a blockchain, users would have true ownership — portable, verifiable, and not dependent on any single server.

**Cross-instance transfers.** Self-hosted Mu instances are currently isolated. A shared ledger would enable credit transfers between instances, creating a network of interconnected nodes.

**Marketplace settlement.** The services marketplace currently requires Mu as a payment intermediary. On-chain settlement would allow direct provider payments with transparent, auditable transactions.

**Verifiable usage records.** Transaction history on-chain provides an immutable audit trail that neither the platform nor the user can dispute.

### 9.3 Potential Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Mu Instances (self-hosted nodes)                       │
│                                                         │
│  Each runs the full Mu binary with local storage        │
│  Connected via shared settlement layer                  │
└─────────────────────┬───────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────┐
│  Settlement Layer                                       │
│                                                         │
│  - Credit token (ERC-20 or native)                      │
│  - Wallet balances as on-chain state                    │
│  - Cross-instance transfers                             │
│  - Marketplace service payments                         │
│  - Usage metering and verification                      │
└─────────────────────┬───────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────┐
│  Base Layer (L2)                                        │
│                                                         │
│  - Low-cost transactions                                │
│  - USDC/EURC stablecoin settlement                      │
│  - x402 protocol compatibility                          │
│  - Smart contract infrastructure                        │
└─────────────────────────────────────────────────────────┘
```

### 9.4 Credit Token

A Mu credit token would represent the unit of value across the network:

- **Minting:** Credits are minted when users top up via card (Stripe) or stablecoin (x402)
- **Burning:** Credits are burned when consumed by service usage
- **Transfer:** Users can transfer credits to any address on the network
- **Staking:** Marketplace providers could stake credits as a quality guarantee

The token would be pegged to a stable value (1 credit = £0.01 or $0.01) to maintain predictable pricing.

### 9.5 Decentralised Marketplace

With on-chain settlement, the marketplace becomes permissionless:

1. **Service registration** — providers register their MCP endpoint on-chain
2. **Discovery** — any Mu instance can read the registry
3. **Payment** — agents pay providers directly via x402 or token transfer
4. **Reputation** — service ratings stored on-chain, tamper-proof
5. **Disputes** — smart contract arbitration for failed service calls

### 9.6 Network of Nodes

Self-hosted Mu instances could form a network:

- **Shared identity** — one account works across all instances
- **Credit portability** — wallet balance usable anywhere
- **Content federation** — blog posts and messages flow between instances (already enabled via ActivityPub)
- **Service routing** — the closest or cheapest provider is selected automatically

### 9.7 Implementation Path

The blockchain layer is not required for Mu to function. The current centralised model works well for a single hosted instance. Decentralisation becomes valuable when:

1. Multiple self-hosted instances want to interoperate
2. The marketplace grows beyond a single operator
3. Users demand verifiable ownership of their credits
4. Cross-instance payments become a common use case

The path forward:

1. **Phase 1 (Current):** Centralised wallet with Stripe and x402 payments
2. **Phase 2:** Credit token on Base L2, wallet balances mirrored on-chain
3. **Phase 3:** Cross-instance transfers via token, decentralised marketplace registry
4. **Phase 4:** Full network of nodes with shared settlement and routing

---

## 10. Security Model

### 10.1 Authentication

Multiple authentication methods:

- **Passkeys (WebAuthn)** — passwordless, phishing-resistant
- **Username/password** — traditional login with bcrypt hashing
- **Session tokens** — cookie-based browser sessions
- **Personal Access Tokens (PAT)** — for programmatic API access
- **x402 payments** — agent identity via wallet address (no account needed)

### 10.2 Wallet Security

- Balances can never go negative
- All transactions have a full audit trail
- Transfer amounts are capped (max £500 per transfer)
- Stripe webhooks verified via HMAC-SHA256 signatures
- x402 payments verified via on-chain settlement through a facilitator

### 10.3 Content Safety

- AI-assisted content moderation
- Admin moderation queue with flagging system
- Rate limiting on all write operations
- Input validation at system boundaries

---

## 11. Comparison

| | Traditional Platforms | Mu |
|---|---|---|
| Revenue model | Ads + data mining | Usage credits |
| Content ranking | Algorithmic | Chronological |
| User tracking | Extensive | None |
| API access | Per-provider signup | Single MCP endpoint |
| Agent payments | API keys + subscriptions | x402 per-request |
| Self-hosting | Not possible | Full self-host support |
| Source code | Proprietary | Open source (AGPL-3.0) |
| Federation | Walled gardens | ActivityPub |
| Marketplace | Platform-controlled | Permissionless (x402) |

---

## 12. Roadmap

### Delivered
- Core services (news, video, chat, blog, mail, markets, weather, places, search)
- MCP server with 30+ tools
- Wallet with Stripe and x402 payments
- Wallet transfers between users
- ActivityPub federation
- App builder with AI generation
- Progressive Web App
- Self-hosting support

### In Progress
- Services marketplace (registry and discovery)
- Agent orchestration improvements

### Planned
- Credit token on Base L2
- Cross-instance wallet transfers
- Decentralised marketplace registry
- Network of self-hosted nodes
- Mobile-native apps (optional)

---

## 13. Conclusion

Mu is an attempt to build internet services the way they should have been built: as tools that serve users, not platforms that exploit them. The combination of MCP for universal tool access, x402 for frictionless payments, and a potential blockchain settlement layer creates infrastructure for a new kind of network — one where services are permissionless, payments are transparent, and users own their data.

The code is open source. The platform is self-hostable. The protocol is standard. What remains is to build the network.

---

**Website:** [mu.xyz](https://mu.xyz)
**Source:** [github.com/micro/mu](https://github.com/micro/mu)
**MCP Endpoint:** [mu.xyz/mcp](https://mu.xyz/mcp)
**License:** AGPL-3.0
