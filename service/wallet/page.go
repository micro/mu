package wallet

// The page: your address, what it holds, and the warning that matters.
//
// Most of this was the crypto card on the old /wallet page, where it was framed
// as a way to buy credits. That framing is gone — credits are bought with a
// card and live on the account — but the address, the balance and the network
// warning are not decoration for it. They are what a wallet page is.
//
// The warning is here because it has already cost real money. A bare EVM
// address names no chain, so a wallet scanning one picks whatever it is
// currently on, which is Ethereum mainnet for almost everybody; USDC then
// arrives at the same address on a chain this instance cannot reach. The QR
// carries an EIP-681 URI naming the token and the chain, so there is nothing
// left to choose, and the sentence above it says what happens if you get it
// wrong anyway.

import (
	"fmt"
	"html"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/x402"
)

// Handler serves /wallet.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		body := `<div class="card">` +
			`<p>A key of your own on Base: an address that holds USDC, and an agent that can ` +
			`spend it on priced endpoints anywhere — no account with those servers, no card ` +
			`on file, no key to rotate.</p>` +
			`<p class="text-sm text-muted">Credits for this instance are separate and are bought ` +
			`with a card — see <a href="/account">your account</a>.</p>` +
			`<p><a href="/login" class="btn">Sign in</a> <a href="/signup" class="btn btn-secondary">Sign up</a></p></div>` +
			toolsCard()
		app.Respond(w, r, app.Response{Title: "Wallet", Description: "A key of your own on Base", HTML: body})
		return
	}
	app.Respond(w, r, app.Response{Title: "Wallet", Description: "A key of your own on Base", HTML: Page(sess.Account)})
}

// Page renders the signed-in wallet.
func Page(accountID string) string {
	bw, err := EnsureFor(accountID)
	// Say so rather than vanishing. A card that renders with an empty address
	// and a QR code of nothing is worse than no card: it looks like the feature
	// half-works, and there is nothing to go on.
	if err != nil || bw == nil || bw.Address == "" {
		return `<div class="card">` +
			`<p class="text-sm text-muted">Your wallet could not be opened, so there is no ` +
			`address to show. The server log under <code>wallet</code> says why.</p></div>`
	}

	human, _, balErr := USDCBalanceErr(bw.Address)
	unreadable := ""
	if balErr != nil {
		// Not the same as zero, and the difference is the whole message to
		// somebody who has just sent money: one says wait, the other says go
		// and look at the chain yourself.
		human = "—"
		unreadable = ` <span class="text-gold text-sm">· could not reach ` +
			html.EscapeString(chainName()) + ` to read this, so it may not be zero</span>`
		app.Log("wallet", "could not read the USDC balance of %s: %v", bw.Address, balErr)
	}

	chainID, ok := chainIDFor(x402.Network())
	if !ok {
		chainID = 8453
	}
	payURI := fmt.Sprintf("ethereum:%s@%d/transfer?address=%s", baseUSDC, chainID, bw.Address)
	net := html.EscapeString(chainName())

	// No heading: app.Respond already titles the page "Wallet", and the card
	// repeating it printed the word twice down the left of the screen.
	return fmt.Sprintf(`<div class="card">
  <p class="text-sm text-muted">A key of your own. Your agent can spend it on priced
  endpoints anywhere, capped per call and per day.</p>
  <p class="text-28 mt-2 mb-3"><b>$%s</b> <span class="text-muted text-base">USDC</span>%s</p>
  <p class="cw-net"><b>%s only.</b> USDC sent on Ethereum, Arbitrum or any other
  chain lands at this same address on that chain, where this instance cannot see it
  or move it.</p>
  <button type="button" class="cw-addr" data-addr="%s" onclick="cwCopy(this)">%s</button>
  <div class="cw-copied" id="cw-copied" hidden>Copied to clipboard ✓</div>
  <details class="cw-qrwrap"><summary>Show QR code</summary>
    <div class="cw-qr" id="cw-qr" data-uri="%s"></div>
    <p class="cw-qrnote">Scans as <b>USDC on %s</b> — your wallet should already
    have the network and token filled in. If it offers a different network, stop.</p>
  </details>
  <p class="text-sm text-muted mt-3 m-0"><a href="/wallet/export">Export your
  private key →</a> The key is held on this instance; a copy you hold yourself is the only
  thing that makes losing it here survivable.</p>
  <p class="text-sm text-muted mt-half m-0">Credits for this instance are a
  separate thing and are bought with a card — <a href="/billing#balance">your balance</a>.</p>
</div>
%s
<style>
.cw-addr{display:block;width:100%%;text-align:left;font-family:ui-monospace,Menlo,monospace;font-size:13px;word-break:break-all;background:#f5f5f5;padding:11px;border:1px solid #e2e2e2;border-radius:6px;color:#222;cursor:pointer}
.cw-addr:hover{background:var(--hover-background,#f5f5f5);border-color:var(--border-color,#ddd)}
.cw-copied{font-size:12px;color:#1a7f37;margin-top:6px}
.cw-net{font-size:13px;color:#8a5a00;background:#fff8e6;border:1px solid #f0dfae;border-radius:6px;padding:9px 11px;margin:0 0 10px}
.cw-qrnote{font-size:12px;color:#666;margin:8px 0 0;max-width:260px}
.cw-qrwrap{margin-top:10px;font-size:13px;color:#666}
.cw-qrwrap summary{cursor:pointer}
.cw-qr{margin-top:8px}.cw-qr img{width:180px;height:180px;image-rendering:pixelated}
</style>
<script src="/qrcode.js"></script>
<script>
(function(){var q=document.getElementById('cw-qr');if(!q||!window.qrcode)return;
var addr=document.querySelector('.cw-addr');
// The payment URI, which names the chain. Only ever the bare address as a last
// resort, because that is the thing that sent somebody's money to the wrong network.
var data=q.getAttribute('data-uri')||(addr&&addr.getAttribute('data-addr'));
if(!data)return;
try{var qr=qrcode(0,'M');qr.addData(data);qr.make();q.innerHTML=qr.createImgTag(4,8);}catch(e){}})();
function cwCopy(el){var a=el.getAttribute('data-addr');function done(){var c=document.getElementById('cw-copied');if(c){c.hidden=false;setTimeout(function(){c.hidden=true;},1800);}}
  if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(a).then(done).catch(function(){cwFallback(a,done);});}else{cwFallback(a,done);}}
function cwFallback(a,done){var t=document.createElement('textarea');t.value=a;t.style.position='fixed';t.style.opacity='0';document.body.appendChild(t);t.select();try{document.execCommand('copy');done();}catch(e){}document.body.removeChild(t);}
</script>`,
		html.EscapeString(human), unreadable, net,
		html.EscapeString(bw.Address), html.EscapeString(bw.Address),
		html.EscapeString(payURI), net, toolsCard())
}

// toolsCard says what an agent holding this wallet can do with it, because that
// is the reason for the service and it is not visible from an address.
func toolsCard() string {
	return `<div class="card"><h3>What an agent can do with it</h3>
<p class="text-sm text-muted"><code>wallet_address</code> — where to send funds.
<code>wallet_balance</code> — what it holds.
<code>wallet_list</code> — which priced servers it may pay.
<code>wallet_pay</code> — call a tool on one of them and pay for it.</p>
<p class="text-sm text-muted">Payments are capped per call and per day, so a server
cannot name any price it likes to an agent that has read something misleading.
The key is held on this instance, which is what makes it work out of the box and
also means an operator who can read the disk can spend it — hold what you are
willing to have on a server.</p></div>`
}
