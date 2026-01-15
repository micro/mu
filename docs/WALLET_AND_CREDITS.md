# Wallet & Credits

## Philosophy

Credits are a straightforward way to pay for what you use. No dark patterns, no pressure to upgrade.

- **Free tier**: 10 searches/day - enough to try things out
- **Pay-as-you-go**: Top up your wallet, use credits when you need more
- **Membership**: For those who want to support Mu (£11/month) - unlimited everything

## How It Works

### Credits

- **1 credit = £0.01 GBP** (1 penny)
- Credits stored as integers to avoid floating-point issues
- Top up via card payment (Stripe)
- Credits never expire

### Daily Free Quota

Every registered user gets **10 free searches per day**:
- Resets at midnight UTC
- Covers news search, video search, and chat AI queries
- No credit card required

### Credit Costs

| Feature | Cost | Why |
|---------|------|-----|
| News Search | 1 credit (1p) | Indexed search |
| News Summary | 1 credit (1p) | AI-generated summary |
| Video Search | 2 credits (2p) | YouTube API cost |
| Video Watch | 2 credits (2p) | Ad-free viewing |
| Chat AI Query | 3 credits (3p) | LLM inference cost |
| Chat Room | 1 credit (1p) | Room creation |
| App Create | 5 credits (5p) | AI app generation |
| App Modify | 3 credits (3p) | AI app modification |
| Agent Run | 5 credits (5p) | Agent task execution |

### Who Pays What

| User Type | Daily Free | Credits | Notes |
|-----------|------------|---------|-------|
| Guest | 0 | N/A | Must register |
| Registered | 10 searches | Pay-as-you-go | When free quota exceeded |
| Member | Unlimited | Not needed | £11/month |
| Admin | Unlimited | Not needed | Site administrators |

## Top-up Options

| Amount | Credits | Bonus |
|--------|---------|-------|
| £5 | 500 | - |
| £10 | 1,050 | +5% |
| £25 | 2,750 | +10% |
| £50 | 5,750 | +15% |

Larger top-ups get bonus credits to offset payment processing fees.

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
    Searches int    `json:"searches"` // Free searches used today
}
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/wallet` | View balance and transaction history |
| POST | `/wallet/topup` | Start Stripe checkout |
| GET | `/wallet/success` | Stripe success redirect |
| GET | `/wallet/cancel` | Stripe cancel redirect |
| POST | `/wallet/webhook` | Stripe payment confirmation |
| GET | `/plans` | View available plans |

---

## Pages

### /plans

Overview of all options:
- Free tier (10 searches/day)
- Pay-as-you-go (credit costs)
- Membership (£11/month, unlimited)

### /wallet

- Current balance
- Daily quota progress
- Top-up buttons
- Transaction history

### /membership

- Benefits of membership
- Join link (existing functionality)

---

## Environment Variables

```bash
# Stripe
STRIPE_SECRET_KEY="sk_live_xxx"
STRIPE_PUBLISHABLE_KEY="pk_live_xxx"  
STRIPE_WEBHOOK_SECRET="whsec_xxx"

# Optional: Pre-configured Stripe Price IDs
STRIPE_PRICE_500="price_xxx"   # £5
STRIPE_PRICE_1000="price_xxx"  # £10
STRIPE_PRICE_2500="price_xxx"  # £25
STRIPE_PRICE_5000="price_xxx"  # £50

# Quota (optional - these are defaults)
FREE_DAILY_SEARCHES="10"
CREDIT_COST_NEWS="1"
CREDIT_COST_NEWS_SUMMARY="1"
CREDIT_COST_VIDEO="2"
CREDIT_COST_VIDEO_WATCH="2"
CREDIT_COST_CHAT="3"
```

---

## Implementation

### Quota Check Flow

1. User initiates search/chat
2. Check if member/admin → allow (no charge)
3. Check daily free quota → allow if available, decrement
4. Check wallet balance → allow if sufficient, deduct credits
5. Otherwise → show "quota exceeded" with options

### Stripe Webhook

1. Receive `checkout.session.completed` event
2. Verify webhook signature
3. Extract user ID and credit amount from metadata
4. Add credits to wallet
5. Record transaction

---

## Security

- Stripe webhook signatures always verified in production
- Idempotent credit additions (check for duplicate session IDs)
- Mutex on wallet balance updates
- Full transaction audit trail
- Never allow negative balance
