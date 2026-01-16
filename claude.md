# Mu Development Notes

## Architecture Evolution Plan

### Current State (January 2026)

Single-process Go monolith (~23K lines) running all services:

| Package | Lines | Description |
|---------|-------|-------------|
| apps | 2052 | AI micro app builder |
| news | 2251 | RSS aggregator with AI summary |
| blog | 1693 | Community blog |
| chat | 1500 | AI chat with multiple models |
| video | 1217 | YouTube-style video feed |
| mail | 1208 | Email with SMTP, DKIM, PGP |
| app | 1089 | Core utilities, HTML rendering |
| data | 871 | SQLite + vector search |
| smtp | 851 | Inbound SMTP server |
| wallet | 768 | Credits and payments |
| auth | 582 | Sessions, accounts, presence |
| agent | 470 | AI coding agent |

**Current limitations:**
- Single point of failure
- Memory pressure from all services
- Can't scale individual components
- Deployment requires full restart
- Testing requires full app context

### Phase 1: Internal Modularization (Now)

**Goal:** Clean boundaries without infrastructure changes

1. **Service Interfaces** - Define contracts between packages
   ```go
   // Each service exposes a minimal interface
   type MailService interface {
       Send(msg Message) error
       GetInbox(userID string) []Thread
       GetUnread(userID string) int
   }
   ```

2. **Event Bus** - Decouple cross-service communication
   ```go
   // Instead of direct calls
   events.Publish("user.signup", user)
   events.Publish("mail.received", msg)
   events.Subscribe("user.signup", sendWelcomeEmail)
   ```

3. **Data Isolation** - Each service owns its data
   - `mail.json` → only mail package reads/writes
   - `apps.json` → only apps package reads/writes
   - Shared data through explicit APIs

4. **Configuration** - Centralized config loading
   ```go
   type Config struct {
       Mail   MailConfig
       SMTP   SMTPConfig
       Chat   ChatConfig
       // ...
   }
   ```

### Phase 2: Optional Process Separation

**Goal:** Run heavy services separately when needed

```
┌─────────────────────────────────────────┐
│              mu (main)                  │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐       │
│  │auth │ │blog │ │video│ │home │       │
│  └─────┘ └─────┘ └─────┘ └─────┘       │
└─────────────────────────────────────────┘
         │ gRPC/HTTP │
         ▼           ▼
┌─────────────┐  ┌─────────────┐
│  mu-mail    │  │  mu-chat    │
│  (SMTP+API) │  │  (AI+WS)    │
└─────────────┘  └─────────────┘
```

**Candidates for separation:**
1. **mail** - SMTP server is long-running, independent
2. **chat** - AI calls are expensive, can scale separately
3. **apps** - AI generation is CPU/memory intensive
4. **data** - Vector indexing could be background worker

**Implementation:**
```go
// Build separate binaries
// cmd/mu/main.go        - main web server
// cmd/mu-mail/main.go   - mail service
// cmd/mu-chat/main.go   - chat service

// Services communicate via:
// 1. HTTP API (simple, works now)
// 2. gRPC (efficient, typed)
// 3. Unix socket (same-host, fast)
```

### Phase 3: Horizontal Scaling (Future)

**Only if needed for traffic/load:**

```
                    ┌─────────────┐
                    │   Caddy     │
                    │  (reverse)  │
                    └──────┬──────┘
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
      ┌─────────┐    ┌─────────┐    ┌─────────┐
      │  mu-1   │    │  mu-2   │    │  mu-3   │
      └────┬────┘    └────┬────┘    └────┬────┘
           │              │              │
           └──────────────┼──────────────┘
                          ▼
                   ┌─────────────┐
                   │   SQLite    │
                   │  (Litestream)│
                   └─────────────┘
```

**Requirements:**
- Stateless web handlers (sessions in DB/Redis)
- Shared storage (SQLite with Litestream, or Postgres)
- Load balancer (Caddy already handles this)

### Recommended Immediate Actions

1. **Keep monolith for now** - It works, it's simple
2. **Continue refactoring large files** - mail.go done, next: apps.go, news.go
3. **Add service interfaces** - Prepare for future separation
4. **Extract SMTP** - Most independent component, good first candidate

### File Structure Evolution

```
mu/
├── cmd/
│   ├── mu/main.go           # Main server
│   ├── mu-mail/main.go      # Mail service (optional)
│   └── mu-worker/main.go    # Background jobs (optional)
├── internal/
│   ├── auth/                # Authentication
│   ├── mail/                # Mail logic
│   │   ├── handler.go       # HTTP handlers
│   │   ├── smtp.go          # SMTP server
│   │   ├── service.go       # Business logic
│   │   └── store.go         # Data persistence
│   └── ...
├── pkg/
│   ├── events/              # Event bus
│   └── config/              # Configuration
└── api/
    └── proto/               # gRPC definitions (if needed)
```

---

## Deployment

- Production server: mu.xyz
- SSH: `ssh mu` (user: mu) or `ssh mu-root` (user: root) - both on port 61194
- Service: systemd `mu.service`
- Deploy: Push to main triggers GitHub Action that SSHes as `mu` user, pulls, builds, restarts

## Server Paths (as mu user)

- Source: `/home/mu/src/mu`
- Binary: `/home/mu/go/bin/mu`
- Env vars: `/home/mu/.env`
- Logs: `journalctl -u mu -f`

## SSH Config (on this VM)

```
Host mu          -> user mu (deploy key, limited sudo for systemctl)
Host mu-root     -> user root (full access, use sparingly)
```

## Useful Commands

```bash
# Check status
ssh mu 'sudo systemctl status mu'

# View logs
ssh mu 'journalctl -u mu -f --no-pager -n 50'

# Check memory
ssh mu 'ps aux | grep "[/]home/mu/go/bin/mu"'

# Restart
ssh mu 'sudo systemctl restart mu'
```

## Test Account

Credentials stored in `~/.mu-test/credentials`:
- Username: `shelleytest`
- Password: `shelley123`

## Micro Apps Feature (January 2026)

AI-powered app builder allowing users to create single-page web apps from natural language prompts.

### Routes

- `/apps` - List user's apps and public apps
- `/apps/new` - Create new app (name + prompt → async generation)
- `/apps/{id}` - View app in sandboxed iframe
- `/apps/{id}/develop` - Iterative AI development mode
- `/apps/{id}/preview` - Raw HTML preview for embedding
- `/apps/{id}/status` - JSON status for polling (generating/ready/error)
- `/apps/{id}/delete` - Delete app

### Flow

1. User enters name + prompt at `/apps/new`
2. Click "Create App" → immediately redirects to `/develop`
3. Shows loading spinner, polls `/status` every 2 seconds
4. When ready, page auto-refreshes with preview
5. User iterates: "Add a reset button" → Apply Change → preview updates
6. Changes are auto-saved, history tracked in description field
7. Click "Done" when finished

### Key Files

- `apps/apps.go` - All app logic, handlers, AI generation
- Data stored in `~/.mu/data/apps.json`

### App Model

```go
type App struct {
    ID          string    // Unix nano timestamp
    Name        string
    Description string    // Prompt + change history
    Code        string    // HTML/CSS/JS
    Author      string
    AuthorID    string
    Public      bool
    Status      string    // "generating", "ready", "error"
    Error       string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### AI Prompts

- `generateAppCode()` - Creates new app from scratch
- `modifyAppCode()` - Makes targeted changes to existing code

Both use `chat.AskLLM()` with PriorityHigh.

### Security

Apps run in sandboxed iframe: `sandbox="allow-scripts"`
- JavaScript allowed
- No parent frame access
- No forms, popups, same-origin

### Mu SDK (January 2026)

JavaScript SDK automatically injected into every app:

```javascript
// Database - per-app, per-user storage (100KB quota)
mu.db.get(key)      // async - retrieve value
mu.db.set(key, val) // async - store value
mu.db.delete(key)   // async - delete key
mu.db.list()        // async - list all keys
mu.db.quota()       // async - {used, limit} in bytes

// User context
mu.user.id          // string or null
mu.user.name        // string or null  
mu.user.loggedIn    // boolean

// App context
mu.app.id           // string
mu.app.name         // string
```

Backend: `/apps/api` - RPC endpoint, method routing via `method` field.
Storage: `~/.mu/apps.db` SQLite database.

### Featured Apps (to build)

Three reference apps to validate the platform:

1. **Todo** (`/apps/todo`) - Task management
   - Add/complete/delete tasks
   - Persists via mu.db
   
2. **Timer** (`/apps/timer`) - Pomodoro/focus timer
   - 25min work / 5min break cycles
   - Session tracking
   - Will need: mu.notify() for alerts
   
3. **Expenses** (`/apps/expenses`) - Expense tracking
   - Log expenses with amount/category/date
   - Weekly/monthly summaries
   - More complex data patterns

Reserved single-word URLs redirect to featured/canonical versions.

### Roadmap

**Now:**
- [x] Build three featured apps (Todo, Timer, Expenses) ✓
- [x] Add `/apps/docs` - SDK documentation page ✓
- [x] Update README.md with micro apps section ✓
- [x] Reserve single-word app URLs ✓

**Next:**
- [ ] App forking - copy public apps to customize
- [ ] Public discovery - better browse/search for public apps
- [ ] App templates - pre-built starting points
- [ ] Quota/limits UI - show storage usage per app

**Future SDK extensions:**
- [ ] `mu.ai.complete(prompt)` - LLM access from apps
- [ ] `mu.notify(title, body)` - Push notifications
- [ ] `mu.pay(amount)` - Wallet integration
- [ ] Platform data access (news, markets, etc.)

## Agent Feature (January 2026)

AI agent that can execute tasks across mu services using natural language commands.

### Route
- `/agent` - Agent UI (requires login)
- `/agent/run` - POST endpoint to execute tasks

### Architecture

The agent uses a tool-based architecture with LLM-driven multi-step reasoning:

```
User Request → LLM decides tool → Execute tool → Observe result → Repeat or answer
```

### Available Tools

| Tool | Description |
|------|-------------|
| `video_search` | Search YouTube for videos |
| `video_play` | Play a specific video by ID |
| `news_search` | Search news articles |
| `news_read` | Get full article content |
| `app_create` | Create a new micro app |
| `app_modify` | Modify an existing app |
| `app_list` | List user's or public apps |
| `market_price` | Get current market prices |
| `final_answer` | Provide the final response |

### Example Commands

- "Play bingo songs" → Searches video, returns play link
- "Find news about AI" → Searches news, returns headlines
- "What's the price of Bitcoin?" → Returns BTC price
- "Create an app that tracks my water intake" → Creates and generates app
- "Show my apps" → Lists user's apps

### Key Files

- `agent/agent.go` - Core agent loop and tool definitions
- `agent/tools.go` - Tool implementations
- `agent/handler.go` - HTTP handlers and UI

## Vision / Business Context

Mu is an ethical tech platform for developers' digital daily habits:
- News, video, markets, chat, blog, reminders - without ads/tracking/addiction
- The goal is **10x efficiency**: 10 minutes on Mu replaces 100 minutes across 6 platforms
- Micro apps turn it into a **super app platform** where users can build and share tools
- Agent's job: filter content, generate apps, reduce consumption time - not maximize engagement
- Target audience: developers (2000+ GitHub followers from go-micro)
- Islamic values embedded (reminder system, ethical tech principles)

## Recent Fixes

- Memory leak fix: Added 60s timeout and semaphore (max 5 concurrent) to LLM API calls
- Security: Moved from running as root to dedicated `mu` user with systemd
- SQLite migration: Index and embeddings moved from in-memory JSON to SQLite
  - Enable with `MU_USE_SQLITE=1` environment variable
  - Migration happens automatically on first load
  - Original JSON files preserved as backup
  - Memory reduced from ~750MB to ~40MB
- LLM rate limiting and priority queue:
  - Fanar limit: 10 req/min, we use max 8 with priority tiers
  - Priority High (chat): 8 slots, 15s wait
  - Priority Medium (headlines/rerank): 6 slots, 8s wait  
  - Priority Low (article summaries/tags): 4 slots, 3s wait
  - Topic summaries: every 4 hours (was hourly)
  - Article summaries cached in metadata JSON files
