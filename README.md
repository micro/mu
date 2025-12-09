# mu

The Micro Network

## Overview

A personal app platform that provides services without ads, algorithms, or exploits.

### Features

- **Home** - One place to catchup with everything
- **Chat** - AI assistant with contextual discussions
- **News** - Curated RSS feeds and market data
- **Posts** - Microblogging and community sharing
- **Video** - YouTube search and Ad-free viewing
- **App** - A progressive web app for mobile
- **API** - REST API for programmatic access

Mu runs as a single Go binary on your own server or use the hosted version at [mu.xyz](https://mu.xyz).

## Motivation

Technology should empower people, not exploit them. Mu is built with the idea that tools should serve humanity,
respect privacy and enable consumption without addiction, exploitation or manipulation.

## Roadmap

Starting with:

- [x] API - Basic API
- [x] App - Basic PWA
- [x] Home - Overview
- [x] Chat - LLM chat UI
- [x] News - RSS news feed
- [x] Video - YouTube search
- [x] Posts - Micro blogging

Coming soon:

- [ ] Mail - Private inbox
- [ ] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

[View more](docs/SCREENSHOTS.md)

## Concepts

Basic concepts. The app contains **cards** displayed on the home screen. These are a sort of summary or overview. Each card links to a **micro app** or an external website. For example the latest Video "more" links to the /video page with videos by channel and search, whereas the markets card redirects to an external app. 

There are built in cards and then the idea would be that you could develop or include additional cards or micro apps through configuration or via some basic gist like code editor. Essentially creating a marketplace.

## Hosted Version

**Mu is free to use** at [mu.xyz](https://mu.xyz). Create an account and start using it immediately - no credit card required.

Optional membership is available to support ongoing development and hosting costs. Members get early access to new features and a voice in the project's direction. This is entirely optional - the platform remains free for all users.

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

#### LLM Model

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
- [Contextual Discussions](docs/CONTEXTUAL_DISCUSSIONS.md) - Chat context and discussion features
- [Vector Search](docs/VECTOR_SEARCH.md) - Setting up vector embeddings for semantic search
- [Screenshots](docs/SCREENSHOTS.md) - Application screenshots

## Development 

Join [Discord](https://discord.gg/jwTYuUVAGh) if you'd like to work on this. Read the [code of ethics](https://github.com/asim/mu/issues/52) issue first.

## License

Mu is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

This means you are free to use, modify, and distribute this software, but if you run a modified version on a server and let others interact with it, you must make your modified source code available under the same license.
