# Mu Development Checkpoint

## What is Mu?
A personal AI platform - utility tools, not a destination. Like Google Search circa 2000: arrive with intent, get answer, leave.

## Philosophy
- **Utility, not destination** - tools that solve problems and get out of the way
- **Pay for what you use** - no engagement tricks, no unlimited tiers
- **Self-host option** - run your own instance for free forever

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
- **No membership tier** - removed intentionally to avoid engagement incentives

## Recent Changes (This Session)

### Removed Member Tier
- Deleted `Member` field from Account struct
- Removed `/membership` page and route
- Removed Stripe subscription handlers
- Simplified to just Admin/Regular users
- External email now pay-per-use (was member-only)
- Private posts now admin-only (was member-only)
- Moderation now admin-only

### Email System
- External email costs 4 credits (like a stamp)
- Internal messages (user-to-user within Mu) are free
- Added `/admin/email` page with:
  - Pre-computed stats (total, inbound, outbound, internal)
  - Top external domains
  - Recent 50 messages
- Stats computed on startup, updated incrementally per message

### Wallet/Pricing Updates
- Removed `StripeCustomerID` and `StripeSubscriptionID` from Account
- Removed `STRIPE_MEMBERSHIP_PRICE` env var
- Updated `/plans` page - 2 columns (Free + Pay-as-you-go) + self-host card
- Updated `/wallet` page - shows all costs including email
- Fixed account page format string bug

## Key Files

### Pricing
- `wallet/wallet.go` - Credit costs, quota checking
- `wallet/handlers.go` - Wallet UI, Stripe checkout
- `app/app.go` - `/plans` page, account page
- `docs/WALLET_AND_CREDITS.md` - Pricing docs
- `docs/ENVIRONMENT_VARIABLES.md` - Config docs

### Email
- `mail/mail.go` - Message storage, stats, SendMessage()
- `mail/smtp.go` - Inbound SMTP handling
- `mail/client.go` - Outbound SMTP relay
- `admin/email_log.go` - `/admin/email` page

### Auth
- `auth/auth.go` - Account struct (ID, Name, Secret, Created, Admin, Language, Widgets)

## Environment Variables (Relevant)

```bash
# Stripe (for credit top-ups only)
STRIPE_SECRET_KEY
STRIPE_PUBLISHABLE_KEY
STRIPE_WEBHOOK_SECRET

# Quotas
FREE_DAILY_SEARCHES=10
CREDIT_COST_NEWS=1
CREDIT_COST_VIDEO=2
CREDIT_COST_VIDEO_WATCH=0
CREDIT_COST_CHAT=3
CREDIT_COST_EMAIL=4
CREDIT_COST_APP_CREATE=5
CREDIT_COST_APP_MODIFY=3
CREDIT_COST_AGENT=5
```

## Git Remote
- `git@github.com:asim/mu.git`
- Deploy key configured for this VM

## Notes
- Messages stored newest-first (prepended)
- Email stats use separate mutex (`emailStatsMux`) from messages mutex
- YouTube video summarization blocked by YouTube - discussed alternatives (bookmarklet, Whisper, podcasts)
