# mu

Building blocks for life

# Overview

What are the building blocks for life? Today we have a number of separate services that give us the things 
we need for our daily digital habits; news, social, mail, chat, etc. It's all very disconnected and yet at 
the same time, entirely commercialised. It's very hard to escape the advertising system, cookies, popups, 
and endless noise aka Twitter, reddit, facebook, tiktok, etc. So we're looking to build something new. 

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
