# Wallet & Credits

## Philosophy

Mu is a tool, not a destination. Like Google Search in 2000 — you arrive with intent, get what you need, and leave.

Credits are a straightforward way to pay for what you use. No dark patterns, no pressure to upgrade, no "unlimited" tiers that incentivize us to maximize your engagement.

- **Free tier**: 10 AI queries/day - enough for casual utility use
- **Pay-as-you-go**: Deposit crypto, use credits when you need more
- **Self-host**: Run your own instance for free, forever

We charge because LLMs and APIs cost money. Here's our actual cost breakdown — we're not extracting margin, just covering infrastructure.

## How It Works

### Credits

- **1 credit = £0.01 GBP** (1 penny)
- Credits stored as integers to avoid floating-point issues
- Top up via crypto deposit (Ethereum, Base, Arbitrum, Optimism)
- Credits never expire

### Daily Free Quota

Every registered user gets **10 free AI queries per day**:
- Resets at midnight UTC
- Covers news search, video search, and chat AI queries
- No payment required

This should be enough if you're using Mu as a utility. If you need more, pay-as-you-go.

### Credit Costs

| Feature | Cost | Why |
|---------|------|-----|
| News Search | 1 credit (1p) | Indexed search |
| News Summary | 1 credit (1p) | AI-generated summary |
| Video Search | 2 credits (2p) | YouTube API cost |
| Video Watch | Free | No value added over YouTube |
| Chat AI Query | 3 credits (3p) | LLM inference cost |
| Chat Room | 1 credit (1p) | Room creation |
| Places Search | 5 credits (5p) | Google Places API cost |
| Places Nearby | 2 credits (2p) | Google Places API cost |
| External Email | 4 credits (4p) | SMTP delivery cost |
| Weather Forecast | 1 credit (1p) | Weather API cost |
| Weather Pollen | 1 credit (1p) | Pollen data add-on |

**Note:** Internal messages (user-to-user within Mu) are free. Only external email (to addresses outside Mu) costs credits.

### Who Pays What

| User Type | Daily Free | Credits | Notes |
|-----------|------------|---------|-------|
| Guest | 0 | N/A | Must register |
| Registered | 10 queries | Pay-as-you-go | When free quota exceeded |
| Admin | Unlimited | Not needed | Site administrators |

## Why No "Unlimited" Tier?

Unlimited tiers create misaligned incentives. If you pay a flat fee for unlimited usage, we're incentivized to make you use Mu *more* to feel you're getting value. That's the seed of engagement optimization.

Pay-as-you-go keeps us honest: we want to build efficient tools that solve your problem quickly, not sticky products that maximize your screen time.

If you want truly unlimited and free — self-host. The code is open source.

---

## Card Deposits (Stripe)

When Stripe is configured (`STRIPE_SECRET_KEY`, `STRIPE_PUBLISHABLE_KEY`), users can also top up with a credit or debit card:

1. Go to `/wallet/topup` and select **Card**
2. Choose a topup tier (£5, £10, £25, or £50)
3. Complete payment via the Stripe Checkout page
4. Credits are added automatically via the Stripe webhook (`/wallet/stripe/webhook`)

Configure a webhook in the Stripe Dashboard pointing to `https://your-domain.com/wallet/stripe/webhook` and set `STRIPE_WEBHOOK_SECRET` to the signing secret. The webhook listens for `checkout.session.completed` events.

---

## Crypto Deposits

### How It Works

1. Go to `/wallet/topup`
2. Select your network (Ethereum, Base, Arbitrum, or Optimism)
3. You'll see your unique deposit address and QR code
4. Send ETH, USDC, or any ERC-20 token
5. Deposits are detected automatically (~30 seconds)
6. Credits added based on current exchange rate

### Supported Networks

- **Ethereum** - Mainnet
- **Base** - Coinbase L2
- **Arbitrum** - Arbitrum One
- **Optimism** - OP Mainnet

Same address works on all networks.

### Supported Tokens

- **ETH** - Native token
- **USDC** - USD Coin
- **ERC-20** - Any token on supported networks

### Important

- Select the correct network before sending
- Your address is the same on all networks
- Minimum deposit: ~$1 equivalent
- Deposits typically confirm within 1-2 minutes

### Why Crypto?

- No credit cards, no KYC forms, no payment processor gatekeeping
- Works globally without bank restrictions
- You control your funds until you deposit
- Aligns with Mu philosophy: decentralized, no middlemen

---

## Data Model

### Wallet

```go
type Wallet struct {
    UserID    string    `json:"user_id"`
    Balance   int       `json:"balance"`    // Credits (pennies)
    Currency  string    `json:"currency"`   // "GBP"
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Transaction

```go
type Transaction struct {
    ID        string                 `json:"id"`
    UserID    string                 `json:"user_id"`
    Type      string                 `json:"type"`      // "topup", "spend", "refund"
    Amount    int                    `json:"amount"`    // Positive for topup, negative for spend
    Balance   int                    `json:"balance"`   // Balance after transaction
    Operation string                 `json:"operation"` // e.g., "news_search", "topup"
    Metadata  map[string]interface{} `json:"metadata"`
    CreatedAt time.Time              `json:"created_at"`
}
```

### Daily Usage

```go
type DailyUsage struct {
    UserID   string `json:"user_id"`
    Date     string `json:"date"`     // "2006-01-02"
    Searches int    `json:"searches"` // Free queries used today
}
```

### Crypto Wallet

```go
type CryptoWallet struct {
    UserID       string    `json:"user_id"`
    AddressIndex uint32    `json:"address_index"` // BIP32 derivation index
    Address      string    `json:"address"`       // Derived ETH address
    CreatedAt    time.Time `json:"created_at"`
}
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/wallet` | View balance and transaction history |
| GET | `/wallet/topup` | Show deposit address and instructions |
| POST | `/wallet/stripe/checkout` | Create a Stripe checkout session |
| GET | `/wallet/stripe/success` | Success page after Stripe payment |
| POST | `/wallet/stripe/webhook` | Stripe webhook for payment confirmation |

---

## Environment Variables

```bash
# Crypto Wallet (optional)
# If not set, seed is auto-generated and saved to ~/.mu/keys/wallet.seed
WALLET_SEED="24 word mnemonic phrase"

# Base RPC endpoint (optional)
BASE_RPC_URL="https://mainnet.base.org"

# Deposit polling interval in seconds (optional, default: 30)
DEPOSIT_POLL_INTERVAL="30"

# Stripe card payments (optional)
STRIPE_SECRET_KEY="sk_live_..."
STRIPE_PUBLISHABLE_KEY="pk_live_..."
STRIPE_WEBHOOK_SECRET="whsec_..."  # For verifying Stripe webhook events

# Quota (optional - these are defaults)
FREE_DAILY_SEARCHES="10"
CREDIT_COST_NEWS="1"
CREDIT_COST_NEWS_SUMMARY="1"
CREDIT_COST_VIDEO="2"
CREDIT_COST_VIDEO_WATCH="0"
CREDIT_COST_CHAT="3"
CREDIT_COST_EMAIL="4"
CREDIT_COST_PLACES_SEARCH="5"
CREDIT_COST_PLACES_NEARBY="2"
CREDIT_COST_WEATHER="1"
CREDIT_COST_WEATHER_POLLEN="1"
```

---

## Implementation

### HD Wallet

Mu uses an HD (Hierarchical Deterministic) wallet to derive unique deposit addresses per user:

- Master seed stored in `~/.mu/keys/wallet.seed` (or `WALLET_SEED` env var)
- BIP44 derivation path: `m/44'/60'/0'/0/{index}`
- Index 0 = treasury address
- Each user gets a deterministic index based on their user ID

### Quota Check Flow

1. User initiates search/chat
2. Check if admin → allow (no charge)
3. Check daily free quota → allow if available, decrement
4. Check wallet balance → allow if sufficient, deduct credits
5. Otherwise → show "quota exceeded" with options

### Deposit Detection

Deposits are detected automatically by polling blockchain nodes:

1. A background watcher polls each supported chain every 30 seconds (configurable via `DEPOSIT_POLL_INTERVAL`)
2. For **ETH**: detects balance increases on user deposit addresses
3. For **ERC-20 tokens**: scans `Transfer` event logs for incoming token transfers
4. Fetches the current token price from CoinGecko (cached 5 minutes)
5. Calculates credits (1 credit = $0.01 USD) and adds them to the user's wallet
6. Records each deposit by transaction hash to prevent double-crediting

The watcher starts automatically on server launch when a wallet seed is configured.

**Environment variable:** `DEPOSIT_POLL_INTERVAL` (default: `30` seconds)

---

## Security

- Wallet seed file has 0600 permissions (owner read/write only)
- Never log seed or private keys
- Full transaction audit trail
- Never allow negative balance
- Deposit addresses derived deterministically (no key storage per user)
