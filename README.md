# mu

The Micro Network — apps without ads, algorithms, or tracking.

## Overview

Mu is a collection of micro apps for everyday use. While other platforms monetize your attention with ads and infinite feeds, Mu gives you simple tools that respect your time.

**The problem**: The internet is designed to waste your time. Feeds are infinite, videos autoplay, and everything tracks you.

**The solution**: Small, focused apps that do one thing well. No ads. No algorithms. No tracking.

### Features

- **Home** - Your personalized dashboard
- **Apps** - Build and share micro apps
- **Blog** - Thoughtful microblogging
- **Chat** - Group discussions with AI assistance
- **News** - RSS feeds with summaries
- **Video** - Watch YouTube without ads
- **Notes** - Quick capture with tags, pins, and search
- **Mail** - Private messaging & email
- **Agent** - AI assistant that can use all the above

Mu runs as a single Go binary on your own server or use the hosted version at [mu.xyz](https://mu.xyz).


## Roadmap

Starting with:

- [x] API - Basic API
- [x] App - Basic PWA
- [x] Home - Overview
- [x] Agent - AI assistant
- [x] Apps - Micro app builder
- [x] Blog - Micro blogging
- [x] Chat - LLM chat UI
- [x] News - RSS news feed
- [x] Notes - Personal notes
- [x] Video - YouTube search
- [x] Mail - Private messaging 
- [x] Wallet - Crypto payments (ETH, USDC, ERC-20)
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

### AI Features

Some features are enhanced with AI:

- **News** - Summarize articles
- **Chat** - Knowledge assistant for group discussions
- **Apps** - Generate apps from prompts
- **Agent** - AI assistant with tool access
- **Notes** - Auto-tagging and smart search

## Micro Apps

Mu includes a micro app builder. Create single-page web apps with persistent storage.

### Features

- **Build Apps** - Create apps manually or with AI assistance
- **Persist Data** - Apps can store user data via the Mu SDK
- **Share** - Make apps public for others to use

### SDK

Apps have access to the Mu SDK (`window.mu`):

```javascript
// Persistent storage (per-user, per-app)
await mu.db.get('key');
await mu.db.set('key', value);
await mu.db.delete('key');
await mu.db.list();

// User context
mu.user.id       // User ID or null
mu.user.name     // Display name or null
mu.user.loggedIn // Boolean

// App context
mu.app.id        // App's unique ID
mu.app.name      // App's name
```

### Featured Apps

- [/apps/todo](https://mu.xyz/apps/todo) - Task management
- [/apps/timer](https://mu.xyz/apps/timer) - Focus/pomodoro timer
- [/apps/expenses](https://mu.xyz/apps/expenses) - Expense tracking

### Documentation

See [SDK Documentation](docs/SDK.md) for full API reference.

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

[View more](docs/SCREENSHOTS.md)

## Concepts

Basic concepts. The app contains **cards** displayed on the home screen. These are a sort of summary or overview. Each card links to a **micro app** or an external website. For example the latest Video "more" links to the /video page with videos by channel and search, whereas the markets card redirects to an external app. 

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
git clone https://github.com/asim/mu
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

**Ollama (Default)**

By default, Mu uses [Ollama](https://ollama.ai/) for LLM queries. Install and run Ollama locally:

```
# Install Ollama from https://ollama.ai/
# Pull a model (e.g., llama3.2)
ollama pull llama3.2

# Ollama runs on http://localhost:11434 by default
```

Optional environment variables:
```
export MODEL_NAME=llama3.2              # Default model
export MODEL_API_URL=http://localhost:11434  # Ollama API URL
```

**Fanar (Optional)**

Alternatively, use [Fanar](https://fanar.qa/) by setting the API key:

```
export FANAR_API_KEY=xxx
export FANAR_API_URL=https://api.fanar.qa  # Optional, this is the default
```

When `FANAR_API_KEY` is set, Mu will use Fanar instead of Ollama.

**Note:** Fanar has a rate limit of 10 requests per minute. Mu enforces this limit automatically.

**Anthropic Claude (Optional)**

You can also use Anthropic's Claude API:

```
export ANTHROPIC_API_KEY=xxx
export ANTHROPIC_MODEL=claude-haiku-4.5-20250311  # Optional, this is the default
```

Priority order: Anthropic > Fanar > Ollama

For vector search see this [doc](docs/VECTOR_SEARCH.md)

### Data Storage

By default, Mu stores search index and embeddings in JSON files loaded into memory. For production use with large datasets, enable SQLite storage to reduce memory usage:

```
export MU_USE_SQLITE=1
```

This stores the search index and embeddings in SQLite (`~/.mu/data/index.db`) instead of RAM. Migration from JSON happens automatically on first startup.

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
- [Messaging](docs/MESSAGING_SYSTEM.md) - Email and messaging setup
- [Wallet & Credits](docs/WALLET_AND_CREDITS.md) - Credit system for metered usage
- [Micro Apps SDK](docs/SDK.md) - Build apps with the Mu SDK

**Reference**
- [Configuration](docs/ENVIRONMENT_VARIABLES.md) - All environment variables
- [Vector Search](docs/VECTOR_SEARCH.md) - Semantic search setup
- [API Reference](docs/API_COVERAGE.md) - REST API endpoints
- [Screenshots](docs/SCREENSHOTS.md) - Application screenshots

## Development 

### Git Hooks

Install git hooks to run tests before commits:

```bash
./scripts/install-hooks.sh
```

This will prevent commits if tests fail, helping catch regressions early. See [scripts/README.md](scripts/README.md) for more details.

### Contributing

Join [Discord](https://discord.gg/jwTYuUVAGh) if you'd like to work on this.

## Payments

Mu uses crypto for payments. No credit cards, no payment processors, no KYC.

**Supported chains:**
- Ethereum
- Base
- Arbitrum
- Optimism

**Supported tokens:**
- ETH
- USDC
- Any ERC-20 token

Each user gets a unique deposit address. Send crypto, get credits. 1 credit = 1p.

See [Wallet & Credits](docs/WALLET_AND_CREDITS.md) for details.

## Sponsorship 

You can sponsor the project using [GitHub Sponsors](https://github.com/sponsors/asim) or via [Patreon](https://patreon.com/muxyz) to support ongoing development and hosting costs. Sponsors get early access to new features and can vote on the project roadmap. All features remain free (with daily limits) or pay-as-you-go.

## License

Mu is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

This means you are free to use, modify, and distribute this software, but if you run a modified version on a server and let others interact with it, you must make your modified source code available under the same license.
