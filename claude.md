# Mu Development Checkpoint

## What is Mu?
A personal AI platform - utility tools, not a destination. Like Google Search circa 2000: arrive with intent, get answer, leave.

## Philosophy
- **Utility, not destination** - tools that solve problems and get out of the way
- **Pay for what you use** - no engagement tricks, no unlimited tiers
- **Self-host option** - run your own instance for free forever
- **One way of doing things** - no redundant entry points

## Architecture

### AI Package (`ai/`)
Reusable LLM integration extracted from chat:
- `ai/ai.go` - Types (Prompt, History, Message), constants (PriorityHigh/Medium/Low), Ask()
- `ai/providers.go` - Anthropic, Fanar, Ollama support with rate limiting

Provider priority: Anthropic > Fanar > Ollama (based on env vars)

### Features with AI Integration

| Feature | AI Integration |
|---------|---------------|
| Chat | RAG-powered Q&A with context |
| Agent | Multi-step tool execution via @micro floating button |
| Apps | Generate/modify HTML/CSS/JS from prompts |
| Notes | Auto-tagging, smart search (RAG) |
| News | Auto-summarize articles |
| Blog | Auto-tag posts |
| Mail | Agent tools (send_email, check_inbox) |

### Agent Tools
Located in `agent/tools.go`:
- `video_search`, `video_play` - YouTube
- `news_search`, `news_read` - News articles
- `app_create`, `app_modify`, `app_list` - Micro apps
- `market_price` - Crypto/stock prices
- `save_note`, `search_notes`, `list_notes` - Notes
- `send_email`, `check_inbox` - Mail

## Notes App (`/notes`)
Full-featured notes to replace Google Keep:
- Quick capture with optional title
- Tags (auto-generated if not provided)
- Pin, archive, color coding
- Smart search using RAG
- Grid view like Keep

Key files:
- `notes/notes.go` - Data model, CRUD, search, auto-tagging
- `notes/handlers.go` - HTTP handlers, UI

Storage: `$HOME/.mu/data/notes.json`

## Pricing Model (Pay-as-you-go)

| Feature | Cost |
|---------|------|
| News search | 1p |
| News summary | 1p |
| Video search | 2p |
| Video watch | Free |
| Chat query | 3p |
| Chat room | 1p |
| External email | 4p |
| App create | 5p |
| App modify | 3p |
| Agent run | 5p |

- **Free tier**: 10 AI queries/day
- **Admins**: Full access (no quotas)

## Key Packages

### Core
- `ai/` - LLM integration (Anthropic, Fanar, Ollama)
- `app/` - Shared utilities, HTML rendering, static files
- `auth/` - Accounts, sessions, tokens
- `data/` - Storage, indexing, pub/sub events

### Features
- `agent/` - AI agent with tools
- `apps/` - Micro app builder
- `blog/` - Microblogging
- `chat/` - Topic-based chat rooms
- `mail/` - Email/messaging
- `news/` - RSS aggregation
- `notes/` - Personal notes
- `video/` - YouTube integration
- `wallet/` - Credits system

## Environment Variables

```bash
# LLM Provider (priority order)
ANTHROPIC_API_KEY    # Use Claude
ANTHROPIC_MODEL      # Default: claude-haiku-4-20250514

FANAR_API_KEY        # Use Fanar
FANAR_API_URL        # Default: https://api.fanar.qa

MODEL_NAME           # Ollama model (default: llama3.2)
MODEL_API_URL        # Ollama URL (default: http://localhost:11434)

# Stripe (for credit top-ups)
STRIPE_SECRET_KEY
STRIPE_PUBLISHABLE_KEY
STRIPE_WEBHOOK_SECRET

# Quotas
FREE_DAILY_SEARCHES=10
CREDIT_COST_NEWS=1
CREDIT_COST_VIDEO=2
CREDIT_COST_CHAT=3
CREDIT_COST_EMAIL=4
CREDIT_COST_APP_CREATE=5
CREDIT_COST_APP_MODIFY=3
CREDIT_COST_AGENT=5
```

## Git Remote
- `git@github.com:asim/mu.git`
- Deploy key configured for this VM

## Testing
- `shelleytest` - admin account for testing on mu.xyz
- SSH: `ssh -p 61194 mu@mu.xyz`

## UI Principles
- Floating `@` button is the universal AI entry point (all pages)
- Notes editor: minimal chrome, left-aligned, Keep-like
- No redundant navigation (one way to reach each feature)
- Content width consistent across pages
