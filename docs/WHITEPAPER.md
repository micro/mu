# Mu: A Unified Service Network with Native Payments for Humans and Autonomous Agents

Asim Aslam

asim@mu.xyz

mu.xyz

**Abstract.** The dominant model for internet services relies on advertising revenue, which creates a structural incentive to maximise user engagement rather than user utility. We propose Mu, a unified network of composable services — including news aggregation, web search, messaging, financial data, weather, location services, and AI — accessible through a single protocol endpoint and funded by direct per-use micropayments rather than advertising. Services are exposed via the Model Context Protocol (MCP), a JSON-RPC 2.0 interface that serves both human users and autonomous AI agents. Payments are handled through two complementary mechanisms: traditional card payments for account-holding users, and the x402 HTTP payment protocol for account-free, per-request settlement using on-chain stablecoins. The system is implemented as a single self-hostable binary. We describe the architecture, the credit-based economic model, the payment protocols, a mechanism for peer-to-peer credit transfer, and a path toward a federated network of nodes sharing a common settlement layer.

## 1. Introduction

This paper describes Mu, a unified network of composable services accessible through a single protocol endpoint and funded by direct micropayments rather than advertising. Mu provides the tools people use every day — news, search, chat, video, email, markets, weather — in one place, without ads, tracking, or algorithmic manipulation. Every service is equally available to human users via a web interface and to autonomous AI agents via the Model Context Protocol.

## 2. Motivation

### 2.1 The Advertising Model

The prevailing business model of internet platforms is advertising. Users receive services at no direct monetary cost; in exchange, their attention and behavioural data are sold to advertisers. This creates a structural misalignment: platforms are optimised to maximise time spent rather than tasks accomplished [1].

The consequences are well-documented. Algorithmic feeds prioritise content that provokes engagement — outrage, anxiety, tribal identification — over content that informs or serves. Infinite scroll mechanics eliminate natural stopping points. Push notifications interrupt users to pull them back. Likes and follower counts gamify social interaction, converting human connection into a metric to be optimised.

These are not incidental features. They are the direct result of an economic model that treats user attention as inventory to be sold. A platform that earns more when users spend more time will inevitably optimise for time spent.

### 2.2 Fragmentation

The tools people use daily — news, search, email, chat, markets, weather — are scattered across dozens of platforms. Each one requires a separate account, a separate privacy policy, a separate data relationship. Each one competes independently for attention. There is no single place that brings these utilities together without the noise.

This fragmentation also means redundant infrastructure: every platform builds its own authentication, its own payment system, its own API layer, its own mobile app. Users manage dozens of accounts. Developers integrate with dozens of APIs.

### 2.3 The Agent Problem

A new class of user is emerging: autonomous AI agents that need access to real-world services. An agent that wants to search the web, check the weather, send an email, and look up nearby restaurants currently needs four different providers, four signups, four API keys, and four billing relationships.

No unified protocol exists for an agent to discover, access, and pay for heterogeneous services through a single interface. The overhead of onboarding — account creation, key management, billing setup — is a barrier to agent autonomy. An agent cannot spontaneously decide to use a new service without a human first establishing the relationship.

### 2.4 Work and Meaning

Existing platforms for sharing and finding work — LinkedIn, Upwork, Product Hunt, Hacker News — are subject to the same engagement dynamics. LinkedIn optimises for connection count and content virality. Upwork creates a race-to-the-bottom on pricing. Product Hunt gamifies launches with leaderboards and upvote counts. Hacker News subjects work to anonymous pile-ons.

People want to share their work and get honest feedback from people they trust. They want to post tasks and pay fair prices for results. These are simple interactions that do not require gamification, algorithmic ranking, or engagement metrics.

## 3. Design

### 2.1 Principles

Mu is built on a single premise: technology should serve the user, not the platform. Every design decision is evaluated against this. If a feature increases engagement at the expense of utility, it is excluded.

The system is deliberately anti-addictive. Content feeds are chronological, not algorithmically ranked. There is no infinite scroll — content has an end. There are no likes, follower counts, or engagement metrics visible to users. There are no push notifications. There is no gamification of social interaction. These are not missing features; they are deliberate omissions. Each one is a mechanism that existing platforms use to maximise time spent, and each one misaligns the platform's incentives with the user's.

### 2.2 Service Selection

Mu aggregates the services that people use daily and that are currently scattered across advertising-funded platforms:

| Service | Function | Replaces |
|---------|----------|----------|
| News | RSS aggregation with AI summaries | Google News, Apple News |
| Video | YouTube without ads or shorts | YouTube |
| Web | Privacy-respecting search | Google Search |
| Blog | Microblogging with federation | Twitter/X, Threads |
| Chat | AI conversation | ChatGPT |
| Mail | Messaging and email | Gmail |
| Markets | Live prices | Yahoo Finance |
| Weather | Forecasts | Weather apps |
| Places | Location search | Google Maps |
| Apps | Build and run tools | App stores |
| Agent | AI orchestration across all services | — |
| Work | Share work, post tasks, get feedback | Product Hunt, Upwork |
| Wallet | Credits and payments | — |

Each service does one thing. There are no social feeds combining heterogeneous content types, no "discover" tabs, no trending sections. A user arrives with intent, uses the service, and leaves.

### 2.3 Economic Design

The revenue model determines platform behaviour. Advertising-funded platforms are structurally incentivised to maximise attention. Subscription platforms are incentivised to maximise perceived value, which leads to feature bloat and engagement optimisation.

Mu uses per-use micropayments. The platform is incentivised to build tools that solve the user's problem as quickly as possible — the opposite of engagement maximisation. Browsing is included. Only operations that consume infrastructure (API calls, LLM inference, SMTP delivery) carry a cost. Every account includes a daily quota for casual use.

Operations that create public content or reach external systems (email, blog posts, web fetching) always require real credits, even within the daily quota. This prevents abuse at scale.

### 2.4 Self-Hosting

The system is designed to run as a single binary with no external dependencies. This is a deliberate architectural constraint, not a limitation. It means any individual or organisation can run their own instance with full functionality, no vendor lock-in, and no ongoing costs beyond hosting. When no payment provider is configured, all quotas are disabled — the self-hosted instance runs without restrictions.

## 4. Architecture

### 3.1 Layered Structure

Mu is implemented as a single Go binary comprising three layers:

**Subsystems** provide infrastructure primitives: HTTP rendering, API dispatch, LLM integration, data storage with full-text search, authentication, and administration.

**Building blocks** are user-facing services. Each building block is a self-contained Go package that uses the subsystem primitives. Current building blocks include news aggregation (RSS with AI summarisation), video (YouTube integration without advertising), web search (via Brave Search API), microblogging (with ActivityPub federation), AI chat, messaging (with SMTP and DKIM), financial markets (cryptocurrency and commodity prices via Coinbase API), weather forecasts, location search, a work board, and an application builder.

**Agents** are autonomous processes that compose building blocks. An agent receives a natural language instruction and executes a sequence of tool calls across multiple building blocks to fulfil it. Agents operate through the same MCP interface available to external clients.

### 3.2 Binary and Storage

All static assets — HTML, CSS, JavaScript, icons — are embedded at compile time. Persistent data is stored as JSON files on disk, with optional SQLite and FTS5 indexing for full-text search. The system starts from a single command with no external dependencies.

### 3.3 Communication

Building blocks communicate through an internal publish-subscribe event bus. This avoids direct coupling between packages: the news system publishes article events that the blog system subscribes to for digest generation; the chat system publishes URL references that trigger metadata refresh in the news system; the agent system issues tool calls that the wallet system intercepts for quota enforcement. No building block imports another directly.

### 3.4 Protocol Interface

Every building block is exposed as a tool through the Model Context Protocol at a single HTTP endpoint. MCP defines a JSON-RPC 2.0 interface for tool discovery (`tools/list`) and invocation (`tools/call`). The Mu MCP server currently exposes over thirty tools spanning information retrieval, search, content creation, communication, and account management.

An MCP client — whether a human-operated AI assistant or an autonomous agent — connects to the endpoint and receives a complete catalogue of available tools with typed parameter schemas. Tool invocations are dispatched internally to the corresponding building block handler. Authentication is forwarded from the outer HTTP request via session cookies, bearer tokens, or x402 payment headers.

This design means that every service available through the web interface is equally available to any MCP-compatible client, including Claude Desktop, Cursor, and custom agent implementations.

## 5. Economic Model

### 3.1 Credit System

Mu uses an integer credit system where one credit equals one penny sterling (GBP 0.01). Credits are stored as signed integers to avoid floating-point representation errors. The credit is the atomic unit of value for all transactions within the system.

Credits are acquired through card payment (Section 6.1) or cryptocurrency payment (Section 6.2), and consumed by service usage. Credits do not expire.

### 3.2 Cost Structure

Each operation has a fixed credit cost determined by the underlying infrastructure expense:

| Operation | Credits | Infrastructure basis |
|-----------|---------|---------------------|
| News search | 1 | Indexed query |
| Video search | 2 | YouTube Data API |
| AI chat query | 3 | LLM inference |
| Web search | 5 | Brave Search API |
| Places search | 5 | Google Places API |
| External email | 4 | SMTP delivery |
| Weather forecast | 1 | Weather API |
| AI agent (standard) | 3 | LLM inference |
| AI agent (premium) | 9 | Premium model inference |

Read-only operations — browsing news feeds, reading blog posts, watching videos, viewing market prices — carry no cost.

### 3.3 Daily Quota

Each account includes a daily allocation of one hundred queries, resetting at midnight UTC. This quota is sufficient for casual utility use. When the daily quota is exhausted, subsequent operations consume credits from the user's wallet. This model ensures accessibility while covering infrastructure costs for heavy usage.

### 3.4 Incentive Alignment

The system deliberately avoids subscription tiers. A fixed monthly fee creates pressure on the platform to increase perceived value through engagement maximisation — the same dynamic that produces the harms described in Section 2. Pay-per-use pricing aligns the platform's incentive with the user's: the most valuable outcome is a tool that accomplishes the user's goal as quickly and efficiently as possible.

Users who wish to avoid all payment may self-host the software. When no payment provider is configured, all quota enforcement is disabled.

## 6. Payment Protocols

### 4.1 Card Payments

For human users with accounts, Mu integrates Stripe for card-based credit purchases. The flow is standard: the user selects an amount (GBP 1–500), is redirected to a Stripe Checkout session, and upon completion receives credits via a verified webhook callback. The webhook payload is authenticated using HMAC-SHA256 signature verification. Session identifiers are deduplicated to prevent double crediting.

### 4.2 The x402 Protocol

For autonomous agents and programmatic clients, Mu implements the x402 protocol [2], which extends HTTP with native payment semantics.

When an unauthenticated client requests a metered resource, the server returns HTTP status 402 (Payment Required) with an `X-PAYMENT-REQUIRED` header encoding the payment terms: the amount, the acceptable blockchain networks, the accepted asset contracts, and the recipient wallet address.

The client constructs an on-chain payment (currently USDC or EURC on Base, an Ethereum Layer 2 network), signs the transaction, and retries the original request with an `X-PAYMENT` header containing the payment proof.

The server submits the proof to a facilitator service for verification and settlement. Upon confirmation, the server executes the requested operation and returns the result. The entire payment cycle occurs within the HTTP request-response lifecycle.

This mechanism requires no account creation, no API key, and no prior relationship between client and server. The client's wallet address serves as its identity. Credit costs are converted to USD at a rate of one credit per cent (USD 0.01 per credit).

### 4.3 Comparison of Payment Mechanisms

| Property | Card (Stripe) | Crypto (x402) |
|----------|---------------|---------------|
| Account required | Yes | No |
| Payment model | Pre-fund wallet | Per-request |
| Settlement | Webhook (seconds) | On-chain (seconds) |
| Denomination | GBP | USD (stablecoin) |
| Identity | Username | Wallet address |
| Suitable for | Human users | Autonomous agents |

### 4.4 Credit Transfer

Users may transfer credits to other users on the same instance. A transfer atomically deducts from the sender's balance and credits the recipient's balance, recording transactions on both sides with cross-referencing metadata. Transfers are subject to a configurable maximum (default: 50,000 credits) and are non-reversible.

The transfer mechanism enables peer-to-peer payments between users, creator tipping, and informal service settlement within the network.

## 7. Services Marketplace

### 5.1 Extension Model

The building block architecture is designed for extensibility. A third-party developer can implement an MCP-compatible service — a server that responds to `tools/list` and `tools/call` — and register it in a central marketplace directory.

When a user invokes a marketplace service, the Mu instance acts as a proxy: it verifies the user's credit balance, forwards the MCP tool call to the provider's endpoint, and upon successful response, deducts credits from the user and credits the provider. The default revenue split is 70% to the provider and 30% to the platform.

### 5.2 Direct Settlement

Providers may alternatively run x402-enabled MCP servers and receive payment directly from agents on-chain, bypassing the platform entirely. In this model, the marketplace serves as a discovery and reputation layer rather than a payment intermediary. Providers retain the full payment amount.

This creates a spectrum of integration: providers who want distribution and billing handled for them use the proxied model; providers who want maximum revenue and direct agent relationships use x402 settlement.

## 8. Federation

Blog posts are published as ActivityPub objects with WebFinger discovery. Users on federated platforms — Mastodon, Threads, and other ActivityPub implementations — can follow Mu authors, receive posts in their feeds, and interact through the standard ActivityPub inbox/outbox mechanism.

Internal messages between Mu users are included. External email is delivered via SMTP with DKIM signing, at a credit cost that reflects delivery infrastructure.

## 9. Toward a Federated Network

### 7.1 Current Limitations

In the current architecture, each Mu instance operates independently. User accounts, wallet balances, and transaction histories are local to a single server. A user on one instance cannot transfer credits to a user on another instance. Self-hosted instances are economically isolated.

### 7.2 Shared Settlement Layer

A credit token deployed on a Layer 2 blockchain (such as Base, where x402 settlement already operates) would address these limitations.

**Token mechanics.** Credits would be represented as an ERC-20 token with a stable peg (one token = one penny or one cent). Tokens are minted when a user purchases credits via card or stablecoin, and burned when consumed by service usage. The minting and burning operations maintain the peg without requiring a reserve or algorithmic stabilisation — each token in circulation corresponds to a real payment received.

**Cross-instance identity.** A user's wallet address becomes their portable identity. Any Mu instance can verify an on-chain balance without trusting the originating instance. Authentication reduces to a signature challenge: prove you control this address.

**Cross-instance transfers.** Credit transfers between users on different instances become standard token transfers on the shared ledger. No bilateral trust or federation protocol is required between instances — the blockchain is the common substrate.

**Marketplace settlement.** Service providers register their MCP endpoints in an on-chain registry readable by any instance. Agents discover services by querying the registry and pay providers directly via token transfer or x402, without routing through a central platform.

### 7.3 Network Topology

The resulting architecture is a network of independent Mu nodes sharing a common economic layer:

Each node runs the full Mu binary with local storage and local service execution. Nodes do not need to communicate directly with each other. They share state only through the settlement layer: wallet balances, service registrations, and reputation data are on-chain; content federation uses ActivityPub; everything else remains local.

This preserves the single-binary simplicity of each node while enabling network-level interoperability where it matters: payments, identity, and service discovery.

### 7.4 Implementation Sequencing

This transition does not require a single coordinated migration. Each capability can be deployed independently:

1. Deploy the credit token contract on Base. Mirror existing wallet balances on-chain. Continue operating the local wallet as a cache.
2. Accept the token for credit purchases alongside Stripe and raw stablecoins. Existing payment flows remain unchanged.
3. Enable cross-instance transfers by reading and writing balances from the chain rather than local storage.
4. Deploy the marketplace service registry contract. Enable any instance to discover and route to registered providers.

The centralised system continues to function at each stage. Blockchain settlement is additive, not replacing the existing infrastructure but extending it.

## 10. Security

**Authentication.** The system supports WebAuthn passkeys (phishing-resistant, passwordless), bcrypt-hashed passwords, session cookies, personal access tokens for API use, and x402 wallet-address identity for agents. Each mechanism is appropriate for a different client type.

**Wallet integrity.** Balance deductions are performed under a mutex with an explicit sufficiency check. The balance cannot go negative. All transactions are recorded with unique identifiers, timestamps, and cross-referencing metadata. Stripe webhook payloads are verified by HMAC-SHA256 signature. x402 payments are verified by an on-chain facilitator.

**Content safety.** An administrative moderation queue with user-initiated flagging provides content review. Rate limiting is applied to all write operations. Input validation occurs at system boundaries.

## 11. Related Work

The separation of service provision from advertising revenue has been explored in various contexts. Brave Browser [3] replaces third-party ads with a privacy-preserving attention token. Kagi [4] offers paid web search without tracking. Neither provides a unified multi-service platform or programmatic agent access.

The Model Context Protocol [5] standardises tool access for AI agents. Several platforms expose individual services via MCP, but none provide a bundled service network with integrated micropayments.

The x402 protocol [2] draws on earlier work in HTTP payment negotiation (RFC 7235, HTTP 402 status code) and applies it to blockchain-settled micropayments. Mu is among the first production systems to integrate x402 for service access by autonomous agents.

ActivityPub [6] provides the federation layer for content distribution, complementing the economic federation described in Section 9.

## 12. Conclusion

Mu demonstrates that a viable alternative to advertising-funded internet services can be constructed from three components: a composable service architecture, a standard protocol for tool access, and native micropayment mechanisms for both human users and autonomous agents.

The system currently operates as a single self-hostable binary serving a dozen integrated services through a unified MCP endpoint, with card and cryptocurrency payment support. The architecture admits extension to a federated network of nodes sharing a common settlement layer, enabling cross-instance identity, portable credits, and a permissionless service marketplace — without sacrificing the simplicity of individual node deployment.

The software is open source under the AGPL-3.0 licence.

## References

[1] Zuboff, S. *The Age of Surveillance Capitalism.* PublicAffairs, 2019.

[2] x402 Protocol. https://x402.org

[3] Brave Software. *Basic Attention Token.* https://basicattentiontoken.org

[4] Kagi Inc. *Kagi Search.* https://kagi.com

[5] Model Context Protocol. https://modelcontextprotocol.io

[6] Lemmer-Webber, C. et al. *ActivityPub.* W3C Recommendation, 2018. https://www.w3.org/TR/activitypub/
