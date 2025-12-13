# System Design

This document covers the design philosophy, architecture, and technical decisions for Mu.

## Vision

Mu is a personal app platform that attempts to rectify deficiencies with existing social platforms like Twitter, Facebook, and YouTube. 
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
- **File-based storage** - No database required for MVP
- **Progressive Web App** - Mobile-friendly without app stores
- **Monolithic simplicity** - Easy to understand, deploy, and maintain

### Directory Structure

```
mu/
├── main.go           # Entry point, HTTP routing
├── admin/            # Admin tools and flags
├── api/              # REST API definitions and docs
├── app/              # Core app rendering (HTML, CSS, JS, PWA)
├── auth/             # Authentication and user management
├── blog/             # Posts/microblogging
├── chat/             # LLM chat with RAG (Retrieval Augmented Generation)
├── data/             # Data layer (file storage, indexing, vector search, events)
├── home/             # Home page cards
├── mail/             # Messaging system (uses SMTP protocol for delivery)
├── news/             # RSS feeds, HN comments, market data
├── user/             # User profiles and settings
└── video/            # YouTube search and playback
```

## Technical Components

### Data Layer (`data/`)

The data package provides:
- **File Storage** - JSON files on disk for posts, metadata, user data
- **Vector Search** - Embeddings for semantic search across content
- **Event System** - Pub/sub for decoupled component communication
- **Index** - In-memory search index for fast queries

Key functions:
- `Search(query, limit, opts...)` - Semantic + text search with options
- `Index(entry)` - Add content to search index
- `Subscribe(eventType, handler)` / `Publish(event)` - Event bus

### Chat (`chat/`)

AI-powered chat with:
- **RAG (Retrieval Augmented Generation)** - Searches indexed content for relevant context
- **Multi-LLM Support** - Currently Fanar API, designed for Ollama integration
- **Contextual Discussions** - Room-based conversations with topic tracking
- **HN Comment Refresh** - Event-driven updates for active discussions (5-min throttle)

### News (`news/`)

RSS feed aggregation with:
- **Multi-feed Support** - Configured in `news/feeds.json`
- **Metadata Extraction** - OpenGraph/Twitter card parsing
- **HN Integration** - Fetches and indexes Hacker News comments
- **Market Data** - Crypto prices and futures via Coinbase API
- **Search** - Full-text and semantic search across articles

### Video (`video/`)

YouTube integration:
- **Channel Feeds** - Configured in `video/channels.json`
- **Search** - YouTube Data API v3
- **Ad-free Playback** - Embedded player
- **Recent Searches** - LocalStorage-based history

### Blog (`blog/`)

Microblogging platform:
- **Markdown Support** - Posts written in markdown
- **Author Attribution** - Tied to user accounts
- **Edit/Delete** - Author-only modification
- **Optional Write Mode** - `?write=true` parameter

### Messaging (`mail/`)

User-to-user messaging system:
- **Internal Messages** - Direct user-to-user communication
- **External Messages** - Delivered via SMTP protocol to external addresses
- **SMTP Server** - Receives incoming messages from internet
- **DKIM Signing** - Authenticates outbound messages
- **Threading** - Conversation view with replies
- **Access Control** - Restricted to admins and members

### App (`app/`)

Progressive Web App infrastructure:
- **HTML Rendering** - Server-side templates
- **CSS Framework** - Custom responsive styles
- **PWA Manifest** - Install prompt for mobile
- **Service Worker** - Offline capability (TODO)

### API (`api/`)

REST endpoints:
- `GET /news` - News feed
- `POST /news` - Search news
- `GET /video` - Video channels
- `POST /video` - Search videos
- `POST /chat` - AI chat
- `GET /posts` - All posts
- `POST /post` - Create post
- `GET /post?id={id}` - Get post
- `PATCH /post?id={id}` - Update post

See `api/api.go` for full documentation.

## Design Patterns

### Functional Options

Search uses functional options for extensibility:

```go
results := data.Search(query, 20, data.WithType("news"))
```

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

### Throttling

Expensive operations are throttled (e.g., HN comment refresh limited to once per 5 minutes per room).

## Configuration

All user-configurable data lives in JSON files:
- `chat/prompts.json` - System prompts for LLM
- `home/cards.json` - Home page cards
- `news/feeds.json` - RSS feed URLs
- `video/channels.json` - YouTube channel IDs

## Future Work

### Marketplace & Services

Part of building societal fabric is monetary transaction. Future plans include:
- **Wallet** - Credits for usage, payments
- **Services Marketplace** - Backend services accessible via chat or app
- **Agent System** - AI agents that fulfill work by calling services
- **MUCP Protocol** - Micro Communication Protocol for distributed Mu instances (not ActivityPub)

### Economic Model

Users can self-host or use the hosted version at mu.xyz. Optional membership supports development while keeping the platform free for all.

## Agents

More on this soon.

