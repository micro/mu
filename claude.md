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
- **Extracted inline styles to mu.css** - Reduced from 309 to 62 inline styles
  - Added 50+ utility classes (display, flex, text, spacing, etc.)
  - Component classes: .progress, .stats-table, .data-table, .blog-form, .mail-form
  - Button variants: .btn, .btn-primary, .btn-outline, .btn-secondary
  - Alert classes: .alert-error, .alert-success, .alert-info
  - Remaining inline styles are dynamic values, JS toggles, iframe positioning
- Rebranded to "The Micro Network" - apps-first positioning
- Agent hero input on home page (subtle, in left column)
- Customize link moved next to page title
- Restructured /admin as dashboard with links
- Standardized button padding, color tokens, search bar styling
- Agent is task executor only - redirects general questions to Chat

## Card System (cards/)

Reusable UI components for all data types. ONE place to define how each type renders.

### Card Types
| Type | Template | Data Fields |
|------|----------|-------------|
| news | cards/templates/news.html | ID, Title, Description, URL, Category, Summary, Time |
| video | cards/templates/video.html | ID, Title, Thumbnail, Channel, Duration, Views |
| note | cards/templates/note.html | ID, Title, Content, Tags[], Color, Pinned, Time |
| mail | cards/templates/mail.html | ID, From, Subject, Preview, Time, Unread |
| blog | cards/templates/blog.html | ID, Title, Author, Preview, Tags[], Time |
| app | cards/templates/app.html | ID, Name, Summary, Author |
| chat | cards/templates/chat.html | User, Text, Time, IsUser |
| market | cards/templates/market.html | Symbol, Price, Change, Up |

### Server-side Usage (Go)
```go
import "mu/cards"

// Render single card
html := cards.RenderHTML(cards.TypeNews, cards.NewsData{
    Title: "Breaking News",
    Category: "tech",
})

// Render in grid layout
html := cards.Grid(cards.RenderHTML(...))

// Render list
html := cards.ListLayout(cards.RenderHTML(...))
```

### Client-side Usage (JS)
```js
// Fetch template
const resp = await fetch('/card/news');
const template = await resp.text();

// Or render server-side with data
const resp = await fetch('/card/news/render', {
  method: 'POST',
  body: JSON.stringify({title: "...", category: "..."})
});
const html = await resp.text();
```

### Card CSS Classes (mu.css)
```
Base:     .card
Elements: .card-title, .card-desc, .card-meta, .card-time, .card-author
          .card-category, .card-tags, .card-summary, .card-content, .card-preview
Layouts:  .card-grid, .card-list, .card-row
Colors:   .card-yellow, .card-green, .card-blue, .card-pink, .card-purple, .card-gray
States:   .card-unread, .card-user, .card-up, .card-down
```

**IMPORTANT: Use cards for rendering data. Do NOT create new bespoke HTML for data types.**

## CSS Utility Classes (mu.css)
```
Display:    d-none, d-block, d-inline, d-flex, d-inline-block
Flex:       flex-column, items-center, justify-between, gap-1/2/3/4/5
Text:       text-center, text-left, text-right, text-error, text-success, text-muted
Font:       text-xs, text-sm, text-lg, text-xl, font-bold, font-normal, italic
Spacing:    m-0, mt-*, mb-*, ml-*, mr-*, mx-*, my-*, p-*, py-*, px-*
Width:      w-full, max-w-sm/md/lg/xl
Background: bg-light, bg-success-light, bg-warning-light
```

## UI Components (mu.css)

**Use these. Do NOT create page-specific CSS duplicating these patterns.**

### Page Header with Search
```html
<div class="search-bar">
  <input type="text" name="q" placeholder="Search...">
  <a href="/thing/new" class="btn">+ New</a>
</div>
```

### Buttons
- `a.btn` or `button` - primary action (dark background)
- `.btn-secondary` - secondary
- `.btn-danger` - destructive (red)
- `.btn-success` - positive (green)

### Form Inputs
All inputs get consistent height (36px) via mu.css. Don't override.

## Next Up
- Migrate existing pages to use card system
- Remove duplicate per-page CSS
