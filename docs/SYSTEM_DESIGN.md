# System Design

This document covers the design philosophy, architecture, and technical decisions for Mu.

## Vision

Mu is a personal AI platform that attempts to rectify deficiencies with existing social platforms like Twitter, Facebook, and YouTube.
The primary issue is exploitation and addiction. American-owned corporations exploit their users with click/rage bait, advertising,
and algorithms to drive engagement and profit. Their number one goal isn't user satisfaction—it's making money.

The goal is to build an alternative platform that removes these exploits:
- No algorithmic feeds
- No click and rage bait
- No ads
- No likes and retweets
- No addiction mechanics

What does a social platform look like without the addiction? A chronological feed with relevant content from you and those around you.
Not a follower count game, but a focus on thoughtful reflection. Content should be posted with deep introspection. Beyond microblogging,
we integrate useful AI chat, curated news headlines, and videos from select channels—no short videos, no clickbait, no ads.

## Architecture Overview

Mu is built as a **single Go binary** that runs on one server. This architectural simplicity provides:

- **Single language stack** - Everything in Go (backend, templates, API)
- **No distributed complexity** - Single process, single server
- **Progressive Web App** - Mobile-friendly without app stores
- **Monolithic simplicity** - Easy to understand, deploy, and maintain

### Layering Model

The system is structured in layers, from fundamental subsystems up through building blocks to agents that compose them.

```
┌─────────────────────────────────────────────────────────────────┐
│  Agents                                                         │
│  Autonomous processes that compose and orchestrate building     │
│  blocks to observe, analyze, and act on the world               │
│                                                                 │
│  agent/                                                         │
└────────────────────────────┬────────────────────────────────────┘
                             │ compose
┌────────────────────────────▼────────────────────────────────────┐
│  Building Blocks                                                │
│  User-facing features, each built on the subsystems below       │
│                                                                 │
│  blog/  news/  chat/  video/  mail/                             │
│  markets/  wallet/  places/  weather/  search/  home/           │
└────────────────────────────┬────────────────────────────────────┘
                             │ built on
┌────────────────────────────▼────────────────────────────────────┐
│  Subsystems                                                     │
│  Fundamental infrastructure that everything is made of          │
│                                                                 │
│  app/  api/  ai/  data/  auth/  admin/                          │
└─────────────────────────────────────────────────────────────────┘
```

**Subsystems** are the fundamental internals — app rendering, API layer, AI integration, data storage,
authentication, and admin tools. They provide the primitives that everything else is built on.

**Building blocks** are the user-facing features — blog, news, chat, video, mail, markets, wallet,
places, weather, search, and home. Each one is composed from the subsystems: it uses `app/` for rendering,
`api/` for endpoints, `ai/` for intelligence, and `data/` for storage and events.

**Agents** sit on top and compose building blocks autonomously. An agent can read news, check markets,
generate analysis with AI, and publish to the blog — all by orchestrating the existing building blocks.
No special infrastructure needed; agents are processes that combine what already exists.

### Directory Structure

```
mu/
├── main.go               # Entry point, HTTP routing, server startup
│
│   Subsystems (fundamental infrastructure)
├── app/                  # PWA rendering, HTML templates, CSS, JS
├── api/                  # REST API definitions, MCP server
├── ai/                   # LLM integration (Anthropic Claude)
├── data/                 # Storage, indexing, FTS5 search, event bus
├── auth/                 # Authentication (passkeys, sessions, tokens)
├── admin/                # Admin tools, moderation, logging
│
│   Building Blocks (user-facing features)
├── blog/                 # Microblogging, markdown, ActivityPub
├── news/                 # RSS feeds, HN comments, AI summaries
│   ├── markets/          # Crypto prices, futures (Coinbase API)
│   └── reminder/         # Daily reminders
├── chat/                 # AI chat with RAG, WebSocket streaming
├── video/                # YouTube search and playback
├── mail/                 # Messaging, SMTP server, DKIM signing
├── wallet/               # Credits, payments (Stripe)
├── places/               # Location search (Google Places / OSM)
├── weather/              # Weather forecasts
├── search/               # Web search (Brave API), URL fetching
├── home/                 # Dashboard cards
├── user/                 # User profiles, presence tracking
│
│   Agents (compose building blocks)
├── agent/                # AI agent with MCP tool access
│
├── docs/                 # Documentation
└── scripts/              # Build, deployment, DKIM tools
```

## Subsystems

### App (`app/`)

The rendering and presentation layer. Every building block uses `app/` to serve HTML or JSON responses.

- **Server-side templates** - Go HTML templates for all pages
- **Progressive Web App** - Manifest, icons, mobile install support
- **Static assets** - CSS, JavaScript, embedded at build time
- **Response handling** - JSON for API calls, HTML for browser requests

### API (`api/`)

The interface layer. Defines REST endpoints and exposes the MCP server for AI agent integration.

- **REST API** - Documented endpoints for all building blocks (see `api/api.go`)
- **MCP Server** - Model Context Protocol at `/mcp` for AI tool integration (see `api/mcp.go`)
  - 20+ tools: signup, login, chat, search, blog CRUD, mail, markets, weather, etc.
  - JSON-RPC 2.0 protocol
  - Integrated quota checking via wallet
- **Authentication middleware** - Bearer tokens, session cookies, PAT tokens

### AI (`ai/`)

The intelligence layer. Provides LLM access to any building block that needs it.

- **Anthropic Claude** - Primary LLM provider
- **Model configuration** - Configurable via environment variables
- **Provider abstraction** - Pluggable model providers
- **Search integration** - Context retrieval for grounded responses

### Data (`data/`)

The storage and communication layer. Every building block stores and retrieves data through this subsystem.

- **File storage** - JSON files on disk (default)
- **SQLite with FTS5** - Full-text search for production (`MU_USE_SQLITE=1`)
- **Event system** - Pub/sub for decoupled component communication
- **Indexing** - Priority queue processing for search index

Key functions:
```go
data.Search(query, limit, data.WithType("news"))   // Full-text search
data.Index(entry)                                    // Add to search index
data.Subscribe(eventType, handler)                   // Listen for events
data.Publish(event)                                  // Emit events
```

### Auth (`auth/`)

The identity layer. Handles all authentication and session management.

- **Passkeys (WebAuthn)** - Passwordless authentication
- **Username/password** - Traditional login with hashing
- **Session tokens** - Cookie-based sessions
- **Personal Access Tokens** - For programmatic API access

### Admin (`admin/`)

The operations layer. Server management, moderation, and monitoring.

- **User management** - Create, modify, deactivate accounts
- **Content moderation** - Review queue, flagging
- **Email/API logs** - Delivery tracking, debugging
- **System monitoring** - Memory usage, health checks, ring buffer logs

## Building Blocks

### Blog (`blog/`)

Microblogging with federation support.

- **Markdown posts** - Write and render markdown content
- **ActivityPub** - Federation with Mastodon, Threads, etc. (inbox/outbox, WebFinger)
- **Comments** - Threaded comment system
- **Author controls** - Edit/delete for post authors
- **Daily digests** - AI-generated summaries from trending news

### News (`news/`)

RSS feed aggregation with AI enhancement.

- **Multi-feed support** - Configured in `news/feeds.json`
- **Metadata extraction** - OpenGraph/Twitter card parsing
- **HN integration** - Fetches and indexes Hacker News comments
- **AI summaries** - Article summarization via chat module
- **Full-text search** - Search across all indexed articles

### Chat (`chat/`)

AI-powered conversation with contextual knowledge.

- **RAG** - Retrieves indexed content for grounded responses
- **WebSocket streaming** - Real-time response delivery
- **Multi-topic** - Organized by configurable topics
- **System prompts** - Per-topic personality via `chat/prompts.json`
- **HN context** - Event-driven comment refresh for active discussions

### Video (`video/`)

YouTube integration without ads or tracking.

- **Channel feeds** - Curated channels via `video/channels.json`
- **Search** - YouTube Data API v3
- **Ad-free playback** - Embedded player
- **Recent searches** - Client-side history

### Mail (`mail/`)

Private messaging with full email capability.

- **Internal messaging** - User-to-user, free
- **External email** - SMTP delivery, costs credits
- **SMTP server** - Receives incoming internet mail
- **DKIM signing** - Outbound authentication
- **Spam filtering** - Configurable blocklist
- **Threading** - Conversation view with replies

### Markets (`news/markets/`)

Live financial data.

- **Crypto prices** - Via Coinbase API
- **Futures/commodities** - Real-time market data

### Wallet (`wallet/`)

Credit-based usage metering.

- **Pay as you go** - 20 free credits/day, then 1 credit = 1p
- **Stripe payments** - Card top-up
- **Quota enforcement** - Integrated with API and agent
- **Transaction tracking** - Usage history

### Places (`places/`)

Location search and discovery.

- **Google Places API** - Rich results when API key configured
- **OpenStreetMap fallback** - Free location data
- **Saved categories** - Configurable in `places/locations.json`

### Weather (`weather/`)

Weather forecasts.

- **Google Weather API** - Forecast data
- **Location-based** - Weather by place

### Search (`search/`)

Web search without tracking.

- **Brave Web Search** - Privacy-respecting search API
- **URL fetching** - Fetch and clean web pages for reading
- **No tracking** - No ads, no profiling

### Home (`home/`)

Dashboard overview.

- **Cards** - Configurable summary widgets via `home/cards.json`
- **At-a-glance** - Quick access to all building blocks

## Agents

### Agent (`agent/`)

The agent layer composes building blocks to observe, analyze, and act autonomously.

- **MCP tool access** - Calls the same MCP tools exposed at `/mcp`
- **Conversational interface** - Natural language queries
- **Multi-model** - Standard and premium tier models
- **Credit metering** - Usage tracked via wallet
- **Query history** - Tracks recent interactions

An agent doesn't need special infrastructure — it works by composing existing building blocks.
For example, an opinion agent would: read from news (data), check markets (data), generate
analysis (AI), and publish to the blog (building block). All using the same subsystems and
building blocks that everything else uses.

## Design Patterns

### Event-Driven Architecture

Components communicate via events to avoid tight coupling:

```go
// Chat publishes
data.Publish(data.Event{
    Type: data.EventRefreshHNComments,
    Data: map[string]interface{}{"url": url},
})

// News subscribes
data.Subscribe(data.EventRefreshHNComments, func(event data.Event) {
    news.RefreshHNMetadata(event.Data["url"].(string))
})
```

### Functional Options

Search uses functional options for extensibility:

```go
results := data.Search(query, 20, data.WithType("news"))
```

### Throttling

Expensive operations are throttled (e.g., HN comment refresh limited to once per 5 minutes per room).

## Configuration

All user-configurable data lives in JSON files (embedded at build time):
- `chat/prompts.json` - System prompts for LLM
- `home/cards.json` - Home page cards
- `news/feeds.json` - RSS feed URLs
- `video/channels.json` - YouTube channel IDs
- `places/locations.json` - Saved search categories

## Economic Model

Users can self-host or use the hosted version at mu.xyz. Browsing is free. Searching, posting,
and AI features use credits. 20 free credits per day, then pay as you go at 1 credit = 1p.
This supports development while keeping the platform accessible for casual use.
