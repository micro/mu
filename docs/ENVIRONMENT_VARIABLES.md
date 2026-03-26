# Environment Variables

## AI/Chat Configuration

Mu uses Anthropic Claude for all AI features.

```bash
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export ANTHROPIC_MODEL="claude-sonnet-4-20250514"  # Default model
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

## ActivityPub Configuration

```bash
# Domain for ActivityPub federation (user discovery, actor URLs)
export MU_DOMAIN="yourdomain.com"  # Falls back to MAIL_DOMAIN, then "localhost"
```

**Note:** This must match your public domain so remote servers can resolve your users. See [ActivityPub](/docs/activitypub) for details.

## Messaging Configuration

Mu supports sending and receiving external email via SMTP.

```bash
# SMTP server port for receiving external email
export MAIL_PORT="2525"              # Default: 2525 (use 25 for production)

# Domain for email addresses
export MAIL_DOMAIN="yourdomain.com"  # Default: localhost

# DKIM signing selector
export MAIL_SELECTOR="default"       # Default: default

# DKIM private key (PEM format). Takes precedence over ~/.mu/keys/dkim.key
export DKIM_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n..."
```

**Notes:**
- SMTP configuration only needed for external email (sending/receiving outside Mu)
- Mu delivers external messages directly to recipient servers via SMTP (no relay needed)
- DKIM signing enables automatically when `DKIM_PRIVATE_KEY` is set, or if a key file exists at `~/.mu/keys/dkim.key`
- `DKIM_PRIVATE_KEY` takes precedence over the key file (useful in Docker/cloud deployments)
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

## x402 Payments (Optional)

Enable cryptocurrency payments via the [x402 protocol](https://x402.org). External clients (AI agents, apps) can pay per-request with stablecoins instead of needing a Mu account.

```bash
# Wallet address to receive payments (required to enable x402)
export X402_PAY_TO="0xYourWalletAddress"

# Accepted tokens (comma-separated symbols). Default: USDC,EURC
export X402_ASSETS="USDC,EURC"

# Facilitator URL for payment verification and settlement
export X402_FACILITATOR_URL="https://x402.org/facilitator"  # Default

# Blockchain network identifier
export X402_NETWORK="eip155:8453"  # Default: Base mainnet
```

**Notes:**
- Only `X402_PAY_TO` is required — everything else has sensible defaults
- **USDC and EURC on Base** are accepted by default
- `X402_ASSETS` overrides the default tokens (e.g. `"USDC"` for USDC only)
- `X402_ASSET` (single contract address) still works for backwards compatibility
- When enabled, API clients can send `X-PAYMENT` header instead of authenticating
- Payments settle on-chain via the facilitator — no Stripe needed
- Credit costs are converted to USD at 1 credit = $0.01

## Payment Configuration (Optional)

Enable donations to support your instance. All variables are optional - leave empty for a free instance.

```bash
# One-time donation URL
export DONATION_URL="https://gocardless.com/your-donation-link"
```

**Notes:**
- When empty, donation features are hidden
- Links appear on `/donate` page

## Quota Configuration

```bash
# Daily free AI queries (default: 10)
export FREE_DAILY_QUOTA="10"

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
| `MU_USE_SQLITE` | - | Set to `1` to store search index in SQLite with FTS5 |
| `ANTHROPIC_API_KEY` | - | Anthropic API key for AI features (required) |
| `ANTHROPIC_MODEL` | `claude-sonnet-4-20250514` | Anthropic model name |
| `YOUTUBE_API_KEY` | - | YouTube API key for video functionality |
| `GOOGLE_API_KEY` | - | Google Places API key for enhanced places search |
| `MAIL_PORT` | `2525` | Port for messaging server (SMTP protocol, use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Your domain for message addresses |
| `MAIL_SELECTOR` | `default` | DKIM selector for DNS lookup |
| `DKIM_PRIVATE_KEY` | - | DKIM private key in PEM format (takes precedence over `~/.mu/keys/dkim.key`) |
| `PASSKEY_ORIGIN` | `http://localhost:8080` | Primary origin for WebAuthn passkeys |
| `PASSKEY_RP_ID` | `localhost` | Relying Party ID for WebAuthn passkeys |
| `PASSKEY_EXTRA_ORIGINS` | - | Additional WebAuthn origins, comma-separated (e.g., for Tor .onion access) |
| `DONATION_URL` | - | Payment link for one-time donations (optional) |
| `STRIPE_SECRET_KEY` | - | Stripe secret key for card payments |
| `STRIPE_PUBLISHABLE_KEY` | - | Stripe publishable key for card payments |
| `STRIPE_WEBHOOK_SECRET` | - | Stripe webhook secret for verifying events |
| `X402_PAY_TO` | - | Wallet address for x402 crypto payments |
| `X402_ASSETS` | `USDC,EURC` | Accepted tokens (comma-separated symbols) |
| `X402_FACILITATOR_URL` | `https://x402.org/facilitator` | x402 facilitator endpoint |
| `X402_NETWORK` | `eip155:8453` | Blockchain network for x402 payments |
| `FREE_DAILY_QUOTA` | `10` | Daily free AI queries |
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
# AI/Chat
ANTHROPIC_API_KEY=your-anthropic-api-key

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
Environment="ANTHROPIC_API_KEY=your-anthropic-api-key"

# YouTube
Environment="YOUTUBE_API_KEY=your-youtube-api-key"

# Messaging
Environment="MAIL_PORT=25"
Environment="MAIL_DOMAIN=yourdomain.com"
Environment="MAIL_SELECTOR=default"
Environment="DKIM_PRIVATE_KEY=-----BEGIN RSA PRIVATE KEY-----\n..."

# Donations (optional)
Environment="DONATION_URL=https://gocardless.com/your-donation-link"

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
  -e ANTHROPIC_API_KEY=your-anthropic-api-key \
  -e YOUTUBE_API_KEY=your-youtube-api-key \
  -e MAIL_PORT=25 \
  -e MAIL_DOMAIN=yourdomain.com \
  -e MAIL_SELECTOR=default \
  -e DONATION_URL=https://gocardless.com/your-donation-link \
  -v ~/.mu:/root/.mu \
  mu:latest
```

## Getting API Keys

### Anthropic API
- Sign up at [Anthropic](https://www.anthropic.com)
- Get your API key from the [Console](https://console.anthropic.com)
- Required for all AI features (chat, digest, summaries)

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
| Chat | `ANTHROPIC_API_KEY` |
| Video | `YOUTUBE_API_KEY` |
| Places | `GOOGLE_API_KEY` (optional, falls back to OpenStreetMap) |
| Messaging | `MAIL_PORT`, `MAIL_DOMAIN` (optional: `MAIL_SELECTOR` for DKIM) |
| Donations | `DONATION_URL` |
| Card Payments | `STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`, `STRIPE_WEBHOOK_SECRET` |
| Crypto Payments | `X402_PAY_TO` (optional: `X402_FACILITATOR_URL`, `X402_NETWORK`, `X402_ASSET`) |

