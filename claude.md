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


## Mobile Home Screen Optimization - COMPLETE

### Summary
Minimal CSS-only changes to optimize mobile home screen:
- Column reordering: sidebar (Who's Here, Chat, Reminder, Markets) appears before news
- Headline descriptions hidden on mobile (keeps title, timestamp, source)
- Desktop layout unchanged

### Commit
`9b663db` - WIP: Mobile home - reorder columns, hide description

### Verified
- ✅ Mobile: sidebar first, then news
- ✅ Mobile: descriptions hidden
- ✅ Desktop: two-column layout preserved with descriptions visible
