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

## Recent Session Changes
- Sidebar sorted alphabetically (Home at top, Login/Logout at bottom)
- Notes preserve newlines in card previews (white-space: pre-wrap)
- Chat summaries redesigned: single collapsible "Today's Topics" card, summaries shown directly on topic pages
- Notes auto-save with undo (localStorage stores original, revert on demand)
- Agent added to sidebar
- App generation: stronger prompts against placeholders, mustache template replacement for {{ mu.user.name }} etc.
- GitHub repo moved from asim/mu to micro/mu
- micro.mu landing page updated: "Simple tools for the internet" → links to mu.xyz, GitHub, Go Micro
- App generation now uses Anthropic Claude Haiku for speed (~3-4s vs 9-12s with Fanar)
- Template-based app generation with external /sdk.css and /sdk.js
- Status page shows disk usage, all LLM providers with roles
- Provider selection: ai.Prompt.Provider field to force specific LLM (Anthropic for apps, Fanar for chat)

## Platform Vision

### Agent as Primary Interface
The agent should be the front door to Mu. Users express intent, agent orchestrates:
- "Track my spending" → agent creates expense tracker app
- "Email me weekly summary" → agent wires app + mail + scheduling
- "Show crypto prices" → agent surfaces markets app

Current tools exist (news_search, send_email, app_create). Missing: making agent the primary UI, persistent memory, scheduled tasks.

### App Platform Primitives
Apps need a real platform, not just isolated iframes:

**Current:**
- `mu.db` - persistent storage ✓
- `mu.fetch` - proxied HTTP ✓
- `mu.user` - identity (exists but limited)
- `mu.cache` - local caching ✓

**Needed:**
- `mu.schedule` - run something later (cron-like)
- `mu.notify` - push to user (email, toast)
- `mu.events` - pub/sub between apps

Agent tools are high-level (feature-aware). App primitives are low-level (composable). Clean separation.

### Mu Market (TODO)
With crypto wallet (payments) + app builder (apps), establish a marketplace:

**Concept:**
- Creators build and publish apps
- Apps can be free, paid, or freemium
- Payments via existing wallet/credits system
- Revenue split: creator gets %, Mu takes platform fee

**Features:**
- Browse/search marketplace
- App ratings and reviews
- Install counts, trending
- Creator profiles and earnings
- One-click install to user's app list

**Pricing Models:**
- Free - no cost
- One-time purchase - pay once, own forever
- Usage-based - pay per use (credits)
- Subscription - recurring (future)

**Implementation:**
- `App.Price` field (0 = free, >0 = credits)
- `App.CreatorID` - who built it
- `/market` route - browse/search
- Purchase flow: check balance → deduct credits → grant access
- Creator dashboard: earnings, installs, analytics

This creates an ecosystem: users get utility, creators get paid, Mu takes a cut. Aligns incentives without ads.

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
# LLM Providers
FANAR_API_KEY / FANAR_API_URL    # Default for chat, summaries
ANTHROPIC_API_KEY / ANTHROPIC_MODEL  # Used for app generation (faster)
MODEL_NAME / MODEL_API_URL       # Ollama fallback

# Crypto Wallet (optional)
WALLET_SEED  # BIP39 mnemonic, or auto-generated in ~/.mu/keys/wallet.seed
BASE_RPC_URL  # Default: https://mainnet.base.org
```

## LLM Provider Setup
- **Default order**: Fanar → Anthropic → Ollama (conserves Anthropic credits)
- **App generation**: Forces Anthropic via `ai.ProviderAnthropic` for speed
- **Speed comparison**: Fanar ~40 tok/s, Anthropic Haiku ~100 tok/s
- **Cost**: Anthropic ~$0.001/app, Fanar free
- Status page shows all configured providers and their roles

## Git
- Remote: `git@github.com:micro/mu.git`
- Test credentials: `ssh -p 61194 mu@mu.xyz 'cat ~/.mu/test-creds'`
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
- [x] **blog/** - Has + Write button, feed-style (no search needed)
- [x] **news/** - Uses .search-bar class for consistent styling
- [x] **video/** - Uses .search-bar class for consistent styling
- [x] **mail/** - Uses app.Page() for inbox view with tab navigation
- [x] **chat/** - Interactive chat UI (not suited for app.Page)
- [x] **wallet/** - Dashboard UI (not suited for app.Page)

#### Phase 3: Cleanup
- [x] Move notes editor CSS to mu.css
- [x] Move apps form/card CSS to mu.css
- [x] Move video commonStyles to mu.css
- [ ] Move remaining inline CSS (admin, agent, docs, etc) - 13 left
- [ ] Update claude.md with final patterns

### Current Status
- [x] app/ui.go created with SearchBar, ActionLink, Grid, Title, Page, etc.
- [x] app/router.go created with Route() for content-type dispatch
- [x] notes/ fully migrated to use app.Page()
- [x] apps/ uses app.Page() for list view
- [x] mail/ uses app.Page() with tab navigation
- [x] All search bars use .search-bar class (unified sizing)
- [x] Fixed duplicate title issue in notes edit view
- [ ] Phase 3 cleanup remaining


## Crypto Wallet Plan

### Current State
The wallet is a credits-based system:
- Users have a balance in "credits" (1 credit = 1 penny)
- Top-up via crypto deposits (Base network, any ERC-20)
- Credits spent on Mu services
- HD wallet derives unique deposit address per user

### Why Crypto Instead of Stripe?
- No W8-BEN forms, no KYC overhead on Mu's side
- Decentralized payment rails, not US-centric
- Aligns with Mu philosophy of avoiding gatekeepers
- Users keep control of their funds until they deposit

### Approach: Base Network (Ethereum L2) + Any Token

**Why Base?**
- Ethereum L2 = low fees (~$0.01-0.10 per tx)
- EVM compatible = huge ecosystem
- Any ERC-20 works (ETH, DAI, USDC, whatever)
- No single stablecoin dependency
- Coinbase-backed but decentralized

**Why not Solana?**
- Smaller ecosystem
- Network stability concerns historically

### Architecture

```
┌─────────────────────────────────────────────┐
│           Mu Treasury (HD Wallet)           │
├─────────────────────────────────────────────┤
│ Master seed (in ~/.mu/keys/wallet.seed)     │
│ Derives unique address per user             │
└─────────────────────────────────────────────┘
                      │
         ┌────────────┼────────────┐
         ▼            ▼            ▼
    ┌─────────┐ ┌─────────┐ ┌─────────┐
    │ User 1  │ │ User 2  │ │ User N  │
    │ 0x123...│ │ 0x456...│ │ 0x789...│
    └─────────┘ └─────────┘ └─────────┘
         │            │            │
         └────────────┼────────────┘
                      ▼
              Any ERC-20 deposit
              ETH, DAI, USDC, etc.
```

### User Flow

1. User goes to /wallet, clicks "Add Credits"
2. Shown their unique deposit address: `0xABC123...`
3. Shown: "Send any token on Base network"
4. User sends ETH/DAI/whatever from their wallet
5. Mu detects deposit, checks price, credits account
6. Credits used for Mu services (1 credit = 1p)

### Implementation Phases

#### Phase 1: HD Wallet Setup ✅
- [x] Generate master seed on first run (or use WALLET_SEED env var)
- [x] Store seed in `~/.mu/keys/wallet.seed`
- [x] Derive user addresses using BIP32 (m/44'/60'/0'/0/{userIndex})
- [x] Show deposit address on /wallet page
- [x] Lightweight deps: `go-bip39`, `go-bip32`, stdlib crypto

#### Phase 2: Deposit Detection ✅
- [x] Poll RPCs for incoming transactions (every 30s by default)
- [x] Multi-chain support: Ethereum, Base, Arbitrum, Optimism
- [x] Detect ERC-20 Transfer events to user addresses
- [x] Detect native ETH deposits via balance polling
- [x] Map address → user, credit their account
- [x] Fetch token price (CoinGecko API, 5min cache) to convert to credits
- [x] Mark processed deposits to avoid duplicates
- [x] Chain selector dropdown on deposit page
- [x] QR code with chain ID for mobile wallets

#### Phase 3: Sweep & Consolidation (TODO)
- [ ] Periodically sweep from user addresses to main treasury
- [ ] Reduces number of addresses to monitor
- [ ] Gas paid from treasury ETH balance

#### Phase 4: Withdrawals (TODO)
- [ ] User requests withdrawal to external address
- [ ] Send from treasury (requires gas)
- [ ] Withdrawal fee to cover costs
- [ ] Rate limits, security checks

### Data Model

```go
type CryptoWallet struct {
    UserID       string    `json:"user_id"`
    AddressIndex uint32    `json:"address_index"` // BIP32 derivation index
    Address      string    `json:"address"`       // Derived ETH address
    CreatedAt    time.Time `json:"created_at"`
}

type CryptoDeposit struct {
    ID          string    `json:"id"`
    UserID      string    `json:"user_id"`
    TxHash      string    `json:"tx_hash"`
    Token       string    `json:"token"`       // "ETH" or contract address
    Amount      string    `json:"amount"`      // Raw amount (big int string)
    AmountUSD   float64   `json:"amount_usd"`  // USD value at time of deposit
    Credits     int       `json:"credits"`     // Credits awarded
    BlockNumber uint64    `json:"block_number"`
    CreatedAt   time.Time `json:"created_at"`
}
```

### Key Storage

```
~/.mu/keys/
├── dkim.key          # Existing DKIM key
├── dkim.pub          # Existing DKIM public
└── wallet.seed       # NEW: BIP39 mnemonic (24 words)
```

- If `WALLET_SEED` env var set → use that
- Else if `~/.mu/keys/wallet.seed` exists → load it
- Else → generate new mnemonic, save to file, log warning to backup

### Dependencies (lightweight)

```go
import (
    "github.com/tyler-smith/go-bip39"  // Mnemonic generation
    "github.com/tyler-smith/go-bip32"  // HD key derivation
    "golang.org/x/crypto/sha3"         // Keccak256 for ETH addresses
    "crypto/ecdsa"                     // Stdlib
)
```

NOT using `go-ethereum` - too heavy. Just the minimal crypto libs.

### Config

```bash
# Optional: provide seed via env var (otherwise auto-generated)
WALLET_SEED="word1 word2 word3 ... word24"

# Base RPC endpoint
BASE_RPC_URL=https://mainnet.base.org

# How often to poll for deposits (seconds)
DEPOSIT_POLL_INTERVAL=30
```

### Pricing

Keep existing credit system:
- 1 credit = 1 penny (£0.01)
- Fetch ETH/GBP price at deposit time
- User sends 0.01 ETH (~£25) → gets 2500 credits

### Security

- Seed file permissions: 600 (owner read/write only)
- Never log seed or private keys
- Rate limit deposit checks per user
- Confirm deposits only after N block confirmations

## Tools Registry (January 2025)

### Status
19 tools registered across 9 categories. Agent dynamically discovers and invokes tools.

### Registered Tools
| Category | Tools |
|----------|-------|
| apps | apps.create, apps.list |
| blog | blog.latest |
| mail | mail.inbox, mail.send |
| markets | markets.get_price, markets.list |
| news | news.headlines, news.read, news.search |
| notes | notes.create, notes.get, notes.list, notes.search |
| reminder | reminder.today |
| video | video.latest, video.play, video.search |
| wallet | wallet.balance |

### Architecture
- `tools/tools.go` - Registry with Register, List, Get, Call
- `tools/handler.go` - /tools endpoint (HTML + JSON)
- Each package calls `tools.Register()` in its `Load()` function
- Agent uses `tools.List()` for discovery, `tools.Call()` for invocation
- Agent uses Anthropic provider for speed

### TODO
- `chat.ask` - Chat has complex room/websocket logic, needs refactoring to expose as simple tool
- User-generated app tools via `mu.register()` in SDK
