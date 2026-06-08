package trade

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/trade" && r.Method == "GET":
		handlePage(w, r)
	case path == "/trade/wallet" && r.Method == "POST":
		handleCreateWallet(w, r)
	case path == "/trade/quote" && r.Method == "GET":
		handleQuote(w, r)
	case path == "/trade/swap" && r.Method == "POST":
		handleSwap(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	sess, acc := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}

	var b strings.Builder

	wallet := GetWallet(acc.ID)

	if wallet == nil {
		// No wallet yet — show setup
		b.WriteString(`<div class="card" style="max-width:480px">`)
		b.WriteString(`<h3>Trading</h3>`)
		b.WriteString(`<p>Trade tokens on Base via Uniswap. To get started, create a trading wallet.</p>`)
		b.WriteString(`<form method="POST" action="/trade/wallet">`)
		b.WriteString(`<button type="submit" class="btn" style="margin-top:12px">Create Wallet</button>`)
		b.WriteString(`</form>`)
		b.WriteString(`<p class="text-sm text-muted" style="margin-top:12px">Or import an existing wallet:</p>`)
		b.WriteString(`<form method="POST" action="/trade/wallet" style="margin-top:8px">`)
		b.WriteString(`<input type="text" name="private_key" placeholder="Private key (hex)" class="form-input w-full" style="font-size:13px">`)
		b.WriteString(`<button type="submit" class="btn btn-secondary" style="margin-top:8px">Import</button>`)
		b.WriteString(`</form>`)
		b.WriteString(`</div>`)
	} else {
		// Wallet exists — show balances and trade form
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Wallet</h3>`)
		b.WriteString(fmt.Sprintf(`<p class="text-sm" style="word-break:break-all"><strong>Address:</strong> %s</p>`, wallet.Address))

		if Enabled() {
			balances := GetBalances(wallet.Address)
			if len(balances) > 0 {
				b.WriteString(`<table class="stats-table" style="margin-top:8px">`)
				for symbol, amount := range balances {
					b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td></tr>`, symbol, amount))
				}
				b.WriteString(`</table>`)
			} else {
				b.WriteString(`<p class="text-sm text-muted" style="margin-top:8px">No balances. Send ETH or USDC to your wallet address to start trading.</p>`)
			}
		} else {
			b.WriteString(`<p class="text-sm text-muted" style="margin-top:8px">Set TRADE_RPC_URL to enable balance fetching and trading.</p>`)
		}
		b.WriteString(`</div>`)

		// Swap form
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Swap</h3>`)
		if !Enabled() {
			b.WriteString(`<p class="text-sm text-muted">Trading requires TRADE_RPC_URL to be configured.</p>`)
		} else {
			b.WriteString(`<form method="POST" action="/trade/swap">`)
			b.WriteString(`<div style="display:flex;gap:8px;align-items:end;flex-wrap:wrap">`)
			b.WriteString(`<div><label class="text-sm">Amount</label><input type="text" name="amount" placeholder="100" required class="form-input" style="width:120px"></div>`)
			b.WriteString(`<div><label class="text-sm">From</label><select name="from" class="form-input">`)
			for symbol := range Tokens {
				b.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, symbol, symbol))
			}
			b.WriteString(`</select></div>`)
			b.WriteString(`<div style="padding:8px;font-size:18px">→</div>`)
			b.WriteString(`<div><label class="text-sm">To</label><select name="to" class="form-input">`)
			for symbol := range Tokens {
				b.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, symbol, symbol))
			}
			b.WriteString(`</select></div>`)
			b.WriteString(`<button type="submit" class="btn">Get Quote</button>`)
			b.WriteString(`</div>`)
			b.WriteString(`</form>`)
		}
		b.WriteString(`</div>`)

		// Trade history
		trades := GetTrades(acc.ID, 20)
		if len(trades) > 0 {
			b.WriteString(`<div class="card">`)
			b.WriteString(`<h3>History</h3>`)
			b.WriteString(`<table class="data-table">`)
			b.WriteString(`<tr><th>Date</th><th>Swap</th><th>Status</th></tr>`)
			for i := len(trades) - 1; i >= 0; i-- {
				t := trades[i]
				date := t.CreatedAt
				if len(date) > 16 {
					date = date[:16]
				}
				swap := t.AmountIn + " → " + t.AmountOut
				status := t.Status
				if t.TxHash != "" {
					status = fmt.Sprintf(`<a href="https://basescan.org/tx/%s" target="_blank">%s</a>`, t.TxHash, t.Status)
				}
				b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`, date, swap, status))
			}
			b.WriteString(`</table>`)
			b.WriteString(`</div>`)
		}
	}

	html := app.RenderHTMLForRequest("Trade", "DEX trading via Uniswap on Base", b.String(), r)
	w.Write([]byte(html))
}

func handleCreateWallet(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}

	privKey := strings.TrimSpace(r.FormValue("private_key"))
	if privKey != "" {
		if _, err := ImportWallet(sess.Account, privKey); err != nil {
			http.Redirect(w, r, "/trade?error="+err.Error(), http.StatusSeeOther)
			return
		}
	} else {
		if _, err := CreateWallet(sess.Account); err != nil {
			http.Redirect(w, r, "/trade?error="+err.Error(), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/trade", http.StatusSeeOther)
}

func handleQuote(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	amount := r.URL.Query().Get("amount")

	quote, err := GetQuote(from, to, amount)
	if err != nil {
		app.RespondJSON(w, map[string]string{"error": err.Error()})
		return
	}
	app.RespondJSON(w, quote)
}

func handleSwap(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}

	from := r.FormValue("from")
	to := r.FormValue("to")
	amount := r.FormValue("amount")

	if from == to {
		http.Redirect(w, r, "/trade?error=from+and+to+must+be+different", http.StatusSeeOther)
		return
	}

	trade, err := ExecuteSwap(sess.Account, from, to, amount)
	if err != nil {
		http.Redirect(w, r, "/trade?error="+err.Error(), http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, trade)
		return
	}

	// Show quote result
	var b strings.Builder
	b.WriteString(`<div class="card" style="max-width:480px">`)
	b.WriteString(`<h3>Swap Quote</h3>`)
	b.WriteString(fmt.Sprintf(`<p><strong>%s</strong> → <strong>%s</strong></p>`, trade.AmountIn, trade.AmountOut))
	b.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">Status: %s</p>`, trade.Status))
	b.WriteString(`<p style="margin-top:12px"><a href="/trade">← Back to Trade</a></p>`)
	b.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Swap Quote", "Trade result", b.String(), r)
	w.Write([]byte(html))
}
