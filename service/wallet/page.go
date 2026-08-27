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

	"mu/internal/app"
	"mu/internal/x402"
)

// SignedOut is the card for somebody who is not signed in.
//
// Exported because account/ draws this page. The route belongs to whoever
// composes it, and what is composed is the ledger plus this — so the ledger's
// package owns it. A hook pointing the other way (wallet.Money, filled by the
// server) was tried first and TestTheRulesAlreadyEnforcedAreNotWalkedAround
// refused it: a fourth service reaching up into the account, which is the
// import TestNoServiceImportsTheAccount forbids, wearing a function variable.
func SignedOut() string {
	return `<div class="card">` +
		`<p>What you have here, and a key of your own on Base: an address that holds ` +
		`USDC, and an agent that can spend it on priced endpoints anywhere — no account ` +
		`with those servers, no card on file, no key to rotate.</p>` +
		`<p><a href="/login" class="btn">Sign in</a> <a href="/signup" class="btn btn-secondary">Sign up</a></p></div>`
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

	// Headed again, and it has to be. This card had none, because it was the
	// only one on a page app.Respond already titles "Wallet" and repeating the
	// word printed it twice down the left of the screen. It is now the fourth
	// card on that page and the second number on it that is a balance — the
	// first is credits, which the instance owes you, and this is USDC on a
	// chain, which it does not. Unlabelled, the two read as one figure
	// disagreeing with itself.
	//
	// "Crypto" rather than a more precise name for what it is. The precise
	// names — on-chain key, secp256k1 keypair, Base address — all describe the
	// mechanism, and the reader deciding whether this card is for them is
	// answering a different question: is this the crypto bit. It is.
	return fmt.Sprintf(`<div class="card">
  <h4>Crypto</h4>
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
%s
  <p class="text-sm text-muted mt-3 m-0"><a href="/wallet/export">Export your
  private key →</a> The key is held on this instance; a copy you hold yourself is the only
  thing that makes losing it here survivable.</p>
</div>
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
		html.EscapeString(payURI), net, convertForm())
}

// convertForm turns what the wallet holds into credits on this instance.
//
// The missing half of the card. It offered an address to send USDC to and
// nothing that could spend what arrived — every path out was outbound, the CLI
// paying somebody else's priced endpoint — so money sent here bought nothing
// here.
//
// A hundred credits to the dollar and no rate quoted, because there is not one:
// a credit is a cent and USDC is dollars. That is what the switch off pence was
// for, and it is why this form can say the number and stop.
//
// Absent when the instance takes no USDC. A form that can only fail is worse
// than no form — it reads as broken rather than as unconfigured.
func convertForm() string {
	if !x402.Enabled() {
		return ""
	}
	return `<form class="cw-convert" method="POST" action="/wallet/convert">
  <label for="cw-amount">Turn into credits</label>
  <div class="cw-convert-row">
    <span class="cw-convert-unit">$</span>
    <input id="cw-amount" class="field" type="number" name="amount" min="1" step="1" placeholder="5" required>
    <button class="btn" type="submit">Convert</button>
  </div>
  <p class="cw-convert-note">Moves USDC from this address to the instance and adds it to your
  balance. $1 is 100 credits.</p>
</form>`
}
