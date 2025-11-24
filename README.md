# mu

The Micro Network

# Overview

Big tech failed us. They now fuel addictive behaviour to drive profit above all else. The tools no longer work for us, instead we feed them. 
We're rebuilding services without ads, algorithms or exploits for a better way of life.

## Features

Starting with:

- [x] API - Basic API
- [x] App - Installable PWA
- [x] Chat - LLM based chat UI
- [x] News - RSS news headlines
- [x] Video - YouTube API search
- [x] Posts - Micro blogging like X

Coming soon:

- [ ] Mail - Email without Gmail
- [ ] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Membership

See [MEMBERSHIP.md](MEMBERSHIP.md) for information on how we plan to make this project sustainable.

## Usage

Go to [mu.xyz](https://mu.xyz) for the live version

## Install

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

### API Keys

We need API keys for the following

#### Video Search

- [Youtube Data](https://developers.google.com/youtube/v3)

```
export YOUTUBE_API_KEY=xxx
```

#### LLM Model

Usage requires

- [Fanar](https://fanar.qa/) - for llm queries

```
export FANAR_API_KEY=xxx
```

### Run

Then run the app

```
mu --serve
```

Go to localhost:8081
