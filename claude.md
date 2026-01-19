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

## UI Helpers (app/ui.go)

Layout helpers and element builders for consistent UI. Fast string builders.

### Layout Helpers
```go
// Search bar with search button
app.SearchBar("/notes", "Search notes...", query)

// Primary action link
app.ActionLink("/notes/new", "+ New Note")

// Container layouts
app.Grid(content)   // .card-grid - responsive grid
app.List(content)   // .card-list - vertical stack  
app.Row(content)    // .card-row - horizontal flex

// Card wrapper
app.CardDiv(content)              // <div class="card">...</div>
app.CardDivClass("card-note", content)

// Empty state
app.Empty("No items yet")
```

### Element Builders
```go
app.Title("My Note", "/notes/123")  // .card-title link
app.Desc("Description text")        // .card-desc paragraph
app.Meta("by author · 2h ago")      // .card-meta
app.Tags([]string{"tag1", "tag2"}, "/notes?tag=")  // .card-tags
```

### Card CSS Classes (mu.css)
```
Base:     .card (padding, border, hover shadow)
Elements: .card-title, .card-desc, .card-meta, .card-content, .card-preview
          .card-tags .tag, .card-pin, .card-category
Layouts:  .card-grid, .card-list, .card-row
Colors:   .card-yellow, .card-green, .card-blue, .card-pink, .card-purple, .card-gray
```

### Usage Pattern
Keep render logic in each package, use app helpers for common elements:
```go
func renderNoteCard(n *Note) string {
    var b strings.Builder
    b.WriteString(`<div class="card card-note">`)
    b.WriteString(app.Title(n.Title, "/notes/"+n.ID))
    b.WriteString(app.Desc(n.Content))
    b.WriteString(app.Tags(n.Tags, ""))
    b.WriteString(app.Meta(app.TimeAgo(n.UpdatedAt)))
    b.WriteString(`</div>`)
    return b.String()
}
```

**IMPORTANT: Use .card classes and app helpers. Do NOT create new page-specific CSS for cards.**

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

## Architecture Refactor Plan

### Goal
Consolidate routing, content negotiation, and rendering into app/api packages. Each feature package focuses on core logic only.

### Package Structure (Target)

| Package | Files | Purpose |
|---------|-------|---------|
| **app/** | app.go, ui.go, router.go | Base template, UI helpers, content-type routing |
| **api/** | api.go, handlers.go | API registration, JSON endpoints, documentation |
| **notes/** | notes.go | Note CRUD, search, tagging logic |
| **apps/** | apps.go, db.go, generate.go | App CRUD, code generation |
| **blog/** | blog.go | Post CRUD, comments |
| **news/** | news.go | Feed fetching, caching, search |
| **mail/** | mail.go, smtp.go, client.go | Mail logic, SMTP server |
| **video/** | video.go | YouTube search, playback |
| **chat/** | chat.go | Chat logic, RAG |
| **home/** | home.go | Dashboard card config |
| **agent/** | agent.go, tools.go | Agent execution |
| **auth/** | auth.go | Session, tokens |
| **data/** | data.go, sqlite.go | Storage, indexing |
| **wallet/** | wallet.go | Credits, quotas |
| **admin/** | admin.go | Admin handlers |

### File Naming Convention
- `{package}.go` - Core logic (types, CRUD, business logic)
- `handlers.go` - HTTP handlers (only if complex, otherwise in main file)
- `db.go` - Database/persistence (if separate from logic)

### Content-Type Router (app/router.go) - NEW
```go
// Route registers a handler that responds based on Accept header
func Route(path string, opts RouteOpts)

type RouteOpts struct {
    // JSON handler - called when Accept: application/json
    JSON func(w http.ResponseWriter, r *http.Request)
    // HTML handler - called for browser requests
    HTML func(w http.ResponseWriter, r *http.Request)
    // Auth requirement
    Auth bool
}
```

### API Registration (api/api.go) - ENHANCE
```go
// Register adds endpoint to docs and optionally handles routing
func Register(ep Endpoint)

type Endpoint struct {
    Path        string
    Method      string
    Handler     http.HandlerFunc  // NEW: actual handler
    Auth        bool              // NEW: requires auth
    // ... existing doc fields
}
```

### Standard Page Layout (app/ui.go) - ENHANCE
```go
// Page renders standard page structure
func Page(opts PageOpts) string

type PageOpts struct {
    Action  string // Primary action URL (shows button if set)
    Label   string // Action button label
    Search  string // Search endpoint (shows search bar if set)
    Query   string // Current search query
    Filters string // Filter HTML (tags, toggles)
    Content string // Main content
    Empty   string // Empty state message
}
```

### Refactor Checklist

#### Phase 1: Core Infrastructure
- [x] Create app/router.go with Route() function
- [ ] Update api/api.go with handler registration
- [x] Add app.Page() for standard page layout
- [x] Update app.SearchBar() to match video/news style (has Search button)
- [ ] Consolidate CSS - remove all inline <style> blocks

#### Phase 2: Package Migration (one at a time)
- [x] **notes/** - Uses app.Page(), app.Grid(), app.SearchBar(), app.ActionLink()
- [x] **apps/** - Uses app.Page() for list view
- [ ] **blog/** - Extract rendering, use app.Route(), app.Page()
- [ ] **news/** - Extract rendering, use app.Route(), app.Page()
- [ ] **video/** - Extract rendering, use app.Route(), app.Page()
- [x] **mail/** - Uses app.Page() for inbox view
- [ ] **chat/** - Extract rendering, use app.Route(), app.Page()
- [ ] **wallet/** - Extract rendering, use app.Route(), app.Page()

#### Phase 3: Cleanup
- [ ] Remove duplicate CSS from all packages
- [ ] Consolidate card styles in mu.css
- [ ] Update claude.md with final patterns
- [ ] Audit mu.css - target <500 lines
- [ ] Audit mu.js - target <200 lines

### Current Status
- [x] app/ui.go created with SearchBar, ActionLink, Grid, Title, Page, etc.
- [x] app/router.go created with Route() for content-type dispatch
- [x] notes/ fully migrated to use app.Page()
- [x] apps/ uses app.Page() for list view
- [ ] Other packages still use manual WantsJSON checks and custom layouts
