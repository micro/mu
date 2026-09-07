package agent

import "mu/internal/app"

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
 var saved=sessionStorage.getItem(key);
 if(location.pathname===base&&!location.search&&saved&&/^\/agent\/micro\?(session=[A-Za-z0-9_-]+|new=1)$/.test(saved)){
  location.replace(saved);return;
 }
 function remember(id){sessionStorage.setItem(key,base+(id?'?session='+encodeURIComponent(id):'?new=1'));}
 var query=new URLSearchParams(location.search);
 if(!['bookmark','saved','item','prompt','q'].some(function(k){return query.has(k);}))remember(` + app.JSString(contextID) + `);
 window.addEventListener('mu-chat-thread',function(e){remember(e.detail);});
 window.addEventListener('mu-chat-new',function(){remember('');});
}catch(e){}
})();</script>`
}
