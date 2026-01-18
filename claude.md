# Mu Development Checkpoint

## What is Mu?
A personal AI platform - utility tools, not a destination. Like Google Search circa 2000: arrive with intent, get answer, leave.

## Philosophy
- **Utility, not destination** - tools that solve problems and get out of the way
- **Pay for what you use** - no engagement tricks, no unlimited tiers
- **Self-host option** - run your own instance for free forever
- **One way of doing things** - no redundant entry points

## Core Building Blocks

| Package | Purpose | Key Exports |
|---------|---------|-------------|
| **ai/** | LLM integration | `Ask()`, `Prompt`, `Message`, `History`, `PriorityHigh/Medium/Low` |
| **api/** | REST API framework | `Register()`, `Endpoint`, `Markdown()` |
| **app/** | Shared utilities | `Log()`, `RenderHTML()`, `WantsJSON()`, `RespondJSON()`, error handlers |
| **data/** | Storage & search | `SaveFile()`, `LoadFile()`, `Index()`, `Search()`, `Publish()`, `Subscribe()` |

All feature packages build on these four.

## AI Package (`ai/`)
Single source of truth for LLM integration:
- `ai/ai.go` - Types (`Prompt`, `History`, `Message`), constants, `Ask()`
- `ai/providers.go` - Anthropic, Fanar, Ollama with rate limiting

Provider priority: Anthropic > Fanar > Ollama (based on env vars)

## Feature Packages

| Package | Purpose | AI Integration |
|---------|---------|----------------|
| **agent/** | AI assistant via @micro button | Multi-step tool execution |
| **apps/** | Micro app builder | Generate/modify from prompts |
| **blog/** | Microblogging | Auto-tag posts |
| **chat/** | Contextual chat rooms | RAG-powered Q&A |
| **mail/** | Email/messaging | Agent tools (send, check inbox) |
| **news/** | RSS aggregation | Auto-summarize articles |
| **notes/** | Personal notes | Auto-tag, smart search (RAG) |
| **video/** | YouTube integration | Search only |
| **wallet/** | Credits system | N/A |

## Agent Tools (`agent/tools.go`)
- `video_search`, `video_play` - YouTube
- `news_search`, `news_read` - News articles  
- `app_create`, `app_modify`, `app_list` - Micro apps
- `market_price` - Crypto/stock prices
- `save_note`, `search_notes`, `list_notes` - Notes
- `send_email`, `check_inbox` - Mail

## Notes App (`/notes`)
Google Keep replacement:
- Quick capture with optional title
- Tags (auto-generated via AI if not provided)
- Pin, archive, color coding
- Smart search using RAG
- Grid view

Storage: `$HOME/.mu/data/notes.json`

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
- Floating `@` button = universal AI entry point
- Minimal chrome, left-aligned forms
- No redundant navigation paths
- Consistent content width across pages
