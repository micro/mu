package apps

// An app runs with the capabilities it was given, not with your account.
//
// A saved app used to be served as a full page on this origin with the
// viewer's session attached, and its SDK said so out loud:
//
//	// Raw fetch helpers (for any Mu endpoint, same origin)
//	get:function(p){return get(p)},
//	post:function(p,b){return post(p,b)},
//
// Apps are public, searchable and forkable, and opening one is a click. So
// opening somebody's app ran their JavaScript as you: mu.post('/account/
// transfer', …) moved your credits, mu.get('/mail') read your mail. CSRF was no
// obstacle — the token is in a cookie JavaScript can read, which is how the
// product's own pages get it — and neither was the /sdk/ proxy, because
// nothing had to go through it.
//
// The ad-hoc code path (apps_run) never had this problem. It has always been
// served under `Content-Security-Policy: sandbox allow-scripts` with no
// allow-same-origin, so the code runs in an opaque origin with no cookies. This
// gives saved apps the same treatment, and puts back the capabilities they
// legitimately need through a bridge:
//
//	iframe (opaque origin, no cookies)  ──postMessage──▶  parent (this origin)
//	     app code + window.mu shim                        performs the call
//
// The parent is ours, the app cannot reach into it, and the only things it will
// do are the operations the SDK documents. An app asking for anything else —
// /wallet/transfer, /account, an endpoint invented tomorrow — is refused,
// because the bridge dispatches from a fixed table rather than a path.
//
// Not WASM. WASM bounds CPU and memory for code you execute yourself; the
// problem here is ambient authority — cookies and an origin — which is a
// browser-boundary question, and the browser already has the boundary.

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// bridgeOp is one thing an app may ask the parent to do on its behalf.
//
// Method and Path are fixed here rather than supplied by the app, which is the
// whole point: the app names an operation, not a URL.
type bridgeOp struct {
	Method string // GET or POST
	Path   string // fixed path, or a prefix when Suffix is true
	Suffix bool   // path is completed by an app-supplied, sanitised segment
	Query  bool   // app-supplied query parameters are appended
}

// bridgeOps is the whole surface an app can reach. It is the SDK's own
// documented API and nothing besides.
//
// Adding a row grants every app on the instance that capability, for every
// viewer who opens one. Weigh it as that, not as a convenience.
var bridgeOps = map[string]bridgeOp{
	// Reading what this instance knows. All public pages in JSON.
	"weather":       {Method: "GET", Path: "/weather", Query: true},
	"news":          {Method: "GET", Path: "/news", Query: true},
	"markets":       {Method: "GET", Path: "/markets", Query: true},
	"video":         {Method: "GET", Path: "/video", Query: true},
	"social":        {Method: "GET", Path: "/social", Query: true},
	"search":        {Method: "GET", Path: "/web", Query: true},
	"blog.list":     {Method: "GET", Path: "/blog", Query: true},
	"blog.read":     {Method: "GET", Path: "/blog/post", Query: true},
	"apps.list":     {Method: "GET", Path: "/apps", Query: true},
	"apps.read":     {Method: "GET", Path: "/apps/", Suffix: true},
	"places.search": {Method: "POST", Path: "/places/search"},
	"places.nearby": {Method: "POST", Path: "/places/nearby"},

	// Writing, and spending. Each of these already charges or rate-limits the
	// viewer through the central write gate; the app cannot reach anything the
	// person could not do themselves on the page.
	"blog.create": {Method: "POST", Path: "/blog"},
	"chat":        {Method: "POST", Path: "/chat"},
	"agent":       {Method: "POST", Path: "/agent/run"},

	// Who is looking. The session endpoint returns the account, not a
	// credential — an app knowing whose data it is showing is the point.
	"user": {Method: "GET", Path: "/session"},
}

// sdkProxyOps are the operations already proxied server-side under
// /apps/<slug>/sdk/<op>, where the caller is bound from the session and the
// app is named in the path. They were never the hole; they pass through.
var sdkProxyOps = map[string]bool{
	"ai": true, "store": true, "db": true, "fetch": true,
	"service": true, "services": true,
}

// bridgeTable is the op table as JSON for the parent-side script.
func bridgeTable() string {
	type op struct {
		Method string `json:"m"`
		Path   string `json:"p"`
		Suffix bool   `json:"s,omitempty"`
		Query  bool   `json:"q,omitempty"`
	}
	out := map[string]op{}
	for name, o := range bridgeOps {
		out[name] = op{o.Method, o.Path, o.Suffix, o.Query}
	}
	b, _ := json.Marshal(out)
	proxy := make([]string, 0, len(sdkProxyOps))
	for name := range sdkProxyOps {
		proxy = append(proxy, name)
	}
	p, _ := json.Marshal(proxy)
	return "var OPS=" + string(b) + ";var PROXY=" + string(p) + ";"
}

// sandboxCSP is the header that makes the app's own document an opaque origin.
//
// The iframe carries sandbox="allow-scripts" too, but an app URL can be opened
// directly, and then the attribute does not apply — the header re-applies it on
// the response itself. Both, for the same reason apps_run has both.
const sandboxCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals; " +
	"default-src 'unsafe-inline' 'self' data: blob: https:; " +
	"script-src 'unsafe-inline' 'self'; style-src 'unsafe-inline' 'self'; " +
	"img-src 'self' data: blob: https:; connect-src 'none'"

// appShimJS is window.mu inside the sandbox: the same API, implemented by
// asking the parent instead of by fetching.
//
// connect-src 'none' above means the app cannot make network requests of its
// own even if it tries, so this is the only way out — and it goes through the
// op table.
const appShimJS = `<script>
(function(){
  var seq=0, waiting={};
  window.addEventListener('message', function(e){
    var m=e.data;
    if(!m||m.mu!=='reply'||!waiting[m.id]) return;
    var w=waiting[m.id]; delete waiting[m.id];
    if(m.error){ w.reject(new Error(m.error)); } else { w.resolve(m.result); }
  });
  function ask(op, args){
    return new Promise(function(resolve,reject){
      var id=++seq; waiting[id]={resolve:resolve,reject:reject};
      parent.postMessage({mu:'call', id:id, op:op, args:args||{}}, '*');
      setTimeout(function(){ if(waiting[id]){ delete waiting[id]; reject(new Error('timed out')); } }, 60000);
    });
  }
  function proxy(op,body){ return ask('sdk:'+op, body); }

  window.mu={
    service:function(n,m,a){return proxy('service',{service:n,method:m,args:a||{}})},
    services:function(){return proxy('services',{})},

    weather:function(o){return ask('weather',{query:{lat:o.lat,lon:o.lon,pollen:o.pollen?'1':''}})},
    news:function(){return ask('news',{})},
    markets:function(o){return ask('markets',{query:{category:(o&&o.category)||''}})},
    video:function(){return ask('video',{})},
    social:function(){return ask('social',{})},
    search:function(q){return ask('search',{query:{q:q}})},
    chat:function(p){return ask('chat',{body:{prompt:p}})},
    blog:{
      list:function(){return ask('blog.list',{})},
      read:function(id){return ask('blog.read',{query:{id:id}})},
      create:function(o){return ask('blog.create',{body:o})},
    },
    places:{
      search:function(o){return ask('places.search',{body:o})},
      nearby:function(o){return ask('places.nearby',{body:o})},
    },
    apps:{
      list:function(){return ask('apps.list',{})},
      read:function(s){return ask('apps.read',{suffix:s})},
    },
    ai:function(p,o){return proxy('ai',{prompt:p,options:o||{}}).then(function(j){return j.result||j})},
    agent:function(p){return ask('agent',{body:{prompt:p}}).then(function(j){return j.answer||j})},
    user:function(){return ask('user',{})},

    store:{
      set:function(k,v){return proxy('store',{op:'set',key:k,value:v})},
      get:function(k){return proxy('store',{op:'get',key:k}).then(function(j){return j.result})},
      del:function(k){return proxy('store',{op:'del',key:k})},
      keys:function(){return proxy('store',{op:'keys'}).then(function(j){return j.result})},
    },
    db:{
      create:function(c,d,o){return proxy('db',{op:'create',collection:c,data:d,public:!!(o&&o.public)}).then(function(j){return j.record})},
      get:function(c,id){return proxy('db',{op:'get',collection:c,id:id}).then(function(j){return j.record})},
      list:function(c,o){o=o||{};return proxy('db',{op:'list',collection:c,scope:o.scope||'mine',where:o.where||null,sort:o.sort||'',order:o.order||'desc',limit:o.limit||0}).then(function(j){return j.records||[]})},
      update:function(c,id,d,o){return proxy('db',{op:'update',collection:c,id:id,data:d,public:!!(o&&o.public)}).then(function(j){return j.record})},
      del:function(c,id){return proxy('db',{op:'delete',collection:c,id:id})},
    },
    web:{
      fetch:function(u,o){o=o||{};return proxy('fetch',{url:u,method:o.method||'GET',headers:o.headers||null,body:o.body||''})},
    },

    // mu.get and mu.post used to take any path on this origin, with the
    // viewer's cookies. They take an operation now. The old form is refused
    // loudly rather than silently doing nothing, because an app written
    // against it deserves to say why it stopped working.
    get:function(p){return Promise.reject(new Error(
      'mu.get(path) is gone: an app runs sandboxed and cannot call arbitrary endpoints. '+
      'Use the named API — mu.news(), mu.blog.list(), mu.db, mu.service(name, method, args).'))},
    post:function(p,b){return Promise.reject(new Error(
      'mu.post(path, body) is gone: an app runs sandboxed and cannot call arbitrary endpoints. '+
      'Use the named API — mu.blog.create(), mu.chat(), mu.db, mu.service(name, method, args).'))},

    errors:[],
    eval:function(code){
      try{var r=eval(code);return{ok:true,result:String(r)}}
      catch(e){return{ok:false,error:e.message}}
    },
  };
  // ── The web platform, over the same bridge ──────────────────────
  //
  // An app runs in an opaque origin, so localStorage throws and fetch is dead
  // — connect-src 'none'. Everything an app can do therefore had to be spelled
  // mu.something, and that is a real cost rather than a matter of taste: a
  // model has seen localStorage.setItem a billion times and mu.store.set never.
  // Every generated app paid for the difference, and the fix is not a longer
  // prompt.
  //
  // So the two names that matter are put back, implemented over the bridge that
  // was already there. An app written the ordinary way now works, and mu.*
  // stays as the shorthand for the things the web has no name for.

  // localStorage, answering synchronously because that is its contract.
  //
  // The values are already in the page: handleApp inlines this app's saved
  // store into the document before this script runs, so a read is a lookup in
  // a map rather than a round trip. Writes go both ways — into the map now, so
  // the next read is right, and to the server after, so a reload keeps it.
  (function(){
    var mem={}, seed=window.__muStore||{};
    try{ delete window.__muStore; }catch(e){ window.__muStore=undefined; }
    for(var k in seed){
      mem[k] = typeof seed[k]==='string' ? seed[k] : JSON.stringify(seed[k]);
    }
    function quiet(){}
    var saved={
      getItem:function(k){k=String(k);return Object.prototype.hasOwnProperty.call(mem,k)?mem[k]:null},
      setItem:function(k,v){k=String(k);mem[k]=String(v);mu.store.set(k,mem[k]).catch(quiet)},
      removeItem:function(k){k=String(k);delete mem[k];mu.store.del(k).catch(quiet)},
      clear:function(){for(var k in mem){mu.store.del(k).catch(quiet)}mem={}},
      key:function(i){var ks=Object.keys(mem);return i<ks.length?ks[i]:null},
    };
    Object.defineProperty(saved,'length',{get:function(){return Object.keys(mem).length}});

    // sessionStorage keeps nothing. It is the same shape and the same tab-only
    // promise it makes everywhere else, so it stays in memory rather than
    // quietly becoming permanent.
    var temp={}, session={
      getItem:function(k){k=String(k);return Object.prototype.hasOwnProperty.call(temp,k)?temp[k]:null},
      setItem:function(k,v){temp[String(k)]=String(v)},
      removeItem:function(k){delete temp[String(k)]},
      clear:function(){temp={}},
      key:function(i){var ks=Object.keys(temp);return i<ks.length?ks[i]:null},
    };
    Object.defineProperty(session,'length',{get:function(){return Object.keys(temp).length}});

    try{ Object.defineProperty(window,'localStorage',{value:saved,configurable:true}); }catch(e){}
    try{ Object.defineProperty(window,'sessionStorage',{value:session,configurable:true}); }catch(e){}
  })();

  // fetch, for this instance's API and nothing else.
  //
  // /api/v1/<service>/<method> is the door already built for callers who are
  // not trusted — it turns a path into a tool name and hands it to the same
  // ExecuteTool /mcp uses, with that door's auth and price gate. So this is not
  // the hole mu.get(path) was: it is not "any path on this origin with the
  // viewer's cookies", it is the one door designed to be pointed at.
  //
  // Anything else is refused in words rather than by hanging, because an app
  // that tried to reach an external API should be told why it cannot.
  (function(){
    var API=/^\/api\/v1\/([a-z0-9_-]+)\/([a-zA-Z0-9_]+)\/?$/;
    function answer(d){
      var body=JSON.stringify(d);
      return {ok:true,status:200,statusText:'OK',url:'',redirected:false,type:'basic',
        headers:{get:function(n){return String(n).toLowerCase()==='content-type'?'application/json':null}},
        json:function(){return Promise.resolve(d)},
        text:function(){return Promise.resolve(body)},
        clone:function(){return answer(d)}};
    }
    window.fetch=function(input,init){
      var url = typeof input==='string' ? input : (input && input.url) || '';
      var cut = String(url).split('#')[0].split('?');
      var m = API.exec(cut[0]);
      if(!m){
        return Promise.reject(new TypeError(
          'An app can only fetch this instance: /api/v1/<service>/<method>. '+
          'Anything else is blocked by the sandbox. mu.services() lists what is here.'));
      }
      var args={};
      if(cut[1]){
        cut[1].split('&').forEach(function(pair){
          if(!pair) return;
          var kv=pair.split('=');
          args[decodeURIComponent(kv[0])]=decodeURIComponent((kv[1]||'').replace(/\+/g,' '));
        });
      }
      init=init||{};
      if(init.body){ try{ var b=JSON.parse(init.body); for(var k in b){ args[k]=b[k]; } }catch(e){} }
      return mu.service(m[1],m[2],args).then(answer);
    };
  })();

  window.onerror=function(msg,src,line){mu.errors.push({type:'error',message:msg,source:src,line:line});};
  window.onunhandledrejection=function(e){mu.errors.push({type:'promise',message:String(e.reason)});};
})();
</script>`

// appSeedJS carries this app's saved values into the document.
//
// localStorage is synchronous and the bridge is not, and no amount of cleverness
// bridges that gap after the page has started: an app that reads a key in its
// first line cannot be made to wait for a postMessage. So the values are already
// there when the script runs, put in by the server that knows both the app and
// who is looking at it.
//
// Nothing at all for a signed-out reader or an app with nothing saved, and
// nothing past seedLimit either — a store that large is one an app should be
// reading through mu.db, and a megabyte of JSON in front of the document is a
// page that takes a second to parse.
func appSeedJS(slug, accountID string) string {
	if slug == "" || accountID == "" {
		return ""
	}
	store := loadStore(fmt.Sprintf("apps/%s/%s.json", slug, accountID))
	if len(store) == 0 {
		return ""
	}
	b, err := json.Marshal(store)
	if err != nil || len(b) > seedLimit {
		return ""
	}
	return `<script>window.__muStore=` + string(b) + `;</script>`
}

// seedLimit bounds what is inlined. The store allows a hundred keys of 64KB,
// which is six megabytes nobody wants in front of a page.
const seedLimit = 256 << 10

// appBridgeJS runs on the parent page — our origin, our code — and is the only
// thing that touches the session on an app's behalf.
func appBridgeJS(slug string) string {
	return `<script>
(function(){
  ` + bridgeTable() + `
  var SLUG=` + jsString(slug) + `;
  var frame=document.getElementById('app-frame');
  var j='application/json';

  function csrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}

  function reply(win,id,result,error){
    win.postMessage({mu:'reply',id:id,result:result,error:error}, '*');
  }

  function segment(s){ return String(s||'').replace(/[^a-zA-Z0-9_-]/g,''); }

  function query(q){
    if(!q) return '';
    var parts=[];
    for(var k in q){ if(q[k]!=='' && q[k]!=null){ parts.push(encodeURIComponent(k)+'='+encodeURIComponent(q[k])); } }
    return parts.length?('?'+parts.join('&')):'';
  }

  window.addEventListener('message', function(e){
    var m=e.data;
    if(!m||m.mu!=='call') return;
    // Only the frame we created. A sandboxed frame has a null origin, so the
    // window identity is the check that means anything.
    if(!frame||e.source!==frame.contentWindow) return;

    var op=String(m.op||''), args=m.args||{};

    // The server-side proxy: caller bound from the session, app named in the
    // path. These were never the problem.
    if(op.indexOf('sdk:')===0){
      var sub=op.slice(4);
      if(PROXY.indexOf(sub)<0){ reply(e.source,m.id,null,'unknown operation'); return; }
      fetch('/apps/'+encodeURIComponent(SLUG)+'/sdk/'+sub,{
        method:'POST',headers:{'Content-Type':j,'Accept':j,'X-CSRF-Token':csrf()},
        body:JSON.stringify(args)})
        .then(function(r){return r.json()})
        .then(function(d){reply(e.source,m.id,d,null)})
        .catch(function(err){reply(e.source,m.id,null,String(err))});
      return;
    }

    var spec=OPS[op];
    if(!spec){ reply(e.source,m.id,null,'this app asked for something it is not allowed to do: '+op); return; }

    var path=spec.p;
    if(spec.s){ path=path+segment(args.suffix); }
    if(spec.q){ path=path+query(args.query); }

    var init={headers:{'Accept':j}};
    if(spec.m==='POST'){
      init.method='POST';
      init.headers['Content-Type']=j;
      init.headers['X-CSRF-Token']=csrf();
      init.body=JSON.stringify(args.body||{});
    }
    fetch(path,init)
      .then(function(r){return r.json().catch(function(){return {}})})
      .then(function(d){reply(e.source,m.id,d,null)})
      .catch(function(err){reply(e.source,m.id,null,String(err))});
  });
})();
</script>`
}

// jsString quotes a Go string for embedding in JavaScript.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// sandboxPage wraps an app in the frame that isolates it.
func sandboxPage(slug, title string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<title>` + html.EscapeString(title) + `</title>`)
	// The white flash between the two documents.
	//
	// Opening an app is two paints and cannot be one: this page arrives first
	// and the app inside it arrives after, because inlining the app would put
	// untrusted HTML in this origin, which is the whole thing the frame exists
	// to prevent. What can go is the *colour* of that gap — it was a hard #fff,
	// so a reader in dark mode got a full-screen white flash on every app they
	// opened, which reads as the page breaking rather than as it loading.
	//
	// Canvas is the system background and color-scheme is what tells the
	// browser which one to use. Two words, no media query, and it follows the
	// reader rather than a guess made here.
	b.WriteString(`<style>html,body{margin:0;padding:0;height:100%;background:Canvas;color-scheme:light dark}
#app-frame{display:block;width:100%;height:100%;border:0;background:Canvas}</style>`)
	b.WriteString(`</head><body>`)
	// The app itself, not /run. That word is retired — see embed.go — and the
	// document is at the app's own address with raw=1.
	b.WriteString(`<iframe id="app-frame" src="/apps/` + html.EscapeString(slug) +
		`?raw=1" sandbox="allow-scripts allow-forms allow-popups allow-modals" ` +
		`allow="geolocation"></iframe>`)
	b.WriteString(appBridgeJS(slug))
	b.WriteString(`</body></html>`)
	return b.String()
}
