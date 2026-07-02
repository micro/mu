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
	bw, err := wallet.GetOrCreateWallet(acc.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	usdc, _ := wallet.USDCBalance(bw.Address)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"address": bw.Address,
		"usdc":    usdc,
		"network": "base",
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
    <div class="wallet-qr" id="wallet-qr"></div>
    <div class="wallet-addr"><code id="wallet-addr" title="Your Base address">loading…</code></div>
    <div class="wallet-actions">
      <button type="button" onclick="muWalletCopy()">Copy address</button>
      <button type="button" onclick="muWalletLoad(true)">Refresh</button>
    </div>
    <p class="wallet-hint">Send <strong>USDC on Base</strong> to this address to fund your agent. Scan the QR or copy it.</p>
  </div>
</div>
<style>
.wallet-panel{border:1px solid var(--card-border,#e8e8e8);border-radius:8px;margin-bottom:12px;overflow:hidden;background:var(--card-background,#fff)}
.wallet-head{width:100%;display:flex;justify-content:space-between;align-items:center;padding:10px 12px;background:none;border:0;cursor:pointer;font-size:14px;font-weight:600;color:inherit}
.wallet-head .wallet-bal{color:#1a7f37;font-variant-numeric:tabular-nums;font-weight:600}
.wallet-body{padding:12px;border-top:1px solid var(--card-border,#eee)}
.wallet-qr{display:flex;justify-content:center;margin-bottom:10px}
.wallet-qr img{width:150px;height:150px;image-rendering:pixelated}
.wallet-addr code{display:block;font-size:11px;word-break:break-all;background:#f5f5f5;padding:6px 8px;border-radius:4px;color:#333}
.wallet-actions{display:flex;gap:6px;margin-top:8px}
.wallet-actions button{flex:1;padding:6px 8px;font-size:12px;cursor:pointer;border:1px solid #ddd;border-radius:4px;background:#fafafa}
.wallet-hint{font-size:11px;color:#888;margin:8px 0 0}
</style>
<script src="/qrcode.js"></script>
<script>
var muWalletAddr="";
function muWalletToggle(){var b=document.getElementById('wallet-body');if(!b)return;b.hidden=!b.hidden;if(!b.hidden&&!muWalletAddr)muWalletLoad(false);}
function muWalletCopy(){if(muWalletAddr)navigator.clipboard&&navigator.clipboard.writeText(muWalletAddr);}
function muWalletLoad(force){
  fetch('/agent/wallet',{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    if(d.error||!d.address)return;
    muWalletAddr=d.address;
    var a=document.getElementById('wallet-addr');if(a)a.textContent=d.address;
    var u=document.getElementById('wallet-usdc');if(u)u.textContent='$'+(d.usdc||'0');
    var q=document.getElementById('wallet-qr');
    if(q&&window.qrcode){try{var qr=qrcode(0,'M');qr.addData(d.address);qr.make();q.innerHTML=qr.createImgTag(4,8);}catch(e){}}
  }).catch(function(){});
}
// Prime the balance in the header on load.
muWalletLoad(false);
</script>`
}
