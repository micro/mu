# mu

The Micro Network

# Overview

Big tech failed us. They now fuel addictive behaviour to drive profit above all else. The tools no longer work for us, instead we work for them. 
Let's rebuild these services without ads, algorithms or exploits for a better way of life.

## Features

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

## Try it out 

Go to [mu.xyz](https://mu.xyz) to try it out for free.

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

### Chat

<img width="2768" height="1524" alt="Screenshot 2025-11-30 08 07 04" src="https://github.com/user-attachments/assets/7fd99da9-f2c0-49d5-a780-5b54009ad474" />

### News

<img width="2768" height="1524" alt="Screenshot 2025-11-30 08 07 09" src="https://github.com/user-attachments/assets/fd6d9490-96f5-4c22-ba43-459295abb090" />

### Posts

<img width="2768" height="1524" alt="Screenshot 2025-11-30 08 07 15" src="https://github.com/user-attachments/assets/9698d7f0-3df6-42b1-a046-67c7e09c6a11" />

### Video

<img width="2768" height="1524" alt="Screenshot 2025-11-30 08 07 57" src="https://github.com/user-attachments/assets/1cc7c97e-b0f7-4eed-9414-b9cf2933d0d9" />

### Account

<img width="2768" height="1524" alt="Screenshot 2025-11-30 08 08 04" src="https://github.com/user-attachments/assets/e0ee732c-733a-41b9-ba85-9bbfeafc5503" />

## Concepts

Basic concepts. The app contains **cards** displayed on the home screen. These are a sort of summary or overview. Each card links to a **micro app** or an external website. For example the latest Video "more" links to the /video page with videos by channel and search, whereas the markets card redirects to an external app. 

There are built in cards and then the idea would be that you could develop or include additional cards or micro apps through configuration or via some basic gist like code editor. Essentially creating a marketplace.

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

### Chat Prompts

Set the chat prompts in chat/prompts.json

### Home Cards

Set the home cards in home/cards.json

### News Feed

Set the RSS news feeds in news/feeds.json

### Video Channels

Set the YouTube video channels in video/channels.json

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
- Ollama - TODO

```
export FANAR_API_KEY=xxx
```

For vector search see this [doc](VECTOR_SEARCH.md)

### Run

Then run the app

```
mu --serve
```

Go to localhost:8081
