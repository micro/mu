# Telegram

Mu includes a Telegram bot that gives you access to the AI agent directly from chat. Ask questions, check markets, get news — all from Telegram.

## Setup (Self-hosting)

1. Message [@BotFather](https://t.me/BotFather) on Telegram
2. Send `/newbot`, follow the prompts, copy the token
3. Set the token in Mu: go to `/admin/env` and paste it under **Telegram → TELEGRAM_BOT_TOKEN**

The bot connects automatically within 30 seconds. No webhook, no exposed ports — it uses long polling.

## Using the Bot

### First time

DM the bot or use a command in a group. If you don't have a Mu account, one is created automatically from your Telegram username.

### Linking an existing account

DM the bot:
```
link myusername mypassword
```
Only works in DMs for security.

### Direct messages

Just send any message to the bot:
- "What's the BTC price?"
- "Read my email"
- "Today's news"

### Groups

In groups, use commands or @mention the bot:
- `/ask what's happening in crypto?`
- `/news`
- `/markets`
- `/weather London`
- `@yourbotname summarise today's news`

The bot has privacy mode — it only sees commands and @mentions in groups, not every message.

### Commands

| Command | Description |
|---------|-------------|
| `/ask <question>` | Ask the AI agent anything |
| `/news` | Latest news headlines |
| `/markets` | Live crypto market prices |
| `/weather <location>` | Weather forecast |
| `/usage` | Your query usage stats |

Commands are registered with Telegram and appear in the `/` menu.

### Unlinking

DM the bot `unlink` to disconnect your Telegram from your Mu account.

## Notifications

The bot DMs you when:
- You receive a **new email** (with AI-generated summary)
- A **trading strategy signal** triggers

## Privacy

- **Direct messages** have full access to your data (mail, wallet)
- **Group messages** are public — no private data is included in responses
- **Credentials** (`link` command) only work in DMs, never in groups
- **Auto-created accounts** get a random password — you authenticate via Telegram

## How it works

The bot uses Telegram's long polling API — no webhook needed. It dials out to Telegram's servers, so it works behind NAT and firewalls with no port forwarding. The bot reconnects automatically if the connection drops.
