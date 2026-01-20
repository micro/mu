package wallet

import (
	"fmt"
	"net/http"
	"strings"

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

	// WalletConnect / Direct wallet payment section
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Pay with Wallet</h3>`)
	sb.WriteString(`<p class="text-muted text-sm">Connect your wallet to send ETH or tokens directly.</p>`)
	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<button id="connect-wallet-btn" class="btn">Connect Wallet</button>`)
	sb.WriteString(`<span id="wallet-status" class="ml-3 text-sm"></span>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<div id="send-section" style="display:none;" class="mt-4">`)
	sb.WriteString(`<p class="text-sm">Connected: <span id="connected-address"></span></p>`)
	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label class="text-sm">Amount (ETH)</label>`)
	sb.WriteString(`<input type="number" id="send-amount" value="0.01" step="0.001" min="0.001" style="width: 120px;" class="ml-2">`)
	sb.WriteString(`<button id="send-btn" class="btn ml-3">Send</button>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`<p class="text-sm text-muted mt-2">~$<span id="usd-estimate">25</span> → ~<span id="credits-estimate">2500</span> credits</p>`)
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Manual deposit address section
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Or Send Manually</h3>`)
	sb.WriteString(`<p class="text-muted text-sm">Base Network (Ethereum L2)</p>`)
	sb.WriteString(fmt.Sprintf(`<code class="deposit-address">%s</code>`, depositAddr))
	sb.WriteString(`<p class="text-sm mt-3"><button onclick="navigator.clipboard.writeText('` + depositAddr + `'); this.textContent='Copied!'; setTimeout(() => this.textContent='Copy Address', 2000)" class="btn-secondary">Copy Address</button></p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Supported Tokens</h3>`)
	sb.WriteString(`<ul>`)
	sb.WriteString(`<li><strong>ETH</strong> - Ethereum</li>`)
	sb.WriteString(`<li><strong>USDC</strong> - USD Coin</li>`)
	sb.WriteString(`<li><strong>DAI</strong> - Dai Stablecoin</li>`)
	sb.WriteString(`<li>Any ERC-20 token on Base</li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>How it works</h3>`)
	sb.WriteString(`<ol>`)
	sb.WriteString(`<li>Send any supported token to the address above</li>`)
	sb.WriteString(`<li>Wait for confirmation (~1 minute)</li>`)
	sb.WriteString(`<li>Credits are added automatically based on current rates</li>`)
	sb.WriteString(`</ol>`)
	sb.WriteString(`<p class="text-sm text-muted">1 credit = 1p · Minimum deposit: $1 equivalent</p>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Important</h3>`)
	sb.WriteString(`<ul class="text-sm text-muted">`)
	sb.WriteString(`<li>Only send on <strong>Base network</strong></li>`)
	sb.WriteString(`<li>Sending on wrong network will result in lost funds</li>`)
	sb.WriteString(`<li>Deposits typically confirm within 1-2 minutes</li>`)
	sb.WriteString(`</ul>`)
	sb.WriteString(`</div>`)

	// Add wallet connection script
	sb.WriteString(fmt.Sprintf(`
<script>
const DEPOSIT_ADDRESS = '%s';
const BASE_CHAIN_ID = '0x2105'; // 8453 in hex

let connectedAddress = null;

// Check for injected wallet (MetaMask, etc)
async function connectWallet() {
	const btn = document.getElementById('connect-wallet-btn');
	const status = document.getElementById('wallet-status');
	
	if (!window.ethereum) {
		status.textContent = 'No wallet found. Install MetaMask.';
		status.className = 'ml-3 text-sm text-error';
		return;
	}
	
	try {
		btn.textContent = 'Connecting...';
		btn.disabled = true;
		
		// Request accounts
		const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
		connectedAddress = accounts[0];
		
		// Switch to Base network
		try {
			await window.ethereum.request({
				method: 'wallet_switchEthereumChain',
				params: [{ chainId: BASE_CHAIN_ID }]
			});
		} catch (switchError) {
			// Chain not added, add it
			if (switchError.code === 4902) {
				await window.ethereum.request({
					method: 'wallet_addEthereumChain',
					params: [{
						chainId: BASE_CHAIN_ID,
						chainName: 'Base',
						nativeCurrency: { name: 'ETH', symbol: 'ETH', decimals: 18 },
						rpcUrls: ['https://mainnet.base.org'],
						blockExplorerUrls: ['https://basescan.org']
					}]
				});
			}
		}
		
		// Update UI
		btn.textContent = 'Connected';
		btn.className = 'btn-secondary';
		document.getElementById('send-section').style.display = 'block';
		document.getElementById('connected-address').textContent = 
			connectedAddress.slice(0,6) + '...' + connectedAddress.slice(-4);
		
		updateEstimate();
		
	} catch (err) {
		status.textContent = err.message;
		status.className = 'ml-3 text-sm text-error';
		btn.textContent = 'Connect Wallet';
		btn.disabled = false;
	}
}

async function sendPayment() {
	const btn = document.getElementById('send-btn');
	const amount = document.getElementById('send-amount').value;
	
	if (!connectedAddress || !amount) return;
	
	try {
		btn.textContent = 'Confirm in wallet...';
		btn.disabled = true;
		
		// Convert ETH to wei (hex)
		const weiValue = '0x' + (BigInt(Math.floor(parseFloat(amount) * 1e18))).toString(16);
		
		const txHash = await window.ethereum.request({
			method: 'eth_sendTransaction',
			params: [{
				from: connectedAddress,
				to: DEPOSIT_ADDRESS,
				value: weiValue
			}]
		});
		
		btn.textContent = 'Sent!';
		btn.className = 'btn-success';
		
		// Show success message
		const status = document.getElementById('wallet-status');
		status.innerHTML = 'Transaction sent! <a href="https://basescan.org/tx/' + txHash + '" target="_blank">View</a>';
		status.className = 'ml-3 text-sm text-success';
		
	} catch (err) {
		btn.textContent = 'Send';
		btn.disabled = false;
		if (err.code !== 4001) { // User didn't reject
			alert('Error: ' + err.message);
		}
	}
}

function updateEstimate() {
	const amount = parseFloat(document.getElementById('send-amount').value) || 0;
	// Rough ETH price estimate (will be replaced by actual detection)
	const ethPrice = 2500;
	const usd = amount * ethPrice;
	const credits = Math.floor(usd * 100);
	
	document.getElementById('usd-estimate').textContent = usd.toFixed(0);
	document.getElementById('credits-estimate').textContent = credits;
}

// Event listeners
document.getElementById('connect-wallet-btn').addEventListener('click', connectWallet);
document.getElementById('send-btn').addEventListener('click', sendPayment);
document.getElementById('send-amount').addEventListener('input', updateEstimate);

// Check if already connected
if (window.ethereum && window.ethereum.selectedAddress) {
	connectWallet();
}
</script>
`, depositAddr))

	html := app.RenderHTMLForRequest("Deposit", "Add credits via crypto", sb.String(), r)
	w.Write([]byte(html))
}
