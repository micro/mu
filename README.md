# mu

Building blocks for life

# Overview

What are the building blocks for life? Today there are a number of services we use for our daily digital habits e.g news, social, mail, chat, etc but it's all very disconnected. Yet at the same time, the whole thing is entirely commercialised. It's very hard to escape the machine aka ads, cookies, popups. The endless noise on Twitter, reddit, facebook and tiktok doesn't help either. So we're looking to build something new. 

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

## Payments

One of the core things we'll look to feature is crypto payments. Using micro transactions for everything this should enable a sort of anti spam filtering but also promote a self sustainable model for long term maintenance and provide some sort of financial incentive to users for tipping good behaviour. In the long long term its also a way to charge for services.

## Usage

Go to [mu.xyz](https://mu.xyz)

Otherwise install Go then;

```
git clone https://github.com/micro/mu
cd mu
go install
```

## Keys

Export OpenAI API key for chat

```
export OPENAI_API_KEY=xxx
```

Export CryptoCompare API key for market data

```
export CRYPTO_API_KEY=xxx
```

## Run

Export path

```
export PATH=$HOME/go/bin:$PATH
```

Run it

```
mu --serve
```

Go to localhost:8081
