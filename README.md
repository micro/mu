# mu

The Muslim Network

# Overview

What are the building blocks for life? There are services we use for our daily digital habits e.g news, video, mail, chat, etc but it's all pretty exploitative and addictive now because of big tech. We can't escape ads, cookies, popups, paywalls, doomscrolling, swiping, algorithms. X, Instagram, YouTube, TikTok, etc only make it worse, a form of usury and profiteering. So we're looking to build something new, with Islamic values.

## Features

Starting with:

- [x] API - Basic API
- [x] App - Installable PWA
- [x] Chat - LLM based chat UI
- [x] News - Latest news headlines
- [x] Video - Video search interface

Coming soon

- [ ] Blog - Micro blogging 
- [ ] Inbox - Web notifications
- [ ] Wallet - Credits for usage
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

We need API keys for the following

#### Video

- [Youtube Data](https://developers.google.com/youtube/v3)

```
export YOUTUBE_API_KEY=xxx
```

#### Models

Usage requires

- [OpenAI](https://openai.com) - for embeddings
- [Fanar](https://fanar.qa/) - for llm queries

```
export OPENAI_API_KEY=xxx
export FANAR_API_KEY=xxx
```

### Run

Then run the app

```
mu --serve
```

Go to localhost:8081
