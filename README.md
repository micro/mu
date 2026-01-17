# mu

A personal AI platform — without ads

## Overview

Mu is your private AI-powered home on the internet. While other platforms monetize your attention with ads (yes, even ChatGPT now), Mu helps you consume less, not more.

**The problem**: The internet is designed to waste your time. Feeds are infinite, videos autoplay, and AI assistants are becoming ad-supported.

**The solution**: AI that works for you, not advertisers. Summarize instead of scroll. Ask instead of search. Create instead of consume.

### Features

- **Chat** - AI assistant with context-aware conversations
- **Agent** - AI that executes tasks across your data
- **Apps** - Build micro apps from natural language
- **News** - AI-curated feeds with summaries
- **Video** - Ad-free viewing with AI summaries
- **Mail** - Private email with AI assistance  
- **Blog** - Thoughtful microblogging
- **Home** - Your personalized dashboard

### AI Everywhere

| Before | After with Mu |
|--------|---------------|
| Read 50 news articles | AI summary in 2 minutes |
| Watch 10 YouTube videos | Key points extracted instantly |
| Write email from scratch | AI drafts, you refine |
| Build a simple app | Describe it, AI creates it |
| Search through content | Ask questions, get answers |

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
- [x] Video - YouTube search
- [x] Mail - Private messaging 
- [x] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

### AI Opportunities

The goal is to reduce internet consumption by 10x through AI. Here's how AI can enhance each feature:

| Feature | Current State | AI Opportunity | Impact |
|---------|--------------|----------------|--------|
| **News** | ✅ AI summaries | Already integrated | High - Read 50 articles in 2 min |
| **Chat** | ✅ LLM chat | Already integrated | High - Get answers instantly |
| **Apps** | ✅ AI builder | Already integrated | High - Build apps from prompts |
| **Agent** | ✅ AI assistant | Already integrated | High - Automated workflows |
| **Video** | ⏳ Search only | Summarize videos, extract key points, Q&A on content | High - 10 videos → 2 min summary |
| **Blog** | ⏳ Manual posts | Intention prompts, clarity assistance, kindness hints | Medium - Thoughtful creation |
| **Mail** | ⏳ Manual compose | Draft replies, summarize threads, tone awareness | Medium - 100 emails → 5 min |
| **Wallet** | ✅ Credit system | Infrastructure, AI not applicable | N/A - Payment tracking |

**Priority order for AI integration:**
1. **Video** - Highest impact; people spend hours watching when a summary would suffice
2. **Mail** - High value for power users; AI-drafted replies with tone awareness
3. **Blog** - Focus on intentionality; AI prompts reflection, not engagement

**Philosophy:** AI is a tool, not a destination. It should reduce screen time, not extend it. See [Principles](docs/PRINCIPLES.md) for details.

## Micro Apps

Mu includes an AI-powered micro app builder. Users can create single-page web apps from natural language prompts.

### Features

- **Generate Apps** - Describe what you want, AI builds it
- **Iterate** - Refine with follow-up instructions
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

## Sponsorship 

You can sponsor the project using [GitHub Sponsors](https://github.com/sponsors/asim) or via [Patreon](https://patreon.com/muxyz) to support ongoing development and hosting costs. Patreon members get access to premium features like Mail, early access to new features, and vote on the project roadmap. Most features remain free for all users.

## License

Mu is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

This means you are free to use, modify, and distribute this software, but if you run a modified version on a server and let others interact with it, you must make your modified source code available under the same license.
