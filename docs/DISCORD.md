# Discord

Mu includes a Discord bot that gives you access to the AI agent directly from chat. Ask questions, check markets, get news, trade tokens — all without leaving Discord.

## Setup (Self-hosting)

1. Go to [discord.com/developers/applications](https://discord.com/developers/applications)
2. Create a new application
3. Go to **Bot** → click **Reset Token** → copy the token
4. Enable **Message Content Intent** under Privileged Gateway Intents
5. Set the token in Mu: go to `/admin/env` and paste it under **Discord → DISCORD_BOT_TOKEN**, or set the environment variable
6. Invite the bot to your server:
   ```
   https://discord.com/oauth2/authorize?client_id=YOUR_APP_ID&scope=bot&permissions=2048
   ```
   Replace `YOUR_APP_ID` with the Application ID from the General Information page.

The bot connects automatically when the token is set. No restart needed.

## Using the Bot

### First time

Just DM the bot or @mention it in a channel. If you don't have a Mu account, one is created automatically from your Discord username. You're ready to go immediately.

### Linking an existing account

If you already have a Mu account and want to link it:

**Option 1 — DM with credentials:**
```
link myusername mypassword
```
Only works in DMs (not channels) for security.

**Option 2 — Link code:**
1. Go to `/account` on your Mu instance
2. Click **Generate Link Code**
3. DM the bot: `link a3f7b201`

### Talking to the bot

DM or @mention the bot with anything:
- "What's the BTC price?"
- "Read my email"
- "Today's news"
- "Weather in London"
- "Swap 0.1 ETH for USDC"

The bot remembers the last 10 messages for conversation context, so follow-ups work naturally.

### Slash commands

The bot registers these slash commands automatically:

| Command | Description |
|---------|-------------|
| `/agent` | Ask the AI agent anything |
| `/news` | Latest news headlines |
| `/markets` | Live market prices (crypto, futures, commodities) |
| `/weather` | Weather forecast with optional location |
| `/mail` | Check your inbox |
| `/swap` | Get a swap quote with token dropdowns |
| `/balance` | Trading wallet balances |
| `/apps` | Browse or search apps |
| `/social` | Social feed |
| `/video` | Search videos |
| `/blog` | Latest blog posts |
| `/search` | Search across all content |

Slash commands show a "thinking" indicator while the agent processes, then respond with a rich embed.

### Unlinking

DM the bot `unlink` to disconnect your Discord from your Mu account.

## Notifications

The bot proactively DMs you when:
- A **trading strategy signal** triggers (alert or confirm mode)
- A **trade auto-executes** (auto mode) — includes the reason and tx hash

## Rich Embeds

Responses are formatted as Discord embeds with color coding:
- 🟣 Purple — News
- 🟢 Green — Markets and prices
- 🟡 Gold — Weather and trading
- 🔴 Red — Mail and video
- 🔵 Blue — General, social, blog

## Security

- **Auto-created accounts** get a random password. The user authenticates via Discord — they never need the password unless they want to log in on the web.
- **Link with credentials** only works in DMs, never in channels.
- **Link codes** expire after 5 minutes and are single-use.
- All data access is scoped to the linked Mu account — one user can't access another's mail, wallet, or trades.

## Multiple Servers

The bot works across any number of Discord servers. Each user links their own Mu account independently. The bot only responds to DMs and @mentions — it doesn't read all channel messages.
