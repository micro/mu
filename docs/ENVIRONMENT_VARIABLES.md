# Environment Variables

## AI/Chat Configuration

**Priority order:** Anthropic > Fanar > Ollama

### Anthropic Claude (Optional)

```bash
# Anthropic Claude API for chat functionality
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export ANTHROPIC_MODEL="claude-haiku-4-5-20250311"  # Default model
```

### Fanar API (Optional)

```bash
# Fanar API for chat functionality
export FANAR_API_KEY="your-fanar-api-key"
export FANAR_API_URL="https://api.fanar.qa"  # Default: https://api.fanar.qa
```

**Note:** Fanar has a rate limit of 10 requests per minute, enforced automatically.

### Ollama (Fallback)

```bash
# Used if neither Anthropic nor Fanar API key is set
export MODEL_NAME="llama3.2"                    # Default: llama3.2
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

## YouTube Configuration

```bash
# YouTube API key for video functionality
export YOUTUBE_API_KEY="your-youtube-api-key"
```

## Places Configuration

```bash
# Google Places API key for enhanced places search and nearby POI lookup
# Optional: falls back to OpenStreetMap/Overpass when not set
export GOOGLE_API_KEY="your-google-places-api-key"
```

**Notes:**
- When `GOOGLE_API_KEY` is set, Places uses the [Google Places API (New)](https://developers.google.com/maps/documentation/places/web-service/overview) for search and nearby queries
- Without it, Places falls back to free OpenStreetMap/Overpass and Nominatim data
- Enable the **Places API (New)** in Google Cloud Console; the YouTube Data API key is separate

## Vector Search Configuration

```bash
# Ollama endpoint for semantic vector search (embeddings)
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

**Note:** Vector search requires Ollama with `nomic-embed-text` model. If unavailable, falls back to keyword search. See [Vector Search](/docs/vector-search) for installation.

**TODO:** The Ollama endpoint (`http://localhost:11434`) and embedding model (`nomic-embed-text`) are currently hardcoded in `data/data.go`. Consider making these configurable via `MODEL_API_URL` and a new `EMBEDDING_MODEL` environment variable.

## ActivityPub Configuration

```bash
# Domain for ActivityPub federation (user discovery, actor URLs)
export MU_DOMAIN="yourdomain.com"  # Falls back to MAIL_DOMAIN, then "localhost"
```

**Note:** This must match your public domain so remote servers can resolve your users. See [ActivityPub](/docs/activitypub) for details.

## Messaging Configuration

Mu has two messaging systems:
- **Internal messages** - Free, instant delivery between Mu users
- **External email** - SMTP-based, costs credits, for sending to outside email addresses

```bash
# SMTP server port for receiving external email
export MAIL_PORT="2525"              # Default: 2525 (use 25 for production)

# Domain for email addresses
export MAIL_DOMAIN="yourdomain.com"  # Default: localhost

# DKIM signing selector (requires keys in ~/.mu/keys/dkim.key)
export MAIL_SELECTOR="default"       # Default: default
```

**Notes:**
- Internal messaging works without any configuration
- SMTP configuration only needed for external email (sending/receiving outside Mu)
- Mu delivers external messages directly to recipient servers via SMTP (no relay needed)
- DKIM signing enables automatically if keys exist at `~/.mu/keys/dkim.key`
- External email costs credits (SMTP delivery cost)

## Stripe Configuration (Optional)

Enable card payments via Stripe for topping up credits.

```bash
# Stripe keys (optional - for card payments)
export STRIPE_SECRET_KEY="sk_live_..."
export STRIPE_PUBLISHABLE_KEY="pk_live_..."
export STRIPE_WEBHOOK_SECRET="whsec_..."  # For verifying Stripe webhook events
```

**Notes:**
- When empty, card payment option is hidden on the top-up page
- Configure a Stripe webhook pointing to `/wallet/stripe/webhook` to credit users after payment
- Supported events: `checkout.session.completed`

## Payment Configuration (Optional)

Enable donations to support your instance. All variables are optional - leave empty for a free instance.

```bash
# One-time donation URL
export DONATION_URL="https://gocardless.com/your-donation-link"
```

**Notes:**
- When empty, donation features are hidden
- Links appear on `/donate` page

## Crypto Wallet Configuration (Credits/Payments)

Enable payments via crypto deposits. When configured, users get 10 free AI queries per day, then can pay-as-you-go by depositing crypto.

**When wallet is NOT configured:** All quotas are disabled. Users have unlimited free access. This is the default for self-hosted instances.

```bash
# Wallet seed (optional - auto-generated if not set)
# If not provided, a new seed is generated and saved to ~/.mu/keys/wallet.seed
export WALLET_SEED="24 word mnemonic phrase here"

# Base RPC endpoint (optional - uses public endpoint by default)
export BASE_RPC_URL="https://mainnet.base.org"

# Deposit polling interval in seconds (optional - default: 30)
export DEPOSIT_POLL_INTERVAL="30"

# WalletConnect Project ID (optional - for WalletConnect integration)
export WALLETCONNECT_PROJECT_ID="your-project-id"
```

### Quota Configuration

```bash
# Daily free AI queries (default: 10)
export FREE_DAILY_SEARCHES="10"

# Credit costs per operation (default values shown)
export CREDIT_COST_NEWS="1"        # News search (1p)
export CREDIT_COST_VIDEO="2"       # Video search (2p) - YouTube API cost
export CREDIT_COST_VIDEO_WATCH="0" # Video watch (free) - no value added over YouTube
export CREDIT_COST_CHAT="3"        # Chat AI query (3p) - LLM cost
export CREDIT_COST_EMAIL="4"       # External email (4p) - SMTP delivery cost
export CREDIT_COST_PLACES_SEARCH="5"  # Places text search (5p) - Google Places API cost
export CREDIT_COST_PLACES_NEARBY="2"  # Nearby places lookup (2p) - Google Places API cost
```

**Notes:**
- 1 credit = £0.01 (1 penny)
- Admins get unlimited access (no quotas)
- Credits never expire
- Users deposit any ERC-20 token on Base network

### Wallet Seed Location

The wallet seed is stored in `~/.mu/keys/wallet.seed`. If not provided via environment variable, it will be auto-generated on first run.

**IMPORTANT:** Back up this file! It controls all deposit addresses.

## Example Usage

### Development (Local Testing)

```bash
# Messaging server on port 2525
export MAIL_PORT="2525"
export MAIL_DOMAIN="localhost"

./mu --serve --address :8080
```

### Production

```bash
# Messaging server on standard SMTP port 25
export MAIL_PORT="25"

# Your domain and DKIM configuration
export MAIL_DOMAIN="yourdomain.com"
export MAIL_SELECTOR="default"

./mu --serve --address :8080
```

## All Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MU_DOMAIN` | `localhost` | Domain for ActivityPub federation (falls back to `MAIL_DOMAIN`) |
| `MU_USE_SQLITE` | - | Set to `1` to store search index and embeddings in SQLite instead of RAM |
| `ANTHROPIC_API_KEY` | - | Anthropic API key for chat (highest priority) |
| `ANTHROPIC_MODEL` | `claude-haiku-4-5-20250311` | Anthropic model name |
| `FANAR_API_KEY` | - | Fanar API key for chat |
| `FANAR_API_URL` | `https://api.fanar.qa` | Fanar API endpoint |
| `MODEL_NAME` | `llama3.2` | Ollama model name (fallback when no cloud key is set) |
| `MODEL_API_URL` | `http://localhost:11434` | Ollama API endpoint (also used for vector search embeddings) |
| `YOUTUBE_API_KEY` | - | YouTube API key for video functionality |
| `GOOGLE_API_KEY` | - | Google Places API key for enhanced places search |
| `MAIL_PORT` | `2525` | Port for messaging server (SMTP protocol, use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Your domain for message addresses |
| `MAIL_SELECTOR` | `default` | DKIM selector for DNS lookup |
| `DONATION_URL` | - | Payment link for one-time donations (optional) |
| `STRIPE_SECRET_KEY` | - | Stripe secret key for card payments |
| `STRIPE_PUBLISHABLE_KEY` | - | Stripe publishable key for card payments |
| `STRIPE_WEBHOOK_SECRET` | - | Stripe webhook secret for verifying events |
| `WALLET_SEED` | - | BIP39 mnemonic for HD wallet (auto-generated if not set) |
| `BASE_RPC_URL` | `https://mainnet.base.org` | Base network RPC endpoint |
| `DEPOSIT_POLL_INTERVAL` | `30` | Crypto deposit polling interval in seconds |
| `WALLETCONNECT_PROJECT_ID` | - | WalletConnect Project ID (optional) |
| `FREE_DAILY_SEARCHES` | `10` | Daily free AI queries |
| `CREDIT_COST_NEWS` | `1` | Credits per news search |
| `CREDIT_COST_VIDEO` | `2` | Credits per video search |
| `CREDIT_COST_VIDEO_WATCH` | `0` | Credits per video watch (free by default) |
| `CREDIT_COST_CHAT` | `3` | Credits per chat query |
| `CREDIT_COST_EMAIL` | `4` | Credits per external email |
| `CREDIT_COST_PLACES_SEARCH` | `5` | Credits per places text search |
| `CREDIT_COST_PLACES_NEARBY` | `2` | Credits per nearby places lookup |

## .env File (Optional)

Create a `.env` file:

```bash
# AI/Chat (priority: Anthropic > Fanar > Ollama)
# ANTHROPIC_API_KEY=your-anthropic-api-key
FANAR_API_KEY=your-fanar-api-key
FANAR_API_URL=https://api.fanar.qa
MODEL_NAME=llama3.2
MODEL_API_URL=http://localhost:11434

# YouTube
YOUTUBE_API_KEY=your-youtube-api-key

# Places (optional - falls back to OpenStreetMap without this)
# GOOGLE_API_KEY=your-google-places-api-key

# Messaging (uses SMTP protocol)
MAIL_PORT=2525
MAIL_DOMAIN=yourdomain.com
MAIL_SELECTOR=default

# Stripe card payments (optional)
# STRIPE_SECRET_KEY=sk_live_...
# STRIPE_PUBLISHABLE_KEY=pk_live_...
# STRIPE_WEBHOOK_SECRET=whsec_...

# Donations (optional - leave empty for free instance)
DONATION_URL=https://gocardless.com/your-donation-link

# Crypto wallet (optional - for payments)
# If not set, seed is auto-generated in ~/.mu/keys/wallet.seed
# WALLET_SEED=your 24 word mnemonic phrase
```

Load and run:

```bash
export $(cat .env | xargs) && ./mu --serve --address :8080
```

## Systemd Service Example

```ini
[Unit]
Description=Mu Service
After=network.target

[Service]
Type=simple
User=mu
WorkingDirectory=/opt/mu

# AI/Chat
Environment="FANAR_API_KEY=your-fanar-api-key"
Environment="FANAR_API_URL=https://api.fanar.ai"

# YouTube
Environment="YOUTUBE_API_KEY=your-youtube-api-key"

# Messaging
Environment="MAIL_PORT=25"
Environment="MAIL_DOMAIN=yourdomain.com"
Environment="MAIL_SELECTOR=default"

# Donations (optional)
Environment="DONATION_URL=https://gocardless.com/your-donation-link"

# Crypto wallet (optional - auto-generated if not set)
# Environment="WALLET_SEED=your 24 word mnemonic phrase"

ExecStart=/opt/mu/mu --serve --address :8080
Restart=always

[Install]
WantedBy=multi-user.target
```

## Docker Example

```dockerfile
FROM golang:1.24 AS builder
WORKDIR /app
COPY . .
RUN go build -o mu

FROM debian:bookworm-slim
COPY --from=builder /app/mu /usr/local/bin/
EXPOSE 8080 25
CMD ["mu", "--serve", "--address", ":8080"]
```

Run with environment variables:

```bash
docker run -d \
  -p 8080:8080 \
  -p 25:25 \
  -e FANAR_API_KEY=your-fanar-api-key \
  -e YOUTUBE_API_KEY=your-youtube-api-key \
  -e MAIL_PORT=25 \
  -e MAIL_DOMAIN=yourdomain.com \
  -e MAIL_SELECTOR=default \
  -e DONATION_URL=https://gocardless.com/your-donation-link \
  # Wallet seed auto-generated in ~/.mu/keys/wallet.seed if not set
  # -e WALLET_SEED="your 24 word mnemonic" \
  -v ~/.mu:/root/.mu \
  mu:latest
```

## Getting API Keys

### Anthropic API
- Sign up at [Anthropic](https://www.anthropic.com)
- Get your API key from the [Console](https://console.anthropic.com)
- Highest priority AI provider

### Fanar API
- Sign up at [Fanar AI](https://fanar.ai)
- Get your API key from the dashboard
- Used when Anthropic key is not set

### YouTube API
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project
3. Enable YouTube Data API v3
4. Create credentials (API Key)
5. Required for video search/playback

### Google Places API
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create or reuse a project
3. Enable **Places API (New)**
4. Create credentials (API Key)
5. Optional — Places falls back to OpenStreetMap/Overpass without it

## Feature Requirements

| Feature | Required Environment Variables |
|---------|-------------------------------|
| ActivityPub | `MU_DOMAIN` (optional, falls back to `MAIL_DOMAIN`) |
| Chat | `ANTHROPIC_API_KEY`, `FANAR_API_KEY`, or Ollama (`MODEL_NAME`, `MODEL_API_URL`) |
| Vector Search | Ollama with `nomic-embed-text` model (`MODEL_API_URL`) |
| Video | `YOUTUBE_API_KEY` |
| Places | `GOOGLE_API_KEY` (optional, falls back to OpenStreetMap) |
| Messaging | `MAIL_PORT`, `MAIL_DOMAIN` (optional: `MAIL_SELECTOR` for DKIM) |
| Donations | `DONATION_URL` |
| Card Payments | `STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`, `STRIPE_WEBHOOK_SECRET` |
| Crypto Payments | `WALLET_SEED` or auto-generated in `~/.mu/keys/wallet.seed` |

