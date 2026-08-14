package home

// What you have used, and what it cost.
//
// The instance had counters from the day it started charging, but only the
// operator could read them — /admin/traffic is everyone's traffic behind an
// admin check. The person paying had no way to answer "what has my agent been
// doing", which is the question anyone asks on their second day.
//
// Two halves, from two sources, because neither answers the whole thing:
//
//   - Calls come from the counters. They include everything, free and paid, and
//     they are a shape over time rather than a list.
//   - Spend comes from the wallet's ledger, which records the operation and the
//     amount for anything that cost money.
//
// What is deliberately absent is "which free tools did you call". The counters
// keep names and accounts as separate dimensions on purpose — pairing them is
// the cardinality explosion internal/usage refuses to grow — so the page does
// not guess at an answer it does not have.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

// UsageHandler serves /usage: one caller's own activity.
func UsageHandler(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	account := sess.Account
	win := usage.WindowFor(r.URL.Query().Get("window"))

	var sb strings.Builder
	sb.WriteString(usage.CSS + usagePageCSS)

	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	usage.Stat(&sb, "Last hour", usage.TotalForOver(account, usage.Minute, 60))
	usage.Stat(&sb, "Last 24 hours", usage.TotalForOver(account, usage.Hour, 24))
	usage.Stat(&sb, "Last 7 days", usage.TotalForOver(account, usage.Hour, 24*7))
	usage.Stat(&sb, "Last 90 days", usage.TotalForOver(account, usage.Day, 90))
	sb.WriteString(`</div></div>`)

	sb.WriteString(`<div class="card">`)
	sb.WriteString(usage.Tabs("/usage", win))
	sb.WriteString(usage.ChartSVG(usage.SeriesFor(account, win.Res, win.Points), win))
	sb.WriteString(`</div>`)

	sb.WriteString(spendSection(account, acc.Admin))
	sb.WriteString(`<p class="text-sm text-muted">Your calls only. Counts are kept for ` +
		`2 hours by the minute, 7 days by the hour and 90 days by the day — nothing about ` +
		`a request itself is stored. <a href="/tools">Connect an agent →</a></p>`)

	w.Write([]byte(app.RenderHTMLForRequest("Usage", "What you have used, and what it cost", sb.String(), r)))
}

// spendSection breaks the ledger down by operation, so "what is costing me" has
// an answer with a number next to it rather than a scroll of transactions.
func spendSection(id string, admin bool) string {
	var sb strings.Builder

	// 500 is enough to cover a heavy month and short enough to stay quick; the
	// ledger on /account is the full record.
	txs := account.Transactions(id, 500)

	spentBy := map[string]int{}
	spent, topped := 0, 0
	for _, tx := range txs {
		switch {
		case tx.Amount < 0:
			spentBy[tx.Operation] += -tx.Amount
			spent += -tx.Amount
		case tx.Amount > 0:
			topped += tx.Amount
		}
	}

	rows := make([]usage.Count, 0, len(spentBy))
	for k, v := range spentBy {
		rows = append(rows, usage.Count{Key: k, Count: v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Key < rows[j].Key
	})

	sb.WriteString(`<div class="card"><div class="traffic-stats">`)
	usage.Stat(&sb, "Credits now", account.Balance(id))
	usage.Stat(&sb, "Credits spent", spent)
	usage.Stat(&sb, "Credits added", topped)
	sb.WriteString(`</div>`)
	if admin {
		sb.WriteString(`<p class="text-sm text-muted">You are an admin on this instance, ` +
			`so your calls are never charged.</p>`)
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="traffic-grid">`)
	usage.Table(&sb, "What you spent on", rows)

	// The last few movements, so a surprising number has something to check it
	// against without leaving the page.
	sb.WriteString(`<div class="card"><h3>Recent</h3>`)
	if len(txs) == 0 {
		sb.WriteString(`<p class="text-sm text-muted">Nothing yet. ` +
			`A credit is charged when a call costs this instance something to run — ` +
			`a model call, or a paid third party. Reading and listing are free.</p>`)
	} else {
		sb.WriteString(`<table class="traffic-table usage-recent">`)
		for i, tx := range txs {
			if i >= 12 {
				break
			}
			amount := fmt.Sprintf("%+d", tx.Amount)
			class := "usage-spend"
			if tx.Amount > 0 {
				class = "usage-topup"
			}
			fmt.Fprintf(&sb, `<tr><td>%s</td><td class="usage-when">%s</td><td class="traffic-n %s">%s</td></tr>`,
				html.EscapeString(tx.Operation), html.EscapeString(usage.Since(tx.CreatedAt)), class, amount)
		}
		sb.WriteString(`</table><p class="text-sm text-muted"><a href="/account">Full ledger →</a></p>`)
	}
	sb.WriteString(`</div></div>`)
	return sb.String()
}

const usagePageCSS = `<style>
.usage-recent td{vertical-align:baseline}
.usage-when{color:var(--text-muted);font-size:12px;white-space:nowrap;padding-left:10px!important}
.usage-spend{color:var(--text-primary)}
.usage-topup{color:#1a7f37}
</style>`
