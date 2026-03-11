# mu

The Micro Network — apps without ads, algorithms, or tracking.

## Overview

Mu is a new social network. While other platforms monetize your attention with ads and infinite feeds, Mu gives you simple utilities that respect your time.

**The problem**: The internet is designed to waste your time. Feeds are infinite, videos autoplay, and everything tracks you.

**The solution**: Small, focused apps that do one thing well. No ads. No algorithms. No tracking.

### Services

- **Home** - At a glance dashboard
- **Agent** - AI assistant across all services
- **Blog** - Blogging with comments
- **Chat** - Discuss topics with AI
- **News** - RSS feeds with summaries
- **Mail** - Private messaging & email
- **Video** - Watch YouTube without ads
- **Markets** - Live crypto, futures & commodities
- **Web** - Search without tracking
- **Wallet** - Pay as you go with credits

Mu runs as a single Go binary on your own server or use the hosted version at [mu.xyz](https://mu.xyz).

## Roadmap

- [x] API - Basic API
- [x] App - Basic PWA
- [x] Home - Overview
- [x] Agent - AI assistant
- [x] Blog - Daily digests
- [x] Chat - Discussion rooms
- [x] News - RSS news feed
- [x] Video - YouTube search
- [x] Mail - Private messaging
- [x] Markets - Live prices
- [x] Web - Web search, no Ads
- [x] Wallet - Card payments
- [ ] Services - Marketplace, etc

### AI Features

Some features are enhanced with AI:

- **Blog** - Daily digests
- **News** - Summarize articles
- **Chat** - Knowledge assistant
- **Agent** - Cross app queries

### MCP — AI Agent Integration

Mu exposes a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server at `/mcp` so AI agents and tools (e.g. Claude Desktop, Cursor, or any MCP-compatible client) can connect directly.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp"
    }
  }
}
```

See [MCP Server docs](docs/MCP.md) for available tools and usage.

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

[View more](docs/SCREENSHOTS.md)

## Concepts

The app contains **cards** displayed on the home screen. These are a summary or overview. Each card links to a feature page or external website. For example the Video card links to the /video page with videos by channel and search.

## Hosting

**Mu is free to start**. See [mu.xyz](https://mu.xyz). Create an account and start using it immediately.

### Self Hosting

Ensure you have [Go](https://go.dev/doc/install) installed

Set your Go bin
```
export PATH=$HOME/go/bin:$PATH
```

Download and install Mu

```
git clone https://github.com/micro/mu
cd mu && go install
```

### Configuration

To reconfigure prompts, topics, cards, etc you can adjust the following json files. 

Note: The binary will need to be recompiled as they are embedded at build time.

#### Chat Prompts

Set the chat prompts in chat/prompts.json

#### Home Cards

Set the home cards in home/cards.json

#### News Feed

Set the RSS news feeds in news/feeds.json

#### Places

Set the saved search categories in `places/locations.json`.

When `GOOGLE_API_KEY` is set, Places uses the [Google Places API (New)](https://developers.google.com/maps/documentation/places/web-service/overview) for richer results. Without it, Places falls back to free OpenStreetMap data.

```
export GOOGLE_API_KEY=xxx
```

#### Video Channels

Set the YouTube video channels in video/channels.json

### API Keys

We need API keys for the following

#### Video Search

- [Youtube Data](https://developers.google.com/youtube/v3)

```
export YOUTUBE_API_KEY=xxx
```

#### Chat Model

Mu uses Anthropic Claude for all AI features:

```
export ANTHROPIC_API_KEY=xxx
export ANTHROPIC_MODEL=claude-sonnet-4-20250514  # Optional, this is the default
```

### Data Storage

By default, Mu stores the search index in JSON files loaded into memory. For production use, enable SQLite with FTS5 full-text search:

```
export MU_USE_SQLITE=1
```

This stores the search index in SQLite (`~/.mu/data/index.db`) with FTS5 for fast full-text search. Migration from JSON happens automatically on first startup.

### Run

Then run the app

```
mu --serve
```

Go to localhost:8081

## Documentation

**On the web:** [mu.xyz/docs](https://mu.xyz/docs) | [mu.xyz/about](https://mu.xyz/about)

Full documentation is available in the [docs](docs/) folder and at `/docs` on any Mu instance:

**Getting Started**
- [About](docs/ABOUT.md) - What Mu is and why it exists
- [Principles](docs/PRINCIPLES.md) - Guiding principles for AI and technology
- [Installation](docs/INSTALLATION.md) - Self-hosting and deployment guide

**Features**
- [ActivityPub](docs/ACTIVITYPUB.md) - Federation with Mastodon, Threads, etc.
- [Messaging](docs/MESSAGING_SYSTEM.md) - Email and messaging setup
- [Wallet & Credits](docs/WALLET_AND_CREDITS.md) - Credit system for metered usage
**Reference**
- [Configuration](docs/ENVIRONMENT_VARIABLES.md) - All environment variables
- [API Reference](docs/API_COVERAGE.md) - REST API endpoints
- [MCP Server](docs/MCP.md) - AI tool integration via MCP
- [Screenshots](docs/SCREENSHOTS.md) - Application screenshots

## Development 

### Git Hooks

Install git hooks to run tests before commits:

```bash
./scripts/install-hooks.sh
```

This will prevent commits if tests fail, helping catch regressions early. See [scripts/README.md](scripts/README.md) for more details.

## Payments

Mu uses Stripe for card payments. Top up with a credit or debit card and pay-as-you-go with credits. 1 credit = 1p.

See [Wallet & Credits](docs/WALLET_AND_CREDITS.md) for details.

## License

Mu is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

This means you are free to use, modify, and distribute this software, but if you run a modified version on a server and let others interact with it, you must make your modified source code available under the same license.
