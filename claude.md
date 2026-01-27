# Mu Development Checkpoint

> See [PURPOSE.md](PURPOSE.md) for design philosophy, spiritual grounding, and the "environment principle."

## What is Mu?
The Micro Network - simple, useful tools without ads, algorithms, or tracking. Like Google Search circa 2000: arrive with intent, get what you need, leave.

## Philosophy
- **Utility, not destination** - tools that solve problems and get out of the way
- **Anti-addiction** - reduce screen time, opposite of engagement-driven platforms
- **AI as compression** - summarization not generation, helps you leave faster
- **Pay for what you use** - no ads, no subscriptions, no engagement tricks
- **Self-host option** - run your own instance for free forever

## Value Proposition
No ads, no algorithms, no tracking. Simple tools that respect your time.

## Core Features
- **Notes** - Quick capture with tags, pins and search
- **Mail** - Message other users, receive external email
- **News** - RSS aggregation with AI summaries
- **Video** - YouTube without ads/algorithms/shorts
- **Chat** - Public discussion rooms
- **Blog** - Microblogging

## Core Building Blocks

| Package | Purpose |
|---------|---------|
| **app/** | Base template, UI helpers, routing |
| **api/** | REST API framework |
| **data/** | Storage, search, pub/sub |
| **auth/** | Session, tokens |

## Feature Packages

| Package | Purpose |
|---------|---------|
| **notes/** | Personal notes with tags, search |
| **mail/** | Email (SMTP server + client) |
| **news/** | RSS aggregation + summaries |
| **video/** | YouTube search/play |
| **chat/** | Public chat rooms |
| **blog/** | Microblogging |
| **kids/** | Safe video for children |
| **wallet/** | Credits + crypto deposits |
| **widgets/** | Markets + Reminder cards |

## Widgets (widgets/)
- `widgets/markets.go` - Crypto/futures price ticker
- `widgets/reminder.go` - Daily Islamic reminder

## Home Page Cards
Configurable via `home/cards.json`. Cards: blog, chat, news, markets, reminder, video.

## Pricing (Pay-as-you-go)

| Feature | Cost |
|---------|------|
| News search/summary | 1p |
| Video search | 2p |
| Chat query | 3p |
| External email | 4p |

Free tier: 10 AI queries/day

## Environment Variables

```bash
# LLM Providers
FANAR_API_KEY / FANAR_API_URL    # Default for chat, summaries
ANTHROPIC_API_KEY / ANTHROPIC_MODEL
MODEL_NAME / MODEL_API_URL       # Ollama fallback

# Crypto Wallet (optional)
WALLET_SEED  # BIP39 mnemonic
BASE_RPC_URL  # Default: https://mainnet.base.org
```

## Git & Deployment
- Remote: `git@github.com:micro/mu.git`
- **Production**: https://mu.xyz
- **SSH Access**: `ssh -p 61194 mu@mu.xyz`
- **Deploy**: Push to main → GitHub Action auto-deploys
- Data directory: `~/.mu/` on the server

## UI Principles
- Minimal chrome, left-aligned forms
- No redundant navigation paths
- Consistent content width across pages
- Service worker version bumps to clear cache

## UI Helpers (app/ui.go)

### Layout Helpers
```go
app.SearchBar("/notes", "Search notes...", query)
app.ActionLink("/notes/new", "+ New Note")
app.Grid(content)   // .card-grid - responsive grid
app.List(content)   // .card-list - vertical stack  
app.CardDiv(content)
app.Empty("No items yet")
```

### Element Builders
```go
app.Title("My Note", "/notes/123")
app.Desc("Description text")
app.Meta("by author · 2h ago")
app.Tags([]string{"tag1", "tag2"}, "/notes?tag=")
```

## Crypto Wallet

HD wallet for deposits:
- Unique deposit address per user (BIP32 derivation)
- Multi-chain support: Ethereum, Base, Arbitrum, Optimism
- Token price lookup via CoinGecko
- Credits: 1 credit = 1 penny

## UI State
- Service worker version: v125
- Sidebar: scrollable nav + fixed bottom (account/logout)
- Chat: public rooms by topic

## Removed Features (January 2026)
Simplified by removing:
- **agent/** - AI assistant orchestration
- **apps/** - App builder/generator
- **flow/** - Automation language
- **tools/** - Tool registry

These were ambitious but didn't work well enough. Focus on core features first.
