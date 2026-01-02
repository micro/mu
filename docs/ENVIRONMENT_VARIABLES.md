# Environment Variables

## AI/Chat Configuration

### Fanar API (Primary)

```bash
# Fanar API for chat functionality
export FANAR_API_KEY="your-fanar-api-key"
export FANAR_API_URL="https://api.fanar.ai"  # Default: https://api.fanar.ai
```

### Ollama (Fallback)

```bash
# Used if Fanar API key is not set
export MODEL_NAME="llama3.2"                    # Default: llama3.2
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

## YouTube Configuration

```bash
# YouTube API key for video functionality
export YOUTUBE_API_KEY="your-youtube-api-key"
```

## Vector Search Configuration

```bash
# Ollama endpoint for semantic vector search (embeddings)
export MODEL_API_URL="http://localhost:11434"   # Default: http://localhost:11434
```

**Note:** Vector search requires Ollama with `nomic-embed-text` model. If unavailable, falls back to keyword search. See [Vector Search Setup](VECTOR_SEARCH.md) for installation.

**TODO:** The Ollama endpoint (`http://localhost:11434`) and embedding model (`nomic-embed-text`) are currently hardcoded in `data/data.go`. Consider making these configurable via `MODEL_API_URL` and a new `EMBEDDING_MODEL` environment variable.

## Messaging Configuration

Messaging system uses SMTP protocol for delivery.

```bash
# SMTP server port for receiving messages
export MAIL_PORT="2525"              # Default: 2525 (use 25 for production)

# Domain for message addresses
export MAIL_DOMAIN="yourdomain.com"  # Default: localhost

# DKIM signing selector (requires keys in ~/.mu/keys/dkim.key)
export MAIL_SELECTOR="default"       # Default: default
```

**Notes:**
- SMTP server always runs automatically
- Mu delivers external messages directly to recipient servers via SMTP (no relay needed)
- DKIM signing enables automatically if keys exist at `~/.mu/keys/dkim.key`
- Messaging access is restricted to admins and members only

## Payment Configuration (Optional)

Enable paid memberships and donations to support your instance. All variables are optional - leave empty for a free instance.

```bash
# Membership payment URL (e.g., subscription/recurring payment link)
export MEMBERSHIP_URL="https://gocardless.com/your-membership-link"

# One-time donation URL
export DONATION_URL="https://gocardless.com/your-donation-link"

# Community/support URL (e.g., Discord, forum)
export SUPPORT_URL="https://discord.gg/your-invite"
```

**Notes:**
- Use any payment provider (GoCardless, Stripe, PayPal, etc.)
- Payment callbacks are verified by extracting the domain from your URLs
- When empty, payment/donation features are hidden
- Links appear on `/membership` and `/donate` pages
- For automated membership via Stripe, see Stripe Configuration below

## Stripe Configuration (Credits/Wallet/Membership)

Enable credit top-ups and automated memberships via Stripe. Users get 10 free searches per day, then need credits or a membership.

```bash
# Stripe API keys (from Stripe Dashboard)
export STRIPE_SECRET_KEY="sk_live_xxx"
export STRIPE_PUBLISHABLE_KEY="pk_live_xxx"
export STRIPE_WEBHOOK_SECRET="whsec_xxx"

# Membership subscription (recurring monthly)
export STRIPE_MEMBERSHIP_PRICE="price_xxx"  # Monthly subscription price ID

# Optional: Pre-configured Stripe Price IDs for credit top-ups
# If not set, dynamic pricing is used
export STRIPE_PRICE_500="price_xxx"   # £5 → 500 credits
export STRIPE_PRICE_1000="price_xxx"  # £10 → 1,050 credits
export STRIPE_PRICE_2500="price_xxx"  # £25 → 2,750 credits
export STRIPE_PRICE_5000="price_xxx"  # £50 → 5,750 credits
```

### Quota Configuration

```bash
# Daily free searches for non-members (default: 10)
export FREE_DAILY_SEARCHES="10"

# Credit costs per operation (default: 1, 2, 3)
export CREDIT_COST_NEWS="1"    # News search
export CREDIT_COST_VIDEO="2"   # Video search (YouTube API cost)
export CREDIT_COST_CHAT="3"    # Chat AI query (LLM cost)
```

**Notes:**
- 1 credit = £0.01 (1 penny)
- Members and admins get unlimited access (no quotas)
- Credits never expire
- Top-up tiers: £5 (500), £10 (1,050 +5%), £25 (2,750 +10%), £50 (5,750 +15%)
- Memberships are managed automatically via Stripe webhooks

### Stripe Webhook Setup

1. In Stripe Dashboard, go to Developers → Webhooks
2. Add endpoint: `https://yourdomain.com/wallet/webhook`
3. Select events:
   - `checkout.session.completed` (for credits and new subscriptions)
   - `customer.subscription.created` (membership activated)
   - `customer.subscription.updated` (membership changes)
   - `customer.subscription.deleted` (membership cancelled)
4. Copy the signing secret to `STRIPE_WEBHOOK_SECRET`

### Creating a Membership Product

1. In Stripe Dashboard, go to Products → Add Product
2. Create a recurring subscription product (e.g., "Mu Membership")
3. Set pricing (e.g., £5/month)
4. Copy the Price ID (starts with `price_`) to `STRIPE_MEMBERSHIP_PRICE`

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
| `FANAR_API_KEY` | - | Fanar API key for chat (required for chat) |
| `FANAR_API_URL` | `https://api.fanar.ai` | Fanar API endpoint |
| `MODEL_NAME` | `llama3.2` | Ollama model name (if Fanar not configured) |
| `MODEL_API_URL` | `http://localhost:11434` | Ollama API endpoint (also used for vector search embeddings) |
| `YOUTUBE_API_KEY` | - | YouTube API key for video functionality |
| `MAIL_PORT` | `2525` | Port for messaging server (SMTP protocol, use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Your domain for message addresses |
| `MAIL_SELECTOR` | `default` | DKIM selector for DNS lookup |
| `MEMBERSHIP_URL` | - | External payment link for membership (alternative to Stripe) |
| `DONATION_URL` | - | Payment link for one-time donations (optional) |
| `SUPPORT_URL` | - | Community/support link like Discord (optional) |
| `STRIPE_SECRET_KEY` | - | Stripe secret key for payments |
| `STRIPE_PUBLISHABLE_KEY` | - | Stripe publishable key |
| `STRIPE_WEBHOOK_SECRET` | - | Stripe webhook signing secret |
| `STRIPE_MEMBERSHIP_PRICE` | - | Stripe price ID for monthly membership subscription |
| `FREE_DAILY_SEARCHES` | `10` | Daily free searches for non-members |
| `CREDIT_COST_NEWS` | `1` | Credits per news search |
| `CREDIT_COST_VIDEO` | `2` | Credits per video search |
| `CREDIT_COST_CHAT` | `3` | Credits per chat query |

## .env File (Optional)

Create a `.env` file:

```bash
# AI/Chat
FANAR_API_KEY=your-fanar-api-key
FANAR_API_URL=https://api.fanar.ai
MODEL_NAME=llama3.2
MODEL_API_URL=http://localhost:11434

# YouTube
YOUTUBE_API_KEY=your-youtube-api-key

# Messaging (uses SMTP protocol)
MAIL_PORT=2525
MAIL_DOMAIN=yourdomain.com
MAIL_SELECTOR=default

# Payment (optional - leave empty for free instance)
MEMBERSHIP_URL=https://gocardless.com/your-membership-link
DONATION_URL=https://gocardless.com/your-donation-link
SUPPORT_URL=https://discord.gg/your-invite

# Stripe (optional - for credit top-ups)
STRIPE_SECRET_KEY=sk_live_xxx
STRIPE_PUBLISHABLE_KEY=pk_live_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
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

# Payment (optional)
Environment="MEMBERSHIP_URL=https://gocardless.com/your-membership-link"
Environment="DONATION_URL=https://gocardless.com/your-donation-link"
Environment="SUPPORT_URL=https://discord.gg/your-invite"

# Stripe (optional)
Environment="STRIPE_SECRET_KEY=sk_live_xxx"
Environment="STRIPE_PUBLISHABLE_KEY=pk_live_xxx"
Environment="STRIPE_WEBHOOK_SECRET=whsec_xxx"

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
  -e MEMBERSHIP_URL=https://gocardless.com/your-membership-link \
  -e DONATION_URL=https://gocardless.com/your-donation-link \
  -e SUPPORT_URL=https://discord.gg/your-invite \
  -e STRIPE_SECRET_KEY=sk_live_xxx \
  -e STRIPE_PUBLISHABLE_KEY=pk_live_xxx \
  -e STRIPE_WEBHOOK_SECRET=whsec_xxx \
  -v ~/.mu:/root/.mu \
  mu:latest
```

## Getting API Keys

### Fanar API
- Sign up at [Fanar AI](https://fanar.ai)
- Get your API key from the dashboard
- Required for chat functionality

### YouTube API
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project
3. Enable YouTube Data API v3
4. Create credentials (API Key)
5. Required for video search/playback

### Stripe
1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Get API keys from Developers → API Keys
3. Set up webhook endpoint at Developers → Webhooks
4. Create a subscription product for memberships
5. Required for credit top-ups and automated memberships

## Feature Requirements

| Feature | Required Environment Variables |
|---------|-------------------------------|
| Chat | `FANAR_API_KEY` or Ollama (`MODEL_NAME`, `MODEL_API_URL`) |
| Vector Search | Ollama with `nomic-embed-text` model (`MODEL_API_URL`) |
| Video | `YOUTUBE_API_KEY` |
| Messaging | `MAIL_PORT`, `MAIL_DOMAIN` (optional: `MAIL_SELECTOR` for DKIM) |
| External Payments | `MEMBERSHIP_URL`, `DONATION_URL` (optional: `SUPPORT_URL`) |
| Credit Top-ups | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` |
| Stripe Membership | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_MEMBERSHIP_PRICE` |
| Access Control | User must be admin or member |

