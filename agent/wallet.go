package agent

import (
	"encoding/json"
	"net/http"

	"mu/internal/auth"
	"mu/wallet"
)

// WalletHandler returns the logged-in user's Base wallet (address + USDC
// balance), creating one on first use. Auth required. Backs the wallet panel on
// /agent, which shows a fund-me QR and the live balance.
func WalletHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "login required"})
		return
	}
	// POST toggles pay-with-wallet mode (crypto=on|off).
	if r.Method == http.MethodPost {
		wallet.SetPayWithWallet(acc.ID, r.FormValue("crypto") == "on")
	}
	bw, err := wallet.GetOrCreateWallet(acc.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	usdc, _ := wallet.USDCBalance(bw.Address)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"address":       bw.Address,
		"usdc":          usdc,
		"network":       "base",
		"payWithWallet": wallet.PayWithWallet(acc.ID),
	})
}

// renderWalletPanel renders the collapsible wallet card for the sessions rail.
// It lazy-loads the address, USDC balance and a fund-me QR from /agent/wallet.
func renderWalletPanel() string {
	return `<div class="wallet-panel">
  <button class="wallet-head" type="button" onclick="muWalletToggle()">
    <span>Wallet</span><span class="wallet-bal" id="wallet-usdc">–</span>
  </button>
  <div class="wallet-body" id="wallet-body" hidden>
    <p class="wallet-hint">Send <strong>USDC on Base</strong> to this address to fund your agent.</p>
    <button type="button" class="wallet-addr" id="wallet-addr" title="Tap to copy" onclick="muWalletCopy()">loading…</button>
    <div class="wallet-copied" id="wallet-copied" hidden>Copied to clipboard ✓</div>
    <div class="wallet-actions">
      <button type="button" onclick="muWalletCopy()">Copy address</button>
      <button type="button" onclick="muWalletLoad(true)">Refresh balance</button>
    </div>
    <details class="wallet-qrwrap"><summary>Show QR code</summary><div class="wallet-qr" id="wallet-qr"></div></details>
    <label class="wallet-toggle"><input type="checkbox" id="wallet-crypto" onchange="muWalletSetMode(this.checked)"> Pay for tools from this wallet (USDC) instead of credits</label>
  </div>
</div>
<style>
.wallet-panel{border:1px solid var(--card-border,#e8e8e8);border-radius:8px;margin-bottom:12px;overflow:hidden;background:var(--card-background,#fff)}
.wallet-head{width:100%;display:flex;justify-content:space-between;align-items:center;padding:10px 12px;background:none;border:0;cursor:pointer;font-size:14px;font-weight:600;color:inherit}
.wallet-head .wallet-bal{color:#1a7f37;font-variant-numeric:tabular-nums;font-weight:600}
.wallet-body{padding:12px;border-top:1px solid var(--card-border,#eee)}
.wallet-addr{display:block;width:100%;text-align:left;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;word-break:break-all;background:#f5f5f5;padding:10px;border:1px solid #e2e2e2;border-radius:6px;color:#222;cursor:pointer}
.wallet-addr:hover{background:#eef2ff;border-color:#c7d2fe}
.wallet-copied{font-size:11px;color:#1a7f37;margin-top:6px}
.wallet-actions{display:flex;gap:6px;margin-top:8px}
.wallet-actions button{flex:1;padding:7px 8px;font-size:12px;cursor:pointer;border:1px solid #ddd;border-radius:4px;background:#fafafa}
.wallet-hint{font-size:11px;color:#888;margin:0 0 8px}
.wallet-qrwrap{margin-top:10px;font-size:12px;color:#666}
.wallet-qrwrap summary{cursor:pointer}
.wallet-qr{display:flex;justify-content:center;margin-top:8px}
.wallet-qr img{width:170px;height:170px;image-rendering:pixelated}
.wallet-toggle{display:flex;gap:8px;align-items:flex-start;margin-top:12px;font-size:12px;color:#444;line-height:1.4;cursor:pointer}
.wallet-toggle input{margin-top:2px}
</style>
<script src="/qrcode.js"></script>
<script>
var muWalletAddr="";
function muWalletToggle(){var b=document.getElementById('wallet-body');if(!b)return;b.hidden=!b.hidden;if(!b.hidden&&!muWalletAddr)muWalletLoad(false);}
function muWalletCopied(){var c=document.getElementById('wallet-copied');if(!c)return;c.hidden=false;clearTimeout(window._muWCT);window._muWCT=setTimeout(function(){c.hidden=true;},1800);}
function muWalletCopy(){
  if(!muWalletAddr)return;
  if(navigator.clipboard&&navigator.clipboard.writeText){
    navigator.clipboard.writeText(muWalletAddr).then(muWalletCopied).catch(muWalletCopyFallback);
  } else { muWalletCopyFallback(); }
}
function muWalletCopyFallback(){
  var ta=document.createElement('textarea');ta.value=muWalletAddr;ta.style.position='fixed';ta.style.opacity='0';
  document.body.appendChild(ta);ta.focus();ta.select();
  try{document.execCommand('copy');muWalletCopied();}catch(e){}
  document.body.removeChild(ta);
}
function muWalletLoad(force){
  fetch('/agent/wallet',{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    if(d.error||!d.address)return;
    muWalletAddr=d.address;
    var a=document.getElementById('wallet-addr');if(a)a.textContent=d.address;
    var u=document.getElementById('wallet-usdc');if(u)u.textContent='$'+(d.usdc||'0');
    var q=document.getElementById('wallet-qr');
    if(q&&window.qrcode){try{var qr=qrcode(0,'M');qr.addData(d.address);qr.make();q.innerHTML=qr.createImgTag(4,8);}catch(e){}}
    var c=document.getElementById('wallet-crypto');if(c)c.checked=!!d.payWithWallet;
  }).catch(function(){});
}
function muWalletCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function muWalletSetMode(on){
  var b=new URLSearchParams();b.append('crypto',on?'on':'off');
  fetch('/agent/wallet',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muWalletCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(d){var c=document.getElementById('wallet-crypto');if(c)c.checked=!!d.payWithWallet;}).catch(function(){});
}
// Prime the balance in the header on load.
muWalletLoad(false);
</script>`
}
