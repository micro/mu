# Mu Development Notes

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

### Next Steps (not yet implemented)

- Data storage API for apps (let apps persist user data)
- Access to platform data (news, markets, etc.)
- Wallet integration (charge for apps)
- App versioning

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
