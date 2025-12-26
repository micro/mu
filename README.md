# mu

The Micro Network

## Overview

A personal app platform that provides services without ads, algorithms, or tracking.

### Features

- **App** - Progressive web app for desktop and mobile
- **API** - Basic API for docs and programmatic access
- **Home** - One place to catchup with everything
- **Blog** - Microblogging and community sharing
- **Chat** - AI assistant with contextual discussions
- **Mail** - Private messaging and email inbox 
- **News** - Curated RSS feeds and market data
- **Video** - YouTube search and Ad-free viewing

Mu runs as a single Go binary on your own server or use the hosted version at [mu.xyz](https://mu.xyz).

## Rationale

I'm tired of big tech app platforms. We can't control any of it. Lots of addictive tendencies. It seems easier to turn them into API and data providers and rebuild the UX on top. Starting with blog, chat, news, mail and video.

## Roadmap

Starting with:

- [x] API - Basic API
- [x] App - Basic PWA
- [x] Home - Overview
- [x] Blog - Micro blogging
- [x] Chat - LLM chat UI
- [x] News - RSS news feed
- [x] Video - YouTube search
- [x] Mail - Private messaging 

Coming soon:

- [ ] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

[View more](docs/SCREENSHOTS.md)

## Concepts

Basic concepts. The app contains **cards** displayed on the home screen. These are a sort of summary or overview. Each card links to a **micro app** or an external website. For example the latest Video "more" links to the /video page with videos by channel and search, whereas the markets card redirects to an external app. 

## Free Hosting

**Mu is free to use** at [mu.xyz](https://mu.xyz). Create an account and start using it immediately - no credit card required.

## Paid Hosting

Contact asim@mu.xyz or file an issue to discuss managed hosting.

## Self Hosting

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

For vector search see this [doc](docs/VECTOR_SEARCH.md)

### Run

Then run the app

```
mu --serve
```

Go to localhost:8081

## Documentation

Additional documentation is available in the [docs](docs/) folder:

- [Design Documentation](docs/DESIGN.md) - Architecture and design decisions
- [Messaging System](docs/MESSAGING_SYSTEM.md) - Complete messaging and mail setup guide
- [Environment Variables](docs/ENVIRONMENT_VARIABLES.md) - All configuration options
- [Contextual Discussions](docs/CONTEXTUAL_DISCUSSIONS.md) - Chat context and discussion features
- [Vector Search](docs/VECTOR_SEARCH.md) - Setting up vector embeddings for semantic search
- [Screenshots](docs/SCREENSHOTS.md) - Application screenshots

## Development 

Join [Discord](https://discord.gg/jwTYuUVAGh) if you'd like to work on this.

## Sponsorship 

You can sponsor the project using [GitHub Sponsors](https://github.com/sponsors/asim) or via [Patreon](https://patreon.com/muxyz) to support ongoing development and hosting costs. Patreon members get access to premium features like Mail, early access to new features, and vote on the project roadmap. Most features remain free for all users.

## License

Mu is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

This means you are free to use, modify, and distribute this software, but if you run a modified version on a server and let others interact with it, you must make your modified source code available under the same license.
