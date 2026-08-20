package wallet

// Taking your key with you.
//
// This is the honest answer to a custodial wallet, and the only one that fully
// works. Every other safeguard here — refusing a save that loses keys, atomic
// writes, backups — reduces the chance that this instance loses your key. None
// of them removes it. A copy you hold does: if the server loses the key and you
// have it, you have lost nothing but the convenience.
//
// It exists because the alternative was already run as an experiment. A key
// store was destroyed, and the only reason the money in it was not gone forever
// was that a second file happened to have been left behind by an old migration.
// Nobody should be relying on that twice.
//
// Three things about how it works, and each is the reason for the next.
//
// It is a page action and never a tool. An agent that can read a private key is
// a prompt injection away from posting it somewhere, and the whole design here
// assumes the agent reads text that strangers wrote. There is no wallet_export;
// the Spec has no such endpoint, so none can be derived.
//
// It asks for the password even though you are signed in. A session cookie is
// enough to spend from the wallet under caps; it is not enough to take the key,
// which is the wallet and every cap on it at once. That is the same reason a
// bank asks again before a transfer, and it is the one control that a stolen
// session does not already hold.
//
// It is written down as it happens. An export is the single most serious thing
// an account can do here, and if it was not the account holder who did it they
// need to be able to find out.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// ExportHandler serves /wallet/export.
//
// GET renders the form, POST checks the password and shows the key once.
func ExportHandler(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil || sess == nil || acc == nil {
		app.RedirectToLogin(w, r)
		return
	}

	if r.Method != http.MethodPost {
		app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportForm(r, "")})
		return
	}

	if err := r.ParseForm(); err != nil {
		app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportForm(r, "That form could not be read. Try again.")})
		return
	}

	pw := r.Form.Get("password")
	if strings.TrimSpace(pw) == "" {
		app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportForm(r, "Enter your password.")})
		return
	}

	if err := auth.CheckSecret(acc.ID, pw); err != nil {
		// A wrong password and an account that has none are different problems
		// and want different answers. Somebody who signed in with Google typing
		// their Google password into this box would otherwise be told, over and
		// over, that it was wrong.
		msg := "That password is not right."
		if strings.Contains(err.Error(), "no password") {
			msg = "This account has no password — it signs in with Google or a passkey. " +
				"Set a password first, or ask an admin to export the key for you."
		}
		app.Log("wallet", "SECURITY: failed key export for %s: %v", acc.ID, err)
		app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportForm(r, msg)})
		return
	}

	bw, err := EnsureFor(acc.ID)
	if err != nil || bw == nil || bw.PrivateKey == "" {
		app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportForm(r, "Your wallet could not be opened, so there is no key to show.")})
		return
	}

	// Written down, because if this was not the account holder they need to be
	// able to find out that it happened.
	app.Log("wallet", "SECURITY: private key exported for %s (%s)", acc.ID, bw.Address)

	app.Respond(w, r, app.Response{Title: "Export key", Description: "Take a copy of your private key", HTML: exportedKey(bw)})
}

func exportForm(r *http.Request, errMsg string) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h2>Export your private key</h2>`)
	b.WriteString(`<p>This is the key to your wallet. Anyone who has it can spend everything at ` +
		`your address, on any chain, forever — there is no revoking it and no changing it.</p>`)
	b.WriteString(`<p class="text-sm text-muted">Why you might want it: the key is held on this ` +
		`instance. That is what makes the wallet work the moment you ask for it, and it means ` +
		`you are trusting this server to keep it. A copy you hold yourself is the only thing ` +
		`that makes losing it here survivable.</p>`)
	b.WriteString(`<p class="text-sm text-muted">Where to put it: a password manager, or an ` +
		`encrypted file you have a backup of. Not your email, not a chat message, not a ` +
		`screenshot.</p>`)
	if errMsg != "" {
		b.WriteString(`<p class="text-error">` + html.EscapeString(errMsg) + `</p>`)
	}
	b.WriteString(`<form method="POST" action="/wallet/export" autocomplete="off">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	b.WriteString(`<p><label class="text-sm">Confirm your password</label><br>`)
	b.WriteString(`<input type="password" name="password" required autocomplete="current-password" ` +
		`class="form-input w-320"></p>`)
	b.WriteString(`<button type="submit">Show my key</button> `)
	b.WriteString(`<a href="/wallet" class="text-sm text-muted ml-2">Cancel</a>`)
	b.WriteString(`</form></div>`)
	return b.String()
}

func exportedKey(bw *BaseWallet) string {
	// Not stored anywhere by this page, not put in a link, and not fetched by
	// script: it is in the HTML of one response to one POST, which is the
	// smallest number of places it can be in and still be readable.
	return fmt.Sprintf(`<div class="card">
  <h2>Your private key</h2>
  <p class="cw-net"><b>Shown once.</b> Reloading this page will not show it again —
  come back through the form. Anyone with this key owns the address below.</p>
  <p class="text-sm text-muted m-0 mb-1">Address (public, safe to share)</p>
  <div class="cw-mono">%s</div>
  <p class="text-sm text-muted mt-14 mb-1">Private key (secret)</p>
  <div class="cw-mono cw-secret">%s</div>
  <p class="text-sm text-muted mt-14">It imports into any EVM wallet —
  MetaMask, Rabby, Coinbase Wallet — as a raw private key on <b>%s</b>.</p>
  <p class="mt-14"><a href="/wallet">Back to your wallet</a></p>
</div>
<style>
.cw-mono{font-family:ui-monospace,Menlo,monospace;font-size:13px;word-break:break-all;background:#f5f5f5;padding:11px;border:1px solid #e2e2e2;border-radius:6px;color:#222}
.cw-secret{background:#fff8e6;border-color:#f0dfae}
.cw-net{font-size:13px;color:#8a5a00;background:#fff8e6;border:1px solid #f0dfae;border-radius:6px;padding:9px 11px;margin:0 0 14px}
</style>`,
		html.EscapeString(bw.Address), html.EscapeString(bw.PrivateKey), html.EscapeString(chainName()))
}
