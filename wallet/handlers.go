package wallet

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/skip2/go-qrcode"

	"mu/app"
	"mu/auth"
)

// WalletPage renders the wallet page HTML
func WalletPage(userID string) string {
	wallet := GetWallet(userID)
	usage := GetDailyUsage(userID)
	freeRemaining := GetFreeSearchesRemaining(userID)
	transactions := GetTransactions(userID, 20)

	// Check if user is admin
	isAdmin := false
	if acc, err := auth.GetAccount(userID); err == nil {
		isAdmin = acc.Admin
	}

	var sb strings.Builder

	// Balance
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Balance</h3>`)
	if isAdmin {
		sb.WriteString(`<p class="text-sm text-muted">Admin · Unlimited access</p>`)
		if wallet.Balance > 0 {
			sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
		}
	} else {
		sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
	}
	sb.WriteString(`<p><a href="/wallet/topup">Add Credits →</a></p>`)
	sb.WriteString(`</div>`)

	if !isAdmin {
		// Daily quota
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Free Queries</h3>`)
		usedPct := float64(usage.Searches) / float64(FreeDailySearches) * 100
		if usedPct > 100 {
			usedPct = 100
		}
		sb.WriteString(`<div class="progress">`)
		sb.WriteString(fmt.Sprintf(`<div class="progress-bar" style="width: %.0f%%;"></div>`, usedPct))
		sb.WriteString(`</div>`)
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%d of %d remaining · Resets midnight UTC</p>`, freeRemaining, FreeDailySearches))
		sb.WriteString(`</div>`)

		// Self-hosting note
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Self-Host</h3>`)
		sb.WriteString(`<p class="text-sm text-muted">Want unlimited and free? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
		sb.WriteString(`</div>`)
	}

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td>%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>News summary</td><td>%dp</td></tr>`, CostNewsSummary))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td>%dp</td></tr>`, CostVideoSearch))
	if CostVideoWatch > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Video watch</td><td>%dp</td></tr>`, CostVideoWatch))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td>%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places search</td><td>%dp</td></tr>`, CostPlacesSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places nearby</td><td>%dp</td></tr>`, CostPlacesNearby))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)

	// Transaction history
	if len(transactions) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>History</h3>`)
		sb.WriteString(`<table class="data-table">`)
		sb.WriteString(`<tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th></tr>`)

		for _, tx := range transactions {
			typeLabel := tx.Operation
			if tx.Type == TxTopup {
				typeLabel = "Deposit"
			}
			var amountStr string
			if tx.Amount == 0 {
				amountStr = "free"
			} else if tx.Amount > 0 {
				amountStr = fmt.Sprintf("+%d", tx.Amount)
			} else {
				amountStr = fmt.Sprintf("-%d", abs(tx.Amount))
			}
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountStr, tx.Balance))
		}

		sb.WriteString(`</table>`)
		sb.WriteString(`</div>`)
	}

	return sb.String()
}

// QuotaExceededPage renders the quota exceeded message
func QuotaExceededPage(operation string, cost int) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card center-card-md">`)
	sb.WriteString(`<h2>Daily Limit Reached</h2>`)
	sb.WriteString(`<p>You've used your free queries for today.</p>`)
	sb.WriteString(`<h3 class="mt-5">Options</h3>`)
	sb.WriteString(`<ul class="options-list">`)
	sb.WriteString(`<li>Wait until midnight UTC for more free queries</li>`)
	sb.WriteString(fmt.Sprintf(`<li><a href="/wallet">Use credits</a> (%d credit%s for this)</li>`, cost, pluralize(cost)))
	sb.WriteString(`<li><a href="/wallet/topup">Add credits</a></li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Handler handles wallet-related HTTP requests
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check for balance JSON endpoint
	if r.URL.Query().Get("balance") == "1" {
		sess, _ := auth.TrySession(r)
		if sess == nil {
			app.RespondJSON(w, map[string]int{"balance": 0})
			return
		}
		balance := GetBalance(sess.Account)
		app.RespondJSON(w, map[string]int{"balance": balance})
		return
	}

	switch {
	case path == "/wallet" && r.Method == "GET":
		handleWalletPage(w, r)
	case path == "/wallet/topup" && r.Method == "GET" && app.WantsJSON(r):
		handleTopupJSON(w, r)
	case path == "/wallet/topup" && r.Method == "GET":
		handleDepositPage(w, r)
	case path == "/wallet/stripe/checkout" && r.Method == "POST":
		handleStripeCheckout(w, r)
	case path == "/wallet/stripe/success" && r.Method == "GET":
		handleStripeSuccess(w, r)
	case path == "/wallet/stripe/webhook" && r.Method == "POST":
		HandleStripeWebhook(w, r)
	case path == "/wallet/crypto/verify" && r.Method == "POST":
		handleCryptoVerify(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleWalletPage(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)

	var content string
	if sess != nil {
		content = WalletPage(sess.Account)
	} else {
		content = PublicWalletPage()
	}

	html := app.RenderHTMLForRequest("Wallet", "Credits and pricing", content, r)
	w.Write([]byte(html))
}

// PublicWalletPage renders the wallet page for unauthenticated users
func PublicWalletPage() string {
	var sb strings.Builder

	// Intro
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Credits &amp; Pricing</h3>`)
	sb.WriteString(`<p>Mu is free with ` + fmt.Sprintf("%d", FreeDailySearches) + ` queries/day. Need more? Top up and pay as you go — no subscription required.</p>`)
	sb.WriteString(`<p><a href="/login" class="btn">Login to view your balance</a>&nbsp;<a href="/signup" class="btn btn-secondary">Sign up free</a></p>`)
	sb.WriteString(`</div>`)

	// Credit costs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Costs</h3>`)
	sb.WriteString(`<table class="stats-table">`)
	sb.WriteString(fmt.Sprintf(`<tr><td>News search</td><td>%dp</td></tr>`, CostNewsSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>News summary</td><td>%dp</td></tr>`, CostNewsSummary))
	sb.WriteString(fmt.Sprintf(`<tr><td>Video search</td><td>%dp</td></tr>`, CostVideoSearch))
	if CostVideoWatch > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Video watch</td><td>%dp</td></tr>`, CostVideoWatch))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Chat query</td><td>%dp</td></tr>`, CostChatQuery))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places search</td><td>%dp</td></tr>`, CostPlacesSearch))
	sb.WriteString(fmt.Sprintf(`<tr><td>Places nearby</td><td>%dp</td></tr>`, CostPlacesNearby))
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)

	// Topup options
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Top Up</h3>`)
	sb.WriteString(`<p>Add credits to your account via card or crypto:</p>`)
	sb.WriteString(`<ul>`)
	if StripeEnabled() {
		sb.WriteString(`<li><strong>Card</strong> — secure payment via Stripe</li>`)
	}
	if CryptoWalletEnabled() {
		sb.WriteString(`<li><strong>Crypto</strong> — send ETH to your deposit address on Ethereum mainnet</li>`)
	}
	sb.WriteString(`</ul>`)
	sb.WriteString(`<p><a href="/login">Login</a> or <a href="/signup">sign up</a> to top up.</p>`)
	sb.WriteString(`</div>`)

	// Self-hosting note
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Self-Host</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">Want unlimited and free? <a href="https://github.com/micro/mu">Self-host your own instance</a>.</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

// Supported chains for deposits
var depositChains = []struct {
	ID      string
	Name    string
	ChainID int
}{
	{"ethereum", "Ethereum", 1},
}

func handleDepositPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	// Get selected method (stripe or crypto)
	method := r.URL.Query().Get("method")
	if method == "" {
		method = "stripe" // Default to stripe
	}

	var sb strings.Builder

	// Method tabs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Topup using card or crypto</h3>`)
	sb.WriteString(`<div class="d-flex gap-2">`)
	if StripeEnabled() {
		stripeActive := ""
		if method == "stripe" {
			stripeActive = " btn-primary"
		} else {
			stripeActive = " btn-secondary"
		}
		sb.WriteString(fmt.Sprintf(`<a href="/wallet/topup?method=stripe" class="btn%s">Card</a>`, stripeActive))
	}
	if CryptoWalletEnabled() {
		cryptoActive := ""
		if method == "crypto" {
			cryptoActive = " btn-primary"
		} else {
			cryptoActive = " btn-secondary"
		}
		sb.WriteString(fmt.Sprintf(`<a href="/wallet/topup?method=crypto" class="btn%s">Crypto</a>`, cryptoActive))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	if method == "stripe" && StripeEnabled() {
		sb.WriteString(renderStripeDeposit(r.URL.Query().Get("error")))
	} else if method == "crypto" && CryptoWalletEnabled() {
		sb.WriteString(renderCryptoDeposit(sess.Account, r))
	} else if StripeEnabled() {
		sb.WriteString(renderStripeDeposit(r.URL.Query().Get("error")))
	} else if CryptoWalletEnabled() {
		sb.WriteString(renderCryptoDeposit(sess.Account, r))
	} else {
		sb.WriteString(`<div class="card"><p class="text-error">No payment methods available.</p></div>`)
	}

	html := app.RenderHTMLForRequest("Add Credits", "Top up your wallet", sb.String(), r)
	w.Write([]byte(html))
}

func renderStripeDeposit(errMsg string) string {
	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Select Amount</h3>`)
	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}
	sb.WriteString(`<form method="POST" action="/wallet/stripe/checkout">`)
	sb.WriteString(`<div class="d-flex flex-column gap-2">`)

	for _, tier := range StripeTopupTiers {
		bonusText := ""
		if tier.BonusPct > 0 {
			bonusText = fmt.Sprintf(" <span class=\"text-success\">+%d%% bonus</span>", tier.BonusPct)
		}
		sb.WriteString(fmt.Sprintf(`<label class="topup-option">`+
			`<input type="radio" name="amount" value="%d"> `+
			`<strong>%s</strong> → %d credits%s`+
			`</label>`, tier.Amount, tier.Label, tier.Credits, bonusText))
	}

	sb.WriteString(`</div>`)
	sb.WriteString(`<button type="submit" class="btn mt-4">Continue to Payment</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<p class="text-sm text-muted">Secure payment via Stripe. 1 credit = 1p.</p>`)
	sb.WriteString(`</div>`)

	return sb.String()
}

func renderCryptoDeposit(userID string, r *http.Request) string {
	// Initialize crypto wallet if needed
	if err := InitCryptoWallet(); err != nil {
		app.Log("wallet", "Failed to init crypto wallet: %v", err)
		return `<div class="card"><p class="text-error">Crypto wallet not available. Please try again later.</p></div>`
	}

	// Get user's deposit address
	depositAddr, err := GetUserDepositAddress(userID)
	if err != nil {
		app.Log("wallet", "Failed to get deposit address: %v", err)
		return `<div class="card"><p class="text-error">Failed to generate deposit address.</p></div>`
	}

	var sb strings.Builder

	// Show any error from a previous verification attempt
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-error">%s</p></div>`, errMsg))
	}

	// QR code for mobile / WalletConnect wallets (EIP-681 URI, Ethereum mainnet)
	ethURI := fmt.Sprintf("ethereum:%s@1", depositAddr)
	qrPNG, _ := qrcode.Encode(ethURI, qrcode.Medium, 200)
	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	// Deposit address card
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Your Ethereum Deposit Address</h3>`)
	sb.WriteString(fmt.Sprintf(`<img src="data:image/png;base64,%s" alt="QR Code" class="qr-code">`, qrBase64))
	sb.WriteString(fmt.Sprintf(`<code class="deposit-address">%s</code>`, depositAddr))
	sb.WriteString(`<p class="text-sm mt-3">`)
	sb.WriteString(`<button onclick="navigator.clipboard.writeText('` + depositAddr + `'); this.textContent='Copied!'; setTimeout(() => this.textContent='Copy Address', 2000)" class="btn-secondary">Copy Address</button>`)
	sb.WriteString(`</p>`)
	sb.WriteString(`<p class="text-xs text-muted mt-2">Scan with any WalletConnect-compatible wallet, or copy address to send from your wallet app</p>`)
	sb.WriteString(`</div>`)

	// Connect & Pay card — uses window.ethereum (MetaMask / injected provider)
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Connect Wallet &amp; Pay</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">Send ETH on Ethereum mainnet. You will receive 1 credit per $0.01 at the market rate.</p>`)
	sb.WriteString(`<button id="connect-pay-btn" class="btn mt-2" onclick="connectAndPay()">Connect Wallet &amp; Pay</button>`)
	sb.WriteString(`</div>`)

	// Confirm payment form — paste tx hash after sending
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Confirm Your Payment</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">After sending ETH, enter the transaction hash to confirm and receive credits instantly:</p>`)
	sb.WriteString(`<form id="verify-form" method="POST" action="/wallet/crypto/verify">`)
	sb.WriteString(`<input type="hidden" name="chain" value="ethereum">`)
	sb.WriteString(`<div class="mt-2">`)
	sb.WriteString(`<input type="text" id="tx-hash-input" name="tx_hash" placeholder="0x..." style="width:100%; font-family:monospace; padding:8px; box-sizing:border-box;" required>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<button type="submit" class="btn mt-3">Verify &amp; Add Credits</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	// Rate info
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<p class="text-sm text-muted">1 credit = $0.01 &middot; Converted at market rate &middot; Ethereum mainnet only</p>`)
	sb.WriteString(`</div>`)

	// Inline JS: MetaMask / injected wallet support
	sb.WriteString(`<script>
async function connectAndPay() {
  if (!window.ethereum) {
    alert('No browser wallet detected.\nPlease install MetaMask or scan the QR code with a WalletConnect-compatible wallet.');
    return;
  }
  try {
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    const ethAmount = window.prompt('Enter ETH amount to send\n(e.g. 0.003 ≈ $10 at ~$3,000/ETH):');
    if (!ethAmount || isNaN(parseFloat(ethAmount))) return;
    // Convert to wei using integer arithmetic to avoid float precision loss
    const weiHex = '0x' + (BigInt(Math.round(parseFloat(ethAmount) * 1e6)) * 1000000000000n).toString(16);
    const txHash = await window.ethereum.request({
      method: 'eth_sendTransaction',
      params: [{ from: accounts[0], to: '` + depositAddr + `', value: weiHex }]
    });
    document.getElementById('tx-hash-input').value = txHash;
    document.getElementById('verify-form').submit();
  } catch (e) {
    alert('Error: ' + (e.message || e));
  }
}
</script>`)

	return sb.String()
}

func handleCryptoVerify(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/wallet/topup?method=crypto&error=Invalid+request", http.StatusSeeOther)
		return
	}

	txHash := strings.TrimSpace(r.FormValue("tx_hash"))
	chain := strings.TrimSpace(r.FormValue("chain"))
	if chain == "" {
		chain = "ethereum"
	}

	if txHash == "" {
		http.Redirect(w, r, "/wallet/topup?method=crypto&error=Please+enter+a+transaction+hash", http.StatusSeeOther)
		return
	}
	if !strings.HasPrefix(txHash, "0x") || len(txHash) != 66 {
		http.Redirect(w, r, "/wallet/topup?method=crypto&error=Invalid+transaction+hash+format+(must+be+66+hex+characters+starting+with+0x)", http.StatusSeeOther)
		return
	}

	credits, err := VerifyAndCreditDeposit(chain, txHash, sess.Account)
	if err != nil {
		app.Log("wallet", "Deposit verification failed for %s tx %s: %v", sess.Account, txHash, err)
		errMsg := url.QueryEscape("Verification failed: " + err.Error())
		http.Redirect(w, r, "/wallet/topup?method=crypto&error="+errMsg, http.StatusSeeOther)
		return
	}

	content := fmt.Sprintf(`<div class="card">
		<h2>Payment Confirmed</h2>
		<p>%d credits have been added to your wallet.</p>
		<p><a href="/wallet" class="btn">View Wallet</a></p>
	</div>`, credits)
	html := app.RenderHTMLForRequest("Payment Confirmed", "Credits added", content, r)
	w.Write([]byte(html))
}

// TopupMethod represents a payment method for wallet topup
type TopupMethod struct {
	Type    string            `json:"type"`              // "card" or "crypto"
	Tiers   []StripeTopupTier `json:"tiers,omitempty"`   // For card/Stripe
	Address string            `json:"address,omitempty"` // For crypto: deposit address
	Chains  []string          `json:"chains,omitempty"`  // For crypto: supported chains
}

func handleTopupJSON(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RespondJSON(w, map[string]string{"error": "authentication required"})
		return
	}

	var methods []TopupMethod

	if StripeEnabled() {
		methods = append(methods, TopupMethod{
			Type:  "card",
			Tiers: StripeTopupTiers,
		})
	}

	if CryptoWalletEnabled() {
		addr, err := GetUserDepositAddress(sess.Account)
		if err != nil {
			app.Log("wallet", "Failed to get deposit address for API: %v", err)
		} else {
			chains := make([]string, 0, len(depositChains))
			for _, c := range depositChains {
				chains = append(chains, c.ID)
			}
			methods = append(methods, TopupMethod{
				Type:    "crypto",
				Address: addr,
				Chains:  chains,
			})
		}
	}

	app.RespondJSON(w, map[string]interface{}{
		"methods": methods,
	})
}

func handleStripeCheckout(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/wallet/topup?method=stripe&error=Invalid+form+submission", http.StatusSeeOther)
		return
	}

	amountStr := r.FormValue("amount")
	var amount int
	fmt.Sscanf(amountStr, "%d", &amount)

	if amount == 0 {
		http.Redirect(w, r, "/wallet/topup?method=stripe&error=Please+select+an+amount", http.StatusSeeOther)
		return
	}

	// Build success/cancel URLs
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "mu.xyz") {
		scheme = "http"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	successURL := baseURL + "/wallet/stripe/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := baseURL + "/wallet/topup?method=stripe"

	// Create checkout session
	checkoutURL, err := CreateCheckoutSession(sess.Account, amount, successURL, cancelURL)
	if err != nil {
		app.Log("stripe", "checkout error: %v", err)
		content := `<div class="card"><h2>Payment Error</h2><p>Failed to create checkout session. Please try again.</p><p><a href="/wallet/topup?method=stripe" class="btn">Back</a></p></div>`
		html := app.RenderHTMLForRequest("Payment Error", "Checkout failed", content, r)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(html))
		return
	}

	// Redirect to Stripe
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

func handleStripeSuccess(w http.ResponseWriter, r *http.Request) {
	// Just show success message - actual crediting happens via webhook
	content := `<div class="card">
		<h2>Payment Successful</h2>
		<p>Your credits will be added to your account shortly.</p>
		<p><a href="/wallet" class="btn">View Wallet</a></p>
	</div>`
	html := app.RenderHTMLForRequest("Payment Complete", "Credits added", content, r)
	w.Write([]byte(html))
}
