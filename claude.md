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


## Mobile Home Screen Optimization - In Progress

### Status
Reverted all previous mobile changes. Starting fresh with CSS-only approach.

### Current State
- Commit `86c09c4` - clean state after revert
- Made ONE CSS change in `app/html/mu.css` around line 1673:
  - Added `order: 2` to `.home-left` 
  - Added `order: 1` to `.home-right`
  - Added `.headline .description { display: none; }` in mobile media query
- Server keeps crashing locally (missing Ollama, YouTube API etc) - test on production instead

### Goal
Minimal CSS-only changes in `@media (max-width: 900px)` section:
1. Hide `.headline .description` on mobile
2. Reorder columns: `.home-right` (reminder/markets) before `.home-left` (news)
3. Keep timestamps/source info visible
4. Do NOT touch desktop styles or HTML structure

### Files Changed
- `app/html/mu.css` - mobile media query section around line 1673

### Next Steps
1. Push current change to test on production
2. Verify desktop is unchanged
3. Verify mobile shows reminder/markets before news
4. Verify mobile hides description but keeps info line
