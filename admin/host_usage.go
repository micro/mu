package admin

import (
	"fmt"
	"html"
	"math/big"
	"sort"
	"strings"
	"time"

	"mu/internal/usage"
)

// This is a bounded operational report, not a billing ledger. Missing historical
// host attribution must never be guessed from the current deployment settings.
func hostUsageCard(entries []usage.Activity, since time.Time, caller string) string {
	type key struct{ host, operation, method, asset, network string }
	type row struct {
		calls, successful int
		callers           map[string]int
		amount            *big.Int
	}
	rows := map[key]*row{}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.At.Before(since) || e.Host == "" || (caller != "" && e.Account != caller) {
			continue
		}
		method := e.PaymentMethod
		switch e.Surface {
		case "http":
			if e.Path != "/mcp" && !strings.HasPrefix(e.Path, "/api/v1/") {
				continue
			}
			method = "HTTP requests"
		case "mcp", "api":
			if method == "" {
				method = "unknown"
			}
			method += " tool calls"
		case "payment":
			if e.PaymentMethod != "x402" || e.Outcome != "ok" {
				continue
			}
			if e.Transaction != "" {
				id := e.Network + ":" + e.Transaction
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			method = "x402 settlements"
		default:
			continue
		}
		k := key{e.Host, e.Operation, method, e.Asset, e.Network}
		r := rows[k]
		if r == nil {
			r = &row{callers: map[string]int{}, amount: new(big.Int)}
			rows[k] = r
		}
		r.calls++
		if (e.Surface == "http" && e.Status >= 200 && e.Status < 300) || (e.Surface != "http" && e.Outcome == "ok") {
			r.successful++
		}
		if e.Account != "" && e.Account != "guest" {
			r.callers[e.Account]++
		}
		if e.Surface == "payment" {
			if n, ok := new(big.Int).SetString(e.AmountAtomic, 10); ok && n.Sign() >= 0 {
				r.amount.Add(r.amount, n)
			}
		}
	}
	keys := make([]key, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return a.host+a.method+a.operation+a.network+a.asset < b.host+b.method+b.operation+b.network+b.asset
	})
	var b strings.Builder
	b.WriteString(`<section class="record-card"><h2>Host and payment activity</h2><p class="text-muted">Recorded requests, tool executions and settled x402 payments are separate stages; do not add their counts together. Account-authenticated calls may be free or cached, so they are not proof of a credit charge. Repeat callers have two or more records in the same row; anonymous callers are excluded.</p><p class="text-muted">Selected time window, limited to the latest 5,000 activity records retained for up to 14 days. Collection starts with this update. Amounts are exact asset atomic units, grouped by asset and network; this is not a revenue ledger.</p><div class="table-scroll" tabindex="0" role="region" aria-label="Host and payment activity"><table class="data-table"><thead><tr><th>Host</th><th>Operation</th><th>Type</th><th>Count</th><th>Successful</th><th>Callers</th><th>Repeat callers</th><th>Settled amount</th></tr></thead><tbody>`)
	for _, k := range keys {
		r := rows[k]
		repeat := 0
		for _, n := range r.callers {
			if n > 1 {
				repeat++
			}
		}
		amount := "—"
		if k.method == "x402 settlements" {
			amount = r.amount.String() + " atomic units · " + k.network + " · " + k.asset
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td></tr>`, html.EscapeString(k.host), html.EscapeString(k.operation), html.EscapeString(k.method), r.calls, r.successful, len(r.callers), repeat, html.EscapeString(amount))
	}
	if len(keys) == 0 {
		b.WriteString(`<tr><td colspan="8">No attributed activity recorded in this window.</td></tr>`)
	}
	b.WriteString(`</tbody></table></div></section>`)
	return b.String()
}
