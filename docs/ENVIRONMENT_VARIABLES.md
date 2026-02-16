# Environment Variables

## AI/Chat Configuration

### Fanar API (Primary)

```bash
# Fanar API for chat functionality
export FANAR_API_KEY="your-fanar-api-key"
export FANAR_API_URL="https://api.fanar.qa"  # Default: https://api.fanar.qa
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

**Note:** Vector search requires Ollama with `nomic-embed-text` model. If unavailable, falls back to keyword search. See [Vector Search](/docs/vector-search) for installation.

**TODO:** The Ollama endpoint (`http://localhost:11434`) and embedding model (`nomic-embed-text`) are currently hardcoded in `data/data.go`. Consider making these configurable via `MODEL_API_URL` and a new `EMBEDDING_MODEL` environment variable.

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

## XMPP Chat Configuration

Mu includes an XMPP server for federated chat, similar to how SMTP enables federated email.

```bash
# Enable XMPP server (disabled by default)
export XMPP_ENABLED="true"            # Default: false

# Domain for XMPP addresses (JIDs)
export XMPP_DOMAIN="chat.yourdomain.com"  # Default: localhost

# XMPP client-to-server port
export XMPP_PORT="5222"               # Default: 5222 (standard XMPP port)
```

**Notes:**
- XMPP is disabled by default - set `XMPP_ENABLED=true` to enable
- Users can connect with any XMPP client (Conversations, Gajim, etc.)
- Provides federated chat like email federation via SMTP
- See [XMPP Chat documentation](XMPP_CHAT.md) for setup guide
- Requires DNS SRV records for federation

## Payment Configuration (Optional)

Enable donations to support your instance. All variables are optional - leave empty for a free instance.

```bash
# One-time donation URL
export DONATION_URL="https://gocardless.com/your-donation-link"

# Community/support URL (e.g., Discord, forum)
export SUPPORT_URL="https://discord.gg/your-invite"
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
| `FANAR_API_KEY` | - | Fanar API key for chat (required for chat) |
| `FANAR_API_URL` | `https://api.fanar.ai` | Fanar API endpoint |
| `MODEL_NAME` | `llama3.2` | Ollama model name (if Fanar not configured) |
| `MODEL_API_URL` | `http://localhost:11434` | Ollama API endpoint (also used for vector search embeddings) |
| `YOUTUBE_API_KEY` | - | YouTube API key for video functionality |
| `MAIL_PORT` | `2525` | Port for messaging server (SMTP protocol, use 25 for production) |
| `MAIL_DOMAIN` | `localhost` | Your domain for message addresses |
| `MAIL_SELECTOR` | `default` | DKIM selector for DNS lookup |
| `XMPP_ENABLED` | `false` | Enable XMPP chat server |
| `XMPP_DOMAIN` | `localhost` | Domain for XMPP chat addresses (JIDs) |
| `XMPP_PORT` | `5222` | Port for XMPP client-to-server connections |
| `DONATION_URL` | - | Payment link for one-time donations (optional) |
| `SUPPORT_URL` | - | Community/support link like Discord (optional) |
| `WALLET_SEED` | - | BIP39 mnemonic for HD wallet (auto-generated if not set) |
| `BASE_RPC_URL` | `https://mainnet.base.org` | Base network RPC endpoint |
| `FREE_DAILY_SEARCHES` | `10` | Daily free AI queries |
| `CREDIT_COST_NEWS` | `1` | Credits per news search |
| `CREDIT_COST_VIDEO` | `2` | Credits per video search |
| `CREDIT_COST_VIDEO_WATCH` | `0` | Credits per video watch (free by default) |
| `CREDIT_COST_CHAT` | `3` | Credits per chat query |
| `CREDIT_COST_EMAIL` | `4` | Credits per external email |

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

# Donations (optional - leave empty for free instance)
DONATION_URL=https://gocardless.com/your-donation-link
SUPPORT_URL=https://discord.gg/your-invite

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
Environment="SUPPORT_URL=https://discord.gg/your-invite"

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
  -e SUPPORT_URL=https://discord.gg/your-invite \
  # Wallet seed auto-generated in ~/.mu/keys/wallet.seed if not set
  # -e WALLET_SEED="your 24 word mnemonic" \
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

## Feature Requirements

| Feature | Required Environment Variables |
|---------|-------------------------------|
| Chat | `FANAR_API_KEY` or Ollama (`MODEL_NAME`, `MODEL_API_URL`) |
| Vector Search | Ollama with `nomic-embed-text` model (`MODEL_API_URL`) |
| Video | `YOUTUBE_API_KEY` |
| Messaging | `MAIL_PORT`, `MAIL_DOMAIN` (optional: `MAIL_SELECTOR` for DKIM) |
| XMPP Chat | `XMPP_ENABLED=true`, `XMPP_DOMAIN` (optional: `XMPP_PORT`) |
| Donations | `DONATION_URL` (optional: `SUPPORT_URL`) |
| Payments | `WALLET_SEED` or auto-generated in `~/.mu/keys/wallet.seed` |

