# Wallet & Credits

## Philosophy

Mu is a tool, not a destination. Like Google Search in 2000 — you arrive with intent, get what you need, and leave.

Credits are a straightforward way to pay for what you use. No dark patterns, no pressure to upgrade, no "unlimited" tiers that incentivize us to maximize your engagement.

- **Free tier**: 10 AI queries/day - enough for casual utility use
- **Pay-as-you-go**: Top up with a card, use credits when you need more
- **Self-host**: Run your own instance for free, forever

We charge because LLMs and APIs cost money. Here's our actual cost breakdown — we're not extracting margin, just covering infrastructure.

## How It Works

### Credits

- **1 credit = £0.01 GBP** (1 penny)
- Credits stored as integers to avoid floating-point issues
- Top up via card payment (Stripe)
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

### Quota Check Flow

1. User initiates search/chat
2. Check if admin → allow (no charge)
3. Check daily free quota → allow if available, decrement
4. Check wallet balance → allow if sufficient, deduct credits
5. Otherwise → show "quota exceeded" with options

---

## Security

- Full transaction audit trail
- Never allow negative balance
