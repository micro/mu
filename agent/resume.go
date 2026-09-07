package agent

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// Remember only the default assistant's destination, scoped to this account/tab.
// The server still checks ownership when reopening a thread.
func resumeMicroJS(accountID, agentID, contextID string) string {
	if agentID != "" {
		return ""
	}
	return `<script>(function(){
var key='mu_micro_resume:'+` + app.JSString(accountID) + `;
var base='/agent/micro';
try{
 if(sessionStorage.getItem('mu_chat_hist:landing')||sessionStorage.getItem('mu_chat_draft:landing'))return;
 function remember(id){sessionStorage.setItem(key,base+(id?'?session='+encodeURIComponent(id):'?new=1'));}
 var query=new URLSearchParams(location.search);
 if(!['bookmark','saved','item','prompt','q'].some(function(k){return query.has(k);}))remember(` + app.JSString(contextID) + `);
 window.addEventListener('mu-chat-thread',function(e){remember(e.detail);});
 window.addEventListener('mu-chat-new',function(){remember('');});
}catch(e){}
})();</script>`
}

// MicroHandler renders the signed-in assistant at the front door.
func MicroHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	servePage(w, r)
}

func chatPath(owner, id string) string {
	if id == "" {
		return "/agent/micro"
	}
	return Path(owner, id)
}
