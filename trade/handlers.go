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
	case path == "/trade/strategy" && r.Method == "POST":
		handleCreateStrategy(w, r)
	case path == "/trade/strategy/toggle" && r.Method == "POST":
		handleToggleStrategy(w, r)
	case path == "/trade/strategy/delete" && r.Method == "POST":
		handleDeleteStrategy(w, r)
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

	errMsg := r.URL.Query().Get("error")

	var b strings.Builder

	if errMsg != "" {
		b.WriteString(fmt.Sprintf(`<div class="card"><p style="color:#c00">%s</p></div>`, errMsg))
	}

	wallet := GetWallet(acc.ID)

	if wallet == nil {
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Get Started</h3>`)
		b.WriteString(`<p>Trade tokens on Base via Uniswap. Create a wallet to start.</p>`)
		b.WriteString(`<form method="POST" action="/trade/wallet" style="margin-top:12px">`)
		b.WriteString(`<button type="submit" class="btn">Create Wallet</button>`)
		b.WriteString(`</form>`)
		b.WriteString(`<details style="margin-top:16px"><summary style="font-size:13px;color:#888;cursor:pointer">Import existing wallet</summary>`)
		b.WriteString(`<form method="POST" action="/trade/wallet" style="margin-top:8px">`)
		b.WriteString(`<input type="text" name="private_key" placeholder="Private key (hex)" style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:13px;box-sizing:border-box;font-family:monospace">`)
		b.WriteString(`<button type="submit" class="btn btn-secondary" style="margin-top:8px">Import</button>`)
		b.WriteString(`</form></details>`)
		b.WriteString(`</div>`)
	} else {
		// Wallet
		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<h3>Wallet <span style="font-size:12px;font-weight:400;color:#888">· %s</span></h3>`, ActiveChainName()))
		shortAddr := wallet.Address
		if len(shortAddr) > 10 {
			shortAddr = shortAddr[:6] + "..." + shortAddr[len(shortAddr)-4:]
		}
		b.WriteString(fmt.Sprintf(`<p style="font-size:13px"><a href="`+ChainExplorer()+`/address/%s" target="_blank" style="font-family:monospace;word-break:break-all">%s</a></p>`, wallet.Address, shortAddr))

		b.WriteString(`<div style="display:flex;gap:8px;margin-top:10px;flex-wrap:wrap">`)
		b.WriteString(`<button onclick="copyAddr()" class="btn" style="font-size:13px;padding:6px 14px">Copy Address</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`<p style="font-size:12px;color:#888;margin-top:8px">Send ETH to this address to fund your wallet. Works from Coinbase, Binance, MetaMask, or any wallet.</p>`)
		b.WriteString(fmt.Sprintf(`<script>function copyAddr(){navigator.clipboard.writeText('%s');var b=event.target;b.textContent='Copied!';setTimeout(function(){b.textContent='Copy Address'},1500)}</script>`, wallet.Address))

		balances := GetBalances(wallet.Address)
		b.WriteString(`<table style="width:100%;margin-top:12px;font-size:14px;border-collapse:collapse">`)
		for _, symbol := range tokenOrder {
			amount := balances[symbol]
			if amount == "" {
				amount = "0"
			}
			b.WriteString(fmt.Sprintf(`<tr style="border-bottom:1px solid #f0f0f0"><td style="padding:6px 0;color:#555">%s</td><td style="padding:6px 0;text-align:right;font-family:monospace">%s</td></tr>`, symbol, amount))
		}
		b.WriteString(`</table>`)
		b.WriteString(`</div>`)

		// Swap form
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Swap</h3>`)
		b.WriteString(`<form method="POST" action="/trade/swap" id="swap-form">`)
		b.WriteString(`<div style="display:flex;gap:8px;margin-bottom:8px;align-items:end">`)
		b.WriteString(`<div style="flex:1"><label style="font-size:12px;color:#888;display:block;margin-bottom:4px">From</label>`)
		b.WriteString(`<select name="from" id="swap-from" onchange="updateMax()" style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:14px;font-family:inherit">`)
		for _, symbol := range tokenOrder {
			sel := ""
			if symbol == "ETH" {
				sel = " selected"
			}
			b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, symbol, sel, symbol))
		}
		b.WriteString(`</select></div>`)
		b.WriteString(`<button type="button" onclick="flipTokens()" style="padding:6px 10px;border:1px solid #ddd;border-radius:6px;background:#fff;cursor:pointer;font-size:16px;margin-bottom:1px" title="Swap direction">⇄</button>`)
		b.WriteString(`<div style="flex:1"><label style="font-size:12px;color:#888;display:block;margin-bottom:4px">To</label>`)
		b.WriteString(`<select name="to" id="swap-to" style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:14px;font-family:inherit">`)
		for _, symbol := range tokenOrder {
			sel := ""
			if symbol == "USDC" {
				sel = " selected"
			}
			b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, symbol, sel, symbol))
		}
		b.WriteString(`</select></div>`)
		b.WriteString(`</div>`)
		b.WriteString(`<label style="font-size:12px;color:#888;display:block;margin-bottom:4px">Amount</label>`)
		b.WriteString(`<div style="position:relative">`)
		b.WriteString(`<input type="text" name="amount" id="swap-amount" placeholder="0.00" required style="width:100%;padding:8px 50px 8px 8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:monospace">`)
		b.WriteString(`<button type="button" onclick="setMax()" style="position:absolute;right:6px;top:50%;transform:translateY(-50%);padding:2px 8px;font-size:12px;border:1px solid #ddd;border-radius:4px;background:#f5f5f5;cursor:pointer;color:#555">MAX</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`<button type="submit" class="btn" style="width:100%;margin-top:12px">Get Quote</button>`)
		b.WriteString(`</form>`)

		// Build JS balance map for max button
		b.WriteString(`<script>`)
		b.WriteString(`var balMap={`)
		for _, symbol := range tokenOrder {
			amount := balances[symbol]
			if amount == "" {
				amount = "0"
			}
			b.WriteString(fmt.Sprintf(`"%s":"%s",`, symbol, amount))
		}
		b.WriteString(`};`)
		b.WriteString(`function flipTokens(){var f=document.getElementById('swap-from'),t=document.getElementById('swap-to');var fv=f.value;f.value=t.value;t.value=fv;updateMax()}`)
		b.WriteString(`function updateMax(){var s=document.getElementById('swap-from').value;document.getElementById('swap-amount').placeholder=balMap[s]||'0.00'}`)
		b.WriteString(`function setMax(){var s=document.getElementById('swap-from').value;var v=balMap[s]||'0';document.getElementById('swap-amount').value=v}`)
		b.WriteString(`updateMax();`)
		b.WriteString(`</script>`)
		b.WriteString(`</div>`)

		// Strategies
		strats := GetStrategies(acc.ID)
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Strategies</h3>`)
		if len(strats) > 0 {
			for _, s := range strats {
				statusLabel := "Active"
				statusColor := "#28a745"
				if !s.Active {
					statusLabel = "Paused"
					statusColor = "#888"
				}
				b.WriteString(`<div style="padding:10px 0;border-bottom:1px solid #f0f0f0">`)
				b.WriteString(fmt.Sprintf(`<div style="display:flex;justify-content:space-between;align-items:start">
					<div style="flex:1;min-width:0"><p style="font-size:14px;margin:0 0 4px">%s</p>
					<p style="font-size:12px;color:#888;margin:0">%s · max %s/trade · %s/week</p></div>
					<span style="font-size:12px;color:%s;white-space:nowrap">%s</span></div>`,
					htmlEsc(s.Description), s.Mode, s.MaxPerTrade, s.MaxPerWeek, statusColor, statusLabel))
				b.WriteString(fmt.Sprintf(`<div style="display:flex;gap:6px;margin-top:6px">
					<form method="POST" action="/trade/strategy/toggle"><input type="hidden" name="id" value="%s"><button type="submit" style="font-size:12px;padding:3px 8px;border:1px solid #ddd;border-radius:4px;background:#fff;cursor:pointer">%s</button></form>
					<form method="POST" action="/trade/strategy/delete"><input type="hidden" name="id" value="%s"><button type="submit" style="font-size:12px;padding:3px 8px;border:1px solid #ddd;border-radius:4px;background:#fff;cursor:pointer;color:#c00">Delete</button></form>
					</div>`, s.ID, func() string { if s.Active { return "Pause" }; return "Resume" }(), s.ID))
				b.WriteString(`</div>`)
			}
		}
		b.WriteString(`<details style="margin-top:12px"><summary style="font-size:13px;cursor:pointer">New strategy</summary>`)
		b.WriteString(`<div style="margin-top:10px">`)
		for _, preset := range strategyPresets {
			b.WriteString(fmt.Sprintf(`<form method="POST" action="/trade/strategy" style="display:inline">
				<input type="hidden" name="description" value="%s">
				<input type="hidden" name="mode" value="alert">
				<input type="hidden" name="max_per_trade" value="50">
				<input type="hidden" name="max_per_week" value="200">
				<button type="submit" style="display:block;width:100%%;text-align:left;padding:10px 12px;margin-bottom:6px;border:1px solid #e0e0e0;border-radius:8px;background:#fff;cursor:pointer;font-size:13px;font-family:inherit">
				<strong>%s</strong><br><span style="color:#888;font-size:12px">%s</span>
				</button></form>`, htmlEsc(preset.Description), htmlEsc(preset.Name), htmlEsc(preset.Description)))
		}
		b.WriteString(`</div>`)
		b.WriteString(`<details style="margin-top:8px"><summary style="font-size:12px;color:#888;cursor:pointer">Custom strategy</summary>`)
		b.WriteString(`<form method="POST" action="/trade/strategy" style="margin-top:8px">`)
		b.WriteString(`<textarea name="description" placeholder="Describe your strategy in plain English..." required style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:13px;box-sizing:border-box;font-family:inherit;resize:vertical" rows="2"></textarea>`)
		b.WriteString(`<div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap">`)
		b.WriteString(`<div><label style="font-size:12px;color:#888;display:block;margin-bottom:2px">Mode</label><select name="mode" style="padding:6px;border:1px solid #ddd;border-radius:4px;font-size:13px"><option value="alert">Alert only</option><option value="confirm">Confirm first</option><option value="auto">Auto-execute</option></select></div>`)
		b.WriteString(`<div><label style="font-size:12px;color:#888;display:block;margin-bottom:2px">Max/trade</label><input type="text" name="max_per_trade" value="50" style="width:70px;padding:6px;border:1px solid #ddd;border-radius:4px;font-size:13px"></div>`)
		b.WriteString(`<div><label style="font-size:12px;color:#888;display:block;margin-bottom:2px">Max/week</label><input type="text" name="max_per_week" value="200" style="width:70px;padding:6px;border:1px solid #ddd;border-radius:4px;font-size:13px"></div>`)
		b.WriteString(`</div>`)
		b.WriteString(`<button type="submit" class="btn" style="margin-top:8px">Create</button>`)
		b.WriteString(`</form></details>`)
		b.WriteString(`</details>`)
		b.WriteString(`</div>`)

		// Signals
		sigs := GetSignals(acc.ID, 10)
		if len(sigs) > 0 {
			b.WriteString(`<div class="card">`)
			b.WriteString(`<h3>Signals</h3>`)
			for i := len(sigs) - 1; i >= 0; i-- {
				sig := sigs[i]
				icon := "→"
				if sig.Executed {
					icon = "✓"
				}
				date := sig.CreatedAt
				if len(date) > 16 {
					date = date[:16]
				}
				b.WriteString(fmt.Sprintf(`<div style="padding:8px 0;border-bottom:1px solid #f0f0f0;font-size:13px">
					<div><strong>%s %s %s %s</strong></div>
					<div style="color:#555;margin-top:2px">%s</div>
					<div style="color:#888;font-size:12px;margin-top:2px">%s</div>
					</div>`, icon, sig.Action, sig.Amount, sig.Token, htmlEsc(sig.Reason), date))
			}
			b.WriteString(`</div>`)
		}

		// Trade history
		trades := GetTrades(acc.ID, 20)
		if len(trades) > 0 {
			b.WriteString(`<div class="card">`)
			b.WriteString(`<h3>History</h3>`)
			for i := len(trades) - 1; i >= 0; i-- {
				t := trades[i]
				date := t.CreatedAt
				if len(date) > 10 {
					date = date[:10]
				}
				statusColor := "#888"
				if t.Status == "confirmed" {
					statusColor = "#28a745"
				} else if t.Status == "failed" {
					statusColor = "#c00"
				}
				b.WriteString(fmt.Sprintf(`<div style="display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid #f0f0f0;font-size:14px">`))
				b.WriteString(fmt.Sprintf(`<div><strong>%s → %s</strong><br><span style="font-size:12px;color:#888">%s</span></div>`, t.AmountIn, t.AmountOut, date))
				if t.TxHash != "" {
					b.WriteString(fmt.Sprintf(`<a href="`+ChainExplorer()+`/tx/%s" target="_blank" style="color:%s;font-size:13px">%s</a>`, t.TxHash, statusColor, t.Status))
				} else {
					b.WriteString(fmt.Sprintf(`<span style="color:%s;font-size:13px">%s</span>`, statusColor, t.Status))
				}
				b.WriteString(`</div>`)
			}
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

func handleCreateStrategy(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}

	desc := strings.TrimSpace(r.FormValue("description"))
	mode := ExecutionMode(r.FormValue("mode"))
	maxPerTrade := r.FormValue("max_per_trade")
	maxPerWeek := r.FormValue("max_per_week")

	if _, err := CreateStrategy(sess.Account, desc, mode, maxPerTrade, maxPerWeek); err != nil {
		http.Redirect(w, r, "/trade?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/trade", http.StatusSeeOther)
}

func handleToggleStrategy(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}
	PauseStrategy(sess.Account, r.FormValue("id"))
	http.Redirect(w, r, "/trade", http.StatusSeeOther)
}

func handleDeleteStrategy(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		app.RedirectToLogin(w, r)
		return
	}
	DeleteStrategy(sess.Account, r.FormValue("id"))
	http.Redirect(w, r, "/trade", http.StatusSeeOther)
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
