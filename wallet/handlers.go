package wallet

import (
	"encoding/base64"
	"fmt"
	"net/http"
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

	if isAdmin {
		// Admin status
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Status</h3>`)
		sb.WriteString(`<p>Admin · Full access</p>`)
		sb.WriteString(`</div>`)
	} else {
		// Balance
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h3>Balance</h3>`)
		sb.WriteString(fmt.Sprintf(`<p>%d credits</p>`, wallet.Balance))
		sb.WriteString(`<p><a href="/wallet/deposit">Add Credits →</a></p>`)
		sb.WriteString(`</div>`)

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
		sb.WriteString(`<p class="text-sm text-muted">Want unlimited and free? <a href="https://github.com/asim/mu">Self-host your own instance</a>.</p>`)
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
	sb.WriteString(fmt.Sprintf(`<tr><td>External email</td><td>%dp</td></tr>`, CostExternalEmail))
	sb.WriteString(fmt.Sprintf(`<tr><td>App create</td><td>%dp</td></tr>`, CostAppCreate))
	sb.WriteString(fmt.Sprintf(`<tr><td>App modify</td><td>%dp</td></tr>`, CostAppModify))
	sb.WriteString(fmt.Sprintf(`<tr><td>Agent run</td><td>%dp</td></tr>`, CostAgentRun))
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
			amountPrefix := "-"
			if tx.Amount > 0 {
				amountPrefix = "+"
			}
			sb.WriteString(fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s%d</td>
				<td>%d</td>
			</tr>`, tx.CreatedAt.Format("2 Jan 15:04"), typeLabel, amountPrefix, abs(tx.Amount), tx.Balance))
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
	sb.WriteString(`<li><a href="/wallet/deposit">Add credits</a></li>`)
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
	case path == "/wallet/deposit" && r.Method == "GET":
		handleDepositPage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleWalletPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	content := WalletPage(sess.Account)
	html := app.RenderHTMLForRequest("Wallet", "Manage your credits", content, r)
	w.Write([]byte(html))
}

func handleDepositPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	// Initialize crypto wallet if needed
	if err := InitCryptoWallet(); err != nil {
		app.Log("wallet", "Failed to init crypto wallet: %v", err)
		content := `<div class="card"><p class="text-error">Crypto wallet not available. Please try again later.</p></div>`
		html := app.RenderHTMLForRequest("Deposit", "Add credits", content, r)
		w.Write([]byte(html))
		return
	}

	// Get user's deposit address
	depositAddr, err := GetUserDepositAddress(sess.Account)
	if err != nil {
		app.Log("wallet", "Failed to get deposit address: %v", err)
		content := `<div class="card"><p class="text-error">Failed to generate deposit address.</p></div>`
		html := app.RenderHTMLForRequest("Deposit", "Add credits", content, r)
		w.Write([]byte(html))
		return
	}

	var sb strings.Builder

	// Generate QR code with ethereum: URI for mobile wallets
	ethURI := "ethereum:" + depositAddr + "@8453" // @8453 = Base chainId
	qrPNG, _ := qrcode.Encode(ethURI, qrcode.Medium, 200)
	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	// Deposit address with QR
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Deposit Address</h3>`)
	sb.WriteString(`<p class="text-muted text-sm">Base Network</p>`)
	sb.WriteString(fmt.Sprintf(`<img src="data:image/png;base64,%s" alt="QR Code" class="qr-code">`, qrBase64))
	sb.WriteString(fmt.Sprintf(`<code class="deposit-address">%s</code>`, depositAddr))
	sb.WriteString(`<p class="text-sm mt-3">`)
	sb.WriteString(`<button onclick="navigator.clipboard.writeText('` + depositAddr + `'); this.textContent='Copied!'; setTimeout(() => this.textContent='Copy', 2000)" class="btn-secondary">Copy</button>`)
	sb.WriteString(fmt.Sprintf(` <a href="%s" class="btn ml-2">Open Wallet</a>`, ethURI))
	sb.WriteString(`</p>`)
	sb.WriteString(`</div>`)

	// Supported tokens
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Supported</h3>`)
	sb.WriteString(`<ul>`)
	sb.WriteString(`<li><strong>ETH</strong></li>`)
	sb.WriteString(`<li><strong>USDC</strong></li>`)
	sb.WriteString(`<li><strong>ERC-20</strong> tokens</li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`<p class="text-sm text-muted">Base network · 1 credit = 1p</p>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Deposit", "Add credits", sb.String(), r)
	w.Write([]byte(html))
}
