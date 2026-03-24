# Services Marketplace

## The Idea

Mu is built as a platform of services — news, search, chat, video, weather, places, and more. Each service follows the same pattern: a Go package with a `Load()` function, an HTTP handler, and an MCP tool registration. The MCP server already exposes these services as tools that any AI agent can call via JSON-RPC.

What if anyone could add services to Mu?

Not by deploying code on our servers, but by running their own MCP-compatible service and registering it in a public directory. A marketplace where developers offer useful services — recipe extraction, legal document summarisation, flight tracking, price comparison, language translation — and users pay per use through the existing credit system.

The infrastructure for this already exists in pieces:
- **MCP protocol** for standardised tool calling
- **Wallet system** for per-use billing
- **Agent** that can orchestrate multiple tool calls
- **Event system** for decoupled service communication

The marketplace connects these pieces.

## How It Works

### For Service Providers

A developer builds a service and runs it as an MCP server. They register it in the Mu marketplace with:

```json
{
  "name": "recipe-extract",
  "description": "Extract clean recipes from any cooking website — ingredients, steps, timing, no ads",
  "endpoint": "https://recipes.example.com/mcp",
  "author": "chef_dev",
  "tools": [
    {
      "name": "extract_recipe",
      "description": "Extract a recipe from a URL",
      "params": [
        {"name": "url", "type": "string", "required": true}
      ]
    }
  ],
  "pricing": {
    "per_call": 2,
    "currency": "credits"
  },
  "category": "Food",
  "source_url": "https://github.com/chef_dev/recipe-extract"
}
```

The service runs on their infrastructure. Mu never hosts their code.

### For Users

Users browse the marketplace at `/marketplace` or discover services through the agent. When a user asks "extract the recipe from this URL," the agent can:

1. Check if any marketplace service handles recipe extraction
2. Show the user the service, its cost, and its rating
3. Call the service via MCP on the user's behalf
4. Deduct credits and pass a share to the provider

Users can also browse, install (enable), and review services directly.

### For the Platform

Mu acts as the registry, billing layer, and trust intermediary:

- **Registry**: Directory of available services with descriptions, pricing, ratings
- **Billing**: Credits flow through the existing wallet — user pays, Mu takes a cut, provider gets the rest
- **Trust**: Reviews, usage stats, and verification badges help users evaluate services
- **Discovery**: The agent knows about marketplace services and can suggest them contextually

## Architecture

### Service Registration

```
Developer                    Mu Platform                   Users
   |                            |                            |
   |-- Register service ------->|                            |
   |   (endpoint, tools,        |                            |
   |    pricing, description)   |                            |
   |                            |-- List in marketplace ---->|
   |                            |-- Expose to agent -------->|
   |                            |                            |
   |                            |<-- User calls service -----|
   |<-- MCP tool call ----------|                            |
   |-- Return result ---------->|                            |
   |                            |-- Deduct credits --------->|
   |                            |-- Credit provider -------->|
```

### Data Model

**Service** — a registered marketplace entry:

```go
type Service struct {
    ID          string    // Unique identifier
    Name        string    // Human-readable name
    Slug        string    // URL-friendly slug
    Description string    // What it does
    Endpoint    string    // MCP server URL
    AuthorID    string    // Mu user ID of the provider
    Author      string    // Display name
    Category    string    // Service category
    Tools       []Tool    // MCP tool definitions
    Pricing     Pricing   // Cost per call
    SourceURL   string    // Optional link to source code
    Verified    bool      // Mu-verified service
    Active      bool      // Currently accepting requests
    Rating      float64   // Average user rating (1-5)
    CallCount   int       // Total calls made
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Pricing struct {
    PerCall  int    // Credits per tool call
    Currency string // Always "credits" for now
}
```

**ServiceReview** — user feedback:

```go
type ServiceReview struct {
    ID        string
    ServiceID string
    UserID    string
    Rating    int    // 1-5
    Comment   string
    CreatedAt time.Time
}
```

### MCP Proxy Pattern

When a user calls a marketplace service, Mu acts as a proxy:

1. User's agent (or direct MCP call) requests a marketplace tool
2. Mu verifies the user has sufficient credits
3. Mu forwards the MCP `tools/call` request to the provider's endpoint
4. Provider processes and returns the result
5. Mu deducts credits from the user's wallet
6. Mu credits the provider's wallet (minus platform fee)
7. Mu returns the result to the user

This keeps the provider's endpoint hidden from the user (preventing bypass) and ensures billing is handled centrally.

### Revenue Split

A simple split:

| Recipient | Share | Rationale |
|-----------|-------|-----------|
| Provider  | 70%   | They built and host the service |
| Platform  | 30%   | Registry, billing, discovery, trust |

Providers set their own per-call price in credits. The platform fee is transparent — no hidden charges.

For the initial launch, the split could be more generous (80/20 or even 90/10) to attract providers.

## Agent Integration

The agent already knows about built-in tools. Marketplace services extend this:

```
const agentToolsDesc = `Available tools:
- news: Get latest news feed
- web_search: Search the web
- web_fetch: Fetch a web page
...

Marketplace tools (user-enabled):
- recipe_extract: Extract clean recipes from cooking URLs (2 credits)
- translate: Translate text between languages (1 credit)
- flight_status: Check flight status and delays (3 credits)
`
```

When a user enables a marketplace service, its tools are added to the agent's available tool list. The agent can then use them automatically when relevant.

### Contextual Discovery

When the agent can't fulfil a request with built-in tools, it could suggest marketplace services:

> "I don't have a built-in tool for extracting recipes, but there's a marketplace service called **Recipe Extract** by chef_dev (4.8 stars, 2 credits per use). Would you like me to use it?"

This creates a natural discovery mechanism without aggressive promotion.

## Categories

Initial categories based on common needs:

| Category | Example Services |
|----------|-----------------|
| Food | Recipe extraction, meal planning, nutrition lookup |
| Finance | Stock analysis, tax calculation, invoice parsing |
| Travel | Flight tracking, hotel comparison, visa requirements |
| Language | Translation, grammar check, summarisation |
| Dev | Code review, dependency audit, API testing |
| Health | Symptom lookup, medication interactions, fitness tracking |
| Education | Flashcard generation, quiz creation, citation formatting |
| Legal | Contract summarisation, terms of service analysis |
| Shopping | Price comparison, product reviews, deal alerts |
| Productivity | Calendar scheduling, email drafting, meeting notes |

## Trust & Safety

### Verification Levels

| Level | Badge | Requirements |
|-------|-------|-------------|
| Unverified | None | Just registered |
| Source Available | Source badge | Source code URL provided and reviewed |
| Verified | Verified badge | Mu team has reviewed the service |
| Official | Official badge | Built and maintained by Mu |

### Safety Measures

- **Rate limiting**: Marketplace calls rate-limited per user and per service
- **Timeout enforcement**: All proxy calls have a 15-second timeout
- **Content filtering**: Responses from marketplace services go through the same content moderation as user posts
- **SSRF prevention**: Provider endpoints must be public URLs (no private IPs, no localhost)
- **Audit logging**: All marketplace calls are logged for dispute resolution
- **Kill switch**: Admins can instantly disable any service
- **Abuse reporting**: Users can flag services for review

### Provider Requirements

- Must have a registered Mu account
- Must have a verified email
- Must provide a reachable MCP endpoint that responds to `tools/list`
- Must respond within 15 seconds
- Must not require user credentials (all auth goes through Mu)

## API

### Marketplace Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/marketplace` | Browse services (HTML + JSON) |
| GET | `/marketplace/{slug}` | Service detail page |
| POST | `/marketplace` | Register a new service (provider) |
| PATCH | `/marketplace/{slug}` | Update service details (provider) |
| DELETE | `/marketplace/{slug}` | Remove a service (provider) |
| POST | `/marketplace/{slug}/enable` | Enable a service for your account |
| DELETE | `/marketplace/{slug}/enable` | Disable a service |
| POST | `/marketplace/{slug}/review` | Leave a review |
| POST | `/marketplace/{slug}/call` | Call a service tool directly |

### MCP Integration

Marketplace tools appear in `tools/list` responses when the user has enabled them. They're namespaced to avoid collisions:

```json
{
  "name": "marketplace:recipe_extract:extract_recipe",
  "description": "Extract a recipe from a URL (via recipe_extract by chef_dev, 2 credits)"
}
```

Or with a simpler flat namespace for the agent:

```json
{
  "name": "extract_recipe",
  "description": "[Marketplace] Extract a recipe from a URL — 2 credits"
}
```

## Implementation Plan

### Phase 1: Registry (MVP)

Build the marketplace page and service registration. No proxy calls yet — just a directory.

- `/marketplace` page listing registered services
- Service registration form for providers
- Service detail pages with description, pricing, tools
- Admin approval workflow

### Phase 2: Proxy & Billing

Enable actual service calls through Mu as a proxy.

- MCP proxy that forwards tool calls to provider endpoints
- Credit deduction on successful calls
- Provider wallet crediting
- Call logging and basic analytics

### Phase 3: Agent Integration

Make marketplace services available to the agent.

- Dynamic tool list based on user's enabled services
- Contextual service suggestions when built-in tools can't help
- Tool result formatting for marketplace responses

### Phase 4: Trust & Discovery

Build the community layer around services.

- User reviews and ratings
- Usage statistics
- Verification badges
- Category browsing and search
- Featured/trending services

### Phase 5: Provider Tools

Help providers build and test services.

- Provider dashboard with call analytics and revenue
- Service health monitoring
- Test sandbox for development
- Documentation and examples

## Why This Works

### For Mu

- **Revenue**: 30% of every marketplace transaction without hosting costs
- **Network effects**: More services attract more users, more users attract more providers
- **Differentiation**: No other platform combines MCP tools + AI agent + per-use billing in this way
- **Aligned incentives**: We succeed when providers build useful services, not when users are addicted

### For Providers

- **Distribution**: Access to Mu's user base without building a frontend
- **Billing**: No need to handle payments — Mu does it
- **Discovery**: The agent recommends your service when relevant
- **Simplicity**: Just run an MCP server — the standard is simple and well-documented

### For Users

- **Choice**: Pick the services you need, pay only for what you use
- **Trust**: Reviewed and rated by other users, verified by Mu
- **Integration**: Services work seamlessly with the agent — no context switching
- **No lock-in**: Services are standard MCP — you can call them directly if you prefer

## x402 and the Marketplace

The [x402 protocol](https://x402.org) changes the marketplace economics. Instead of all payments flowing through Mu's credit system, providers can receive payments directly on-chain.

### Two Settlement Models

**Proxied (default):** Mu handles billing. User pays credits, Mu takes a cut, provider gets credited.

```
Agent → Mu (proxy) → Provider
         ↓
    Credit deduction
    Provider payout
```

**Direct (x402):** Provider runs their own x402-enabled MCP server. Agents pay them directly. Mu acts as a discovery layer, not a payment intermediary.

```
Agent → Mu (discovery) → finds provider endpoint
Agent → Provider (direct) → pays via x402
```

### Why This Matters

Direct settlement via x402 means:
- **Providers keep 100%** of revenue (no platform cut on direct calls)
- **Lower latency** — no proxy hop for the actual service call
- **Permissionless** — anyone can run an x402 MCP server, no approval needed
- **Composable** — agents can chain calls across multiple providers in a single workflow

Mu's value shifts from payment intermediary to **discovery and trust**: which services are reliable, well-reviewed, and worth paying for. The marketplace becomes a directory with reputation signals, and the agent uses it to find the best tool for the job.

### The Stack

This is what a services marketplace looks like when you start from first principles:

- **Protocol**: MCP (JSON-RPC 2.0) — simple, standardised, AI-native
- **Payments**: x402 — HTTP-native, per-request, on-chain settlement
- **Discovery**: Marketplace registry — browsable, searchable, agent-discoverable
- **Trust**: Reviews + verification — community-driven quality signals

## Relation to Micro

Mu's architecture draws from the microservices philosophy — small, focused services with clear interfaces. The Go Micro framework pioneered this pattern: services that are easy to build, deploy, and compose.

The marketplace extends this to the platform level. Instead of services running inside one binary, they run anywhere and connect via MCP. The protocol is the contract. The marketplace is the directory. x402 is the settlement layer.

---

*This document is a proposal for discussion.*
