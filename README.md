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

Mu is a membership based network. Support, features and voting are available by via a subscription fee.

What are the benefits to this? Sustainability of the platform. Ad free, algorithm free, etc. Direct voice to the team building and running the network. 
This goes against the idea of a pricing plan or SaaS which is trying to sell you on something. Instead, yes you pay a monthly fee, but it's to be part of something.

Ideally its just a flat fee, nothing more.

https://mu.xyz/membership

## Try it out 

Go to [mu.xyz](https://mu.xyz) to try it out for free.

## Development 

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
