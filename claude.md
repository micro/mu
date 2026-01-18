# Mu Development Checkpoint

## What is Mu?
The Micro Network - apps without ads, algorithms, or tracking. Utility tools, not a destination. Like Google Search circa 2000: arrive with intent, get what you need, leave.

## Philosophy
- **Utility, not destination** - tools that solve problems and get out of the way
- **Anti-addiction** - reduce screen time, opposite of engagement-driven platforms
- **AI as compression** - summarization not generation, helps you leave faster
- **Pay for what you use** - no ads, no subscriptions, no engagement tricks
- **Self-host option** - run your own instance for free forever

## Value Proposition
No ads, no algorithms, no tracking. Simple apps that respect your time.

1. **Micro Apps** - Small, focused tools that do one thing well
2. **Consolidation** - Notes, email, news, video, chat, apps in one place
3. **AI When Useful** - Summarization, app generation, agent assistance
4. **Pay-as-you-go, No Ads** - You're the customer, not the product
5. **Self-hostable** - Single binary, your data stays yours

## Core Building Blocks

| Package | Purpose | Key Exports |
|---------|---------|-------------|
| **ai/** | LLM integration | `Ask()`, `Prompt`, `Message`, `History`, `PriorityHigh/Medium/Low` |
| **api/** | REST API framework | `Register()`, `Endpoint`, `Markdown()` |
| **app/** | Shared utilities | `Log()`, `RenderHTML()`, `WantsJSON()`, `RespondJSON()`, error handlers |
| **data/** | Storage & search | `SaveFile()`, `LoadFile()`, `Index()`, `Search()`, `Publish()`, `Subscribe()` |

## Feature Packages

| Package | Purpose | AI Integration |
|---------|---------|----------------|
| **agent/** | AI assistant via @micro button | Multi-step tool execution |
| **apps/** | Micro app builder + built-ins | Generate/modify from prompts |
| **blog/** | Microblogging | Auto-tag posts |
| **chat/** | Contextual chat rooms | RAG-powered Q&A |
| **mail/** | Email/messaging | Agent tools (send, check inbox) |
| **news/** | RSS aggregation | Auto-summarize articles |
| **notes/** | Personal notes | Auto-tag, smart search (RAG) |
| **video/** | YouTube integration | Search only |
| **wallet/** | Credits system | N/A |

## Built-in Apps (apps/)
- `apps/markets.go` - Crypto/futures price ticker, self-contained with own data fetcher
- `apps/reminder.go` - Daily Islamic reminder, self-contained with own data fetcher

Both moved from news package to apps package for better organization.

## Home Page Card Customization
- Client-side card visibility toggle
- "Customize" link on home page for logged-in users
- Modal with checkboxes to show/hide: Apps, News, Reminder, Markets, Blog, Video
- Preferences saved to localStorage (`mu_hidden_cards`)
- Cards hidden on page load via `applyHiddenCards()`

## Agent Tools (`agent/tools.go`)
- `video_search`, `video_play` - YouTube
- `news_search`, `news_read` - News articles  
- `app_create`, `app_modify`, `app_list` - Micro apps
- `market_price` - Crypto/stock prices (uses `apps.GetAllPrices()`)
- `save_note`, `search_notes`, `list_notes` - Notes
- `send_email`, `check_inbox` - Mail

## Pricing (Pay-as-you-go)

| Feature | Cost |
|---------|------|
| News search/summary | 1p |
| Video search | 2p |
| Chat query | 3p |
| External email | 4p |
| App create | 5p |
| App modify | 3p |
| Agent run | 5p |

Free tier: 10 AI queries/day

## Environment Variables

```bash
# LLM Providers (checked in order)
ANTHROPIC_API_KEY / ANTHROPIC_MODEL
FANAR_API_KEY / FANAR_API_URL  
MODEL_NAME / MODEL_API_URL  # Ollama

# Stripe
STRIPE_SECRET_KEY
STRIPE_PUBLISHABLE_KEY
STRIPE_WEBHOOK_SECRET
```

## Git
- Remote: `git@github.com:asim/mu.git`
- Test: `shelleytest` admin on mu.xyz
- SSH: `ssh -p 61194 mu@mu.xyz`

## UI Principles
- Minimal chrome, left-aligned forms
- No redundant navigation paths
- Consistent content width across pages
- Service worker version bumps to clear cache (currently v95)

## Recent Changes
- Moved markets/reminder from news to apps package
- Added home card customization (client-side localStorage)
- Removed agent modal FAB (to be repositioned later)
- Fixed chat page layout on mobile (100svh, proper offsets)

## Next Up
- App templates (API fetcher, data tracker patterns)
