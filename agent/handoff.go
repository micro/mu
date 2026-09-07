package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

var handoffMu sync.Mutex

// HandoffHandler adopts completed guest turns as account-owned conversation
// history. It never executes their contents or imports browser-rendered HTML.
func HandoffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.RespondError(w, 401, "Sign in to keep this conversation")
		return
	}
	if r.Header.Get("X-CSRF-Token") == "" || !auth.ValidCSRF(r) {
		app.RespondError(w, 403, "Invalid CSRF token")
		return
	}
	var req struct {
		Turns []struct {
			Prompt string `json:"prompt"`
			Answer string `json:"answer"`
		} `json:"turns"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Turns) == 0 || len(req.Turns) > 100 {
		app.RespondError(w, 400, "Invalid conversation")
		return
	}
	for _, t := range req.Turns {
		if strings.TrimSpace(t.Prompt) == "" || strings.TrimSpace(t.Answer) == "" {
			app.RespondError(w, 400, "Only completed turns can be saved")
			return
		}
	}
	// Stable within the account, so retrying a interrupted handoff does not
	// create another copy. No guest run/context ID is trusted as a private ID.
	raw, _ := json.Marshal(req.Turns)
	sum := sha256.Sum256(raw)
	handoffMu.Lock()
	defer handoffMu.Unlock()
	id := Opened(acc.ID, thread.WebClient, "guest:"+hex.EncodeToString(sum[:]), "", "")
	if len(thread.Messages(acc.ID, id, 1)) == 0 {
		for _, t := range req.Turns {
			Said(acc.ID, id, t.Prompt, "", "")
			Answered(acc.ID, id, t.Answer, "")
		}
	}
	app.RespondJSON(w, map[string]string{"id": id})
}

// HandoffHTML carries a guest conversation through any login method, including
// providers that return to Home. Remove browser copies only after saving succeeds.
func HandoffHTML(r *http.Request) string {
	_, acc := auth.TrySession(r)
	if acc == nil {
		return ""
	}
	return `<script>(function(){
var turns,draft,raw;
try{
 raw=sessionStorage.getItem('mu_chat_hist:landing');
 turns=JSON.parse(raw||'[]');
 draft=sessionStorage.getItem('mu_chat_draft:landing')||'';
 var markup=sessionStorage.getItem('mu_chat_conv:landing');
 if(markup){var t=document.createElement('template');t.innerHTML=markup;
 if(t.content.querySelector('.mu-think')){var users=t.content.querySelectorAll('.mu-user');if(users.length&&!draft)draft=users[users.length-1].textContent;}}
}catch(e){return;}
if(!turns.length&&!draft)return;
var ns=` + app.JSString("agent-"+acc.ID+"-") + `;
function finish(id){
 try{
 if(draft)sessionStorage.setItem('mu_chat_draft:'+ns+':'+id,draft);
 ['hist','conv','ctx','draft'].forEach(function(k){sessionStorage.removeItem('mu_chat_'+k+':landing');});
 }catch(e){return;}
 // Keep the person on Home; the imported conversation is in chat history.
}
if(!turns.length){finish('');return;}
fetch('/agent/handoff',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':` + app.JSString(auth.CSRFToken(r)) + `},body:JSON.stringify({turns:turns})})
.then(function(r){if(!r.ok)throw Error();return r.json();}).then(function(r){finish(r.id);})
.catch(function(){var note=document.createElement('p');note.textContent='Your earlier conversation is still saved in this tab. Reload to try bringing it into your account again.';document.getElementById('content').prepend(note);});
})();</script>`
}
