# mu

Building blocks for life

# Overview

What are the building blocks for life? There are services we use for our daily digital habits e.g news, video, mail, chat, etc but it's all pretty disconnected and the whole app ecosystem itself is entirely commercialised. We can't escape ads, cookies, popups, paywalls, etc. The noise on twitter, reddit, and facebook doesn't help either. So we're looking to build something new. 

## Features

Starting with:

- [x] API - Basic API
- [x] App - Installable PWA
- [x] Chat - LLM based chat UI
- [x] News - Latest news headlines
- [x] Video - Video search interface

Coming soon

- [ ] Inbox - Direct messaging
- [ ] Wallet - Transact with crypto
- [ ] Utilities - QR code scanner, etc
- [ ] Services - Marketplace of services

## Hosting

Go to [mu.xyz](https://mu.xyz) for the live version

Otherwise see the install guide

## Install

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

### API Keys

We need API keys for:

- OpenAI
- Gemini
- CryptoCompare
- Youtube Data

Export the following env vars

```
export OPENAI_API_KEY=xxx
export GEMINI_API_KEY=xxx
export CRYPTO_API_KEY=xxx
export YOUTUBE_API_KEY=xxx

```

### Run

Then run the app

```
mu --srve
```

Go to localhost:8081
