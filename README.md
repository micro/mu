# mu

Building blocks for life

# Overview

What are the building blocks for life? Today there are a number of services that give us the things we need for our daily digital habits e.g news, social, mail, chat, etc but it's all very disconnected. Yet at the same time, the whole thing is entirely commercialised. It's very hard to escape the machine; ads, cookies, popups. 
And the endless noise of Twitter, reddit, facebook, tiktok, etc isn't helping. So we're looking to build something new. 

## Features

Starting with:

- API - Basic API
- App - Installable PWA
- Chat - LLM based chat UI

Coming soon

- News - Social feed/news
- Inbox - Direct messaging
- Wallet - Transact with crypto
- Utilities - QR code scanner, etc
- Services - Marketplace of services


## Usage

Go to [mu.xyz](https://mu.xyz)

Otherwise install Go then;

```
git clone https://github.com/micro/mu
cd mu
go install
```

Export API key

```
export OPENAI_API_KEY=xxx
```

Run it

```
mu --serve
```
