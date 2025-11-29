# mu

The Micro Network

# Overview

Big tech failed us. They now fuel addictive behaviour to drive profit above all else. The tools no longer work for us, instead we work for them. 
Let's rebuild these services without ads, algorithms or exploits for a better way of life.

## Features

Starting with:

- [x] API - Basic API
- [x] App - Basic PWA
- [x] Chat - LLM chat UI
- [x] News - RSS news feed
- [x] Video - YouTube search
- [x] Posts - Micro blogging

Coming soon:

- [ ] Mail - Private inbox
- [ ] Wallet - Credits for usage
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Membership

Mu is a membership based network. Support, features and voting are available by via a subscription fee.

What are the benefits to this? Sustainability of the platform. Ad free, algorithm free, etc. Direct voice to the team building and running the network. 
This goes against the idea of a pricing plan which is trying to sell you on something. Instead you pay monthly, but to support the tools.

Ideally its just a flat fee, nothing more.

https://mu.xyz/membership

## Try it out 

Go to [mu.xyz](https://mu.xyz) to try it out for free.

## Screenshots

The homepage

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

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
