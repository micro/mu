package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/apps"
	"mu/internal/app"
	"mu/blog"
	"mu/internal/event"
	"mu/news"
	"mu/markets"
	"mu/reminder"
	"mu/video"
)

// landingTemplate is the full HTML template for the public landing page.
// %s slot: cssVersion only — preview content is fetched client-side from the public API.
var landingTemplate = `<html lang="en">
  <head>
    <title>Mu - The Micro Network</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="description" content="The Micro Network. Apps without ads, algorithms, or tracking.">
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css?%s">
    <style>
      .preview-tabs { display:flex; gap:8px; justify-content:center; margin-bottom:20px; flex-wrap:nowrap; overflow-x:auto; -webkit-overflow-scrolling:touch; scrollbar-width:none; padding:2px 0; }
      .preview-tabs::-webkit-scrollbar { display:none; }
      .preview-tab {
        padding:6px 18px; border:1px solid #ccc; border-radius:20px;
        background:#fff; cursor:pointer; font-size:14px; font-family:inherit;
        transition:background 0.15s,border-color 0.15s;
	color: black; white-space:nowrap; flex-shrink:0;
      }
      .preview-tab:hover { background:#f5f5f5; }
      .preview-tab img { filter:brightness(0); }
      .preview-tab.active { background:#000; color:#fff; border-color:#000; }
      .preview-tab.active img { filter:brightness(0) invert(1); }
      .preview-panel { display:none; }
      .preview-panel.active { display:block; }
      .example-panel { display:none; }
      .example-panel.active { display:flex; gap:20px; flex-wrap:wrap; }
      .skeleton { background:linear-gradient(90deg,#f0f0f0 25%%,#e0e0e0 50%%,#f0f0f0 75%%);
        background-size:200%% 100%%; animation:shimmer 1.4s infinite; border-radius:4px; }
      @keyframes shimmer { 0%%{background-position:200%% 0} 100%%{background-position:-200%% 0} }
      @media(max-width:600px){
        .preview-tabs { gap:6px; flex-wrap:wrap; }
        .preview-tab { padding:5px 10px; font-size:12px; flex-shrink:1; }
        .preview-tab img { display:none; }
      }
    </style>
  </head>
  <body>
    <div style="float: right; padding: 20px;">
      <a href="/about"><b>About</b></a>&nbsp;
      <a href="/docs"><b>Docs</b></a>&nbsp;
      <a href="/plans"><b>Plans</b></a>&nbsp;
      <a href="/login"><b>Login</b></a>
    </div>
    <div id="main">
      <div id="title">Mu</div>
      <div id="desc">The Micro Network</div>
      <p style="font-size: 18px; font-weight: 800; color: #333; margin: 20px 0; text-align: center; max-width: 800px;">
      The internet without the noise.
      </p>
      <p style="color:#555;max-width:600px;margin:0 auto 10px;text-align:center;">
      News, markets, video, search, and AI tools — all in one place. Build your own or let the agent build one for you.
      </p>


      <div style="height: 60px;"></div>

      <h3>Live Data</h3>
      <p style="color:#555;max-width:600px;margin:0 auto 20px;">Real-time information at your fingertips</p>

      <div class="preview-tabs">
        <button class="preview-tab active" onclick="showPreview('news',this)">
          <img src="/news.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">News
        </button>
        <button class="preview-tab" onclick="showPreview('markets',this)">
          <img src="/markets.svg" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Markets
        </button>
        <button class="preview-tab" onclick="showPreview('video',this)">
          <img src="/video.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Video
        </button>
        <button class="preview-tab" onclick="showPreview('web',this)">
          <img src="/search.svg" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Web
        </button>
        <button class="preview-tab" onclick="showPreview('blog',this)">
          <img src="/post.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Blog
        </button>
      </div>

      <!-- Card panels — content loaded client-side from public JSON API -->
      <div style="max-width:750px;margin:0 auto;text-align:left;width:100%%;box-sizing:border-box;padding:0 12px;overflow:hidden;">
        <div id="preview-news" class="preview-panel active">
          <div class="card">
            <h4 style="margin-top:0;"><img src="/news.png" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">News</h4>
            <div id="preview-news-content"><div class="skeleton" style="height:14px;margin:8px 0;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:80%%;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:90%%;"></div></div>
            <a href="/news" class="link" style="margin-top:8px;display:inline-block;">More news &#x2192;</a>
          </div>
        </div>
        <div id="preview-markets" class="preview-panel">
          <div class="card">
            <h4 style="margin-top:0;"><img src="/markets.svg" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">Markets</h4>
            <div id="preview-markets-content"><div class="skeleton" style="height:60px;margin:8px 0;"></div></div>
            <a href="/markets" class="link" style="margin-top:8px;display:inline-block;">More &#x2192;</a>
          </div>
        </div>
        <div id="preview-video" class="preview-panel">
          <div class="card">
            <h4 style="margin-top:0;"><img src="/video.png" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">Video</h4>
            <div id="preview-video-content"><div class="skeleton" style="height:80px;margin:8px 0;"></div></div>
            <a href="/video" class="link" style="margin-top:8px;display:inline-block;">More videos &#x2192;</a>
          </div>
        </div>
        <div id="preview-web" class="preview-panel">
          <div class="card" style="overflow:hidden;">
            <h4 style="margin-top:0;"><img src="/search.svg" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">Web</h4>
            <div id="preview-web-content" style="min-width:0;overflow:hidden;"><div class="skeleton" style="height:14px;margin:8px 0;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:80%%;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:90%%;"></div></div>
            <a href="/web" class="link" style="margin-top:8px;display:inline-block;">Search the web &#x2192;</a>
          </div>
        </div>
        <div id="preview-blog" class="preview-panel">
          <div class="card">
            <h4 style="margin-top:0;"><img src="/post.png" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">Blog</h4>
            <div id="preview-blog-content"><div class="skeleton" style="height:14px;margin:8px 0;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:80%%;"></div><div class="skeleton" style="height:14px;margin:8px 0;width:90%%;"></div></div>
            <a href="/blog" class="link" style="margin-top:8px;display:inline-block;">More posts &#x2192;</a>
          </div>
        </div>
      </div>

      <div style="height: 60px;"></div>

      <h3>Our Mission</h3>
      <p style="max-width: 600px">
      Mu is built with the intention that tools should serve humanity, enabling you to browse, search and chat without ads, algorithms or tracking.
      </p>

      <div style="height: 60px;"></div>

      <h3>Services</h3>
      <p>The tools powering Mu</p>
      <div id="links">
        <a href="/agent" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/agent.svg" alt="Agent" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Agent</b>
            <div class="small">AI that can search, answer, and build</div>
          </div>
        </a>
        <a href="/apps" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/apps.svg" alt="Apps" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Apps</b>
            <div class="small">Build small, useful tools — or ask the agent</div>
          </div>
        </a>
        <a href="/blog" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/post.png" alt="Blog" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Blog</b>
            <div class="small">Microblogging with daily AI-generated digests</div>
          </div>
        </a>
        <a href="/chat" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/chat.png" alt="Chat" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Chat</b>
            <div class="small">AI-powered conversation on any topic</div>
          </div>
        </a>
        <a href="/mail" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/mail.png" alt="Mail" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Mail</b>
            <div class="small">Private messaging and email</div>
          </div>
        </a>
        <a href="/markets" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/markets.svg" alt="Markets" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Markets</b>
            <div class="small">Live crypto, futures, and commodity prices</div>
          </div>
        </a>
        <a href="/news" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/news.png" alt="News" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>News</b>
            <div class="small">Headlines and articles with AI summaries</div>
          </div>
        </a>
        <a href="/video" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/video.png" alt="Video" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Video</b>
            <div class="small">YouTube without ads, algorithms, or shorts</div>
          </div>
        </a>
        <a href="/web" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/search.svg" alt="Web" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Web</b>
            <div class="small">Search the web without tracking</div>
          </div>
        </a>
      </div>

      <div style="height: 60px;"></div>

      <!-- API & MCP section (below Featured Apps) -->
      <h3>API &amp; MCP</h3>
      <p style="color:#555;max-width:600px;margin:0 auto 20px;">Every feature is available via REST API and <a href="/mcp">Model Context Protocol</a> for AI clients and agents.</p>

      <div class="preview-tabs">
        <button class="preview-tab active" onclick="showExample('news',this)">
          <img src="/news.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">News
        </button>
        <button class="preview-tab" onclick="showExample('markets',this)">
          <img src="/markets.svg" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Markets
        </button>
        <button class="preview-tab" onclick="showExample('video',this)">
          <img src="/video.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Video
        </button>
        <button class="preview-tab" onclick="showExample('web',this)">
          <img src="/search.svg" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Web
        </button>
        <button class="preview-tab" onclick="showExample('blog',this)">
          <img src="/post.png" style="width:14px;height:14px;vertical-align:middle;margin-right:4px;">Blog
        </button>
      </div>

      <div style="max-width:900px;margin:0 auto;text-align:left;">
        <!-- News examples -->
        <div id="example-news" class="example-panel active">
          <div class="card" style="flex:1;min-width:260px;">
            <h4>REST API</h4>
            <p class="card-desc">Fetch the latest news feed or search for articles.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /news HTTP/1.1
Accept: application/json

POST /news HTTP/1.1
{"query":"technology"}</pre>
            <a href="/api" class="link">API Docs &#x2192;</a>
          </div>
          <div class="card" style="flex:1;min-width:260px;">
            <h4>MCP</h4>
            <p class="card-desc">AI agents can read the news feed or search articles.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"news_search",
  "arguments":{"query":"technology"}}}</pre>
            <a href="/mcp" class="link">MCP Server &#x2192;</a>
          </div>
        </div>
        <!-- Markets examples -->
        <div id="example-markets" class="example-panel">
          <div class="card" style="flex:1;min-width:260px;">
            <h4>REST API</h4>
            <p class="card-desc">Get live crypto, futures, and commodity prices.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /markets HTTP/1.1
Accept: application/json

GET /markets?category=crypto HTTP/1.1
Accept: application/json</pre>
            <a href="/api" class="link">API Docs &#x2192;</a>
          </div>
          <div class="card" style="flex:1;min-width:260px;">
            <h4>MCP</h4>
            <p class="card-desc">Agents can query live market data and check prices.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"markets",
  "arguments":{"category":"crypto"}}}</pre>
            <a href="/mcp" class="link">MCP Server &#x2192;</a>
          </div>
        </div>
        <!-- Video examples -->
        <div id="example-video" class="example-panel">
          <div class="card" style="flex:1;min-width:260px;">
            <h4>REST API</h4>
            <p class="card-desc">Browse the latest videos or search across channels.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /video HTTP/1.1
Accept: application/json

POST /video HTTP/1.1
{"query":"bitcoin"}</pre>
            <a href="/api" class="link">API Docs &#x2192;</a>
          </div>
          <div class="card" style="flex:1;min-width:260px;">
            <h4>MCP</h4>
            <p class="card-desc">Agents can search and retrieve videos across all channels.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"video_search",
  "arguments":{"query":"bitcoin"}}}</pre>
            <a href="/mcp" class="link">MCP Server &#x2192;</a>
          </div>
        </div>
        <!-- Web examples -->
        <div id="example-web" class="example-panel">
          <div class="card" style="flex:1;min-width:260px;">
            <h4>REST API</h4>
            <p class="card-desc">Search the web powered by Brave — results cached for 5 minutes.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /web?q=what+are+AI+agents HTTP/1.1
Accept: application/json
Authorization: Bearer TOKEN</pre>
            <a href="/api" class="link">API Docs &#x2192;</a>
          </div>
          <div class="card" style="flex:1;min-width:260px;">
            <h4>MCP</h4>
            <p class="card-desc">Agents can search the web for current information and news.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"web_search",
  "arguments":{"q":"what are AI agents"}}}</pre>
            <a href="/mcp" class="link">MCP Server &#x2192;</a>
          </div>
        </div>
        <!-- Blog examples -->
        <div id="example-blog" class="example-panel">
          <div class="card" style="flex:1;min-width:260px;">
            <h4>REST API</h4>
            <p class="card-desc">List blog posts or read the latest daily digest.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /blog HTTP/1.1
Accept: application/json

GET /blog/{id} HTTP/1.1
Accept: application/json</pre>
            <a href="/api" class="link">API Docs &#x2192;</a>
          </div>
          <div class="card" style="flex:1;min-width:260px;">
            <h4>MCP</h4>
            <p class="card-desc">Agents can list posts or read the daily digest.</p>
            <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"blog_list",
  "arguments":{}}}
{"method":"tools/call","params":{
  "name":"blog_read",
  "arguments":{"id":"POST_ID"}}}</pre>
            <a href="/mcp" class="link">MCP Server &#x2192;</a>
          </div>
        </div>
      </div>

      <div style="height: 24px;"></div>

      <!-- Wallet / agent credits highlight -->
      <div style="max-width:900px;margin:0 auto;text-align:left;">
        <div class="card" style="border-left:4px solid #000;">
          <h4 style="margin-top:0;">&#x1F4B3; Agent Wallet</h4>
          <p class="card-desc">AI agents have full access to the built-in wallet via MCP. Check your credit balance, top up via crypto or card, and pay per-query automatically — no manual intervention required.</p>
          <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"method":"tools/call","params":{
  "name":"wallet_balance",
  "arguments":{}}}
{"method":"tools/call","params":{
  "name":"wallet_topup",
  "arguments":{}}}</pre>
          <a href="/wallet" class="link">Wallet &amp; Credits &#x2192;</a>
        </div>
      </div>

      <div style="height: 60px;"></div>

      <hr />

      <div style="text-align: center; margin: 20px 0;">
        <a href="/login"><button class="btn" style="font-size:1em;padding:10px 28px;height:auto;">Get Started</button></a>
        <button id="install-pwa" style="display: none;">Install App</button>
      </div>

      <div style="height: 60px;"></div>

      <hr />

      <h3>FAQ</h3>

      <div style="height: 20px;"></div>

      <p><strong>Is Mu free to use?</strong><br>
      Yes! Create an account and start using Mu immediately at no cost.</p>

      <div style="height: 20px;"></div>

      <p><strong>Can I self-host Mu?</strong><br>
      Absolutely. Mu is open source and runs as a single Go binary. Check <a href="https://github.com/micro/mu" target="_blank">GitHub</a> for install instructions.</p>

      <div style="height: 20px;"></div>

      <p><strong>What about pricing?</strong><br>
      Browsing is free — news, blogs, videos, AI summaries, all of it. Searching, posting, and AI features use credits. You get 20 free credits per day, then pay as you go from 1p. See our <a href="/plans">plans</a> for details.</p>

      <div style="height: 20px;"></div>

      <p><strong>How is this different from big tech platforms?</strong><br>
      No ads, no algorithmic feeds, no data mining. Mu brings news, markets, video, chat, and more together in one place — browse for free, pay only for what you use.</p>

      <div style="height: 20px;"></div>

      <p><strong>Can I build my own tools?</strong><br>
      Yes. Describe what you need and the AI builds it — a working app in seconds. Or use the <a href="/apps/build">app builder</a> with templates and a live preview. Apps are just HTML, no frameworks needed.</p>

      <div style="height: 20px;"></div>

      <p><strong>Can AI agents use Mu?</strong><br>
      Yes. Mu supports the <a href="/mcp">Model Context Protocol (MCP)</a>. Agents can read news, search videos, send mail, query markets, build apps, and manage their own wallet credits. See the <a href="/mcp">MCP page</a> for setup.</p>

      <div style="height: 60px;"></div>
    </div>
  <script>
    // ── Tab switching ──────────────────────────────────────────────────────────
    function showPreview(name, btn) {
      document.querySelectorAll('.preview-panel').forEach(function(el){el.classList.remove('active');});
      var p=document.getElementById('preview-'+name); if(p) p.classList.add('active');
      btn.closest('.preview-tabs').querySelectorAll('.preview-tab').forEach(function(b){b.classList.remove('active');});
      btn.classList.add('active');
    }
    function showExample(name, btn) {
      document.querySelectorAll('.example-panel').forEach(function(el){el.classList.remove('active');});
      var p=document.getElementById('example-'+name); if(p) p.classList.add('active');
      btn.closest('.preview-tabs').querySelectorAll('.preview-tab').forEach(function(b){b.classList.remove('active');});
      btn.classList.add('active');
    }

    // ── Live preview fetching ─────────────────────────────────────────────────
    function esc(s){ return s?String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&#34;'):''; }

    function timeAgo(iso) {
      if (!iso) return '';
      var d=new Date(iso), secs=Math.floor((Date.now()-d)/1000);
      if(secs<60) return 'just now';
      if(secs<3600) return Math.floor(secs/60)+'m ago';
      if(secs<86400) return Math.floor(secs/3600)+'h ago';
      return Math.floor(secs/86400)+'d ago';
    }

    function formatPrice(price) {
      if (!price) return '–';
      if (price >= 1000) return '$' + Math.round(price).toLocaleString();
      if (price >= 1)    return '$' + price.toFixed(2);
      return '$' + price.toFixed(4);
    }

    // News
    fetch('/news', {headers:{'Accept':'application/json'}})
      .then(function(r){return r.json();})
      .then(function(d){
        var seen={};var posts=[];var all=d.feed||[];
        for(var i=0;i<all.length&&posts.length<5;i++){var cat=all[i].category||'_';if(!seen[cat]){seen[cat]=true;posts.push(all[i]);}}

        var el=document.getElementById('preview-news-content');
        if(!el) return;
        if(!posts.length){el.innerHTML='<p style="color:#888;font-size:13px;">No headlines yet.</p>';return;}
        var h='';
        posts.forEach(function(p){
          var link=p.id?'/news?id='+esc(p.id):esc(p.url||'#');
          var cat=p.category?'<a href="/news#'+esc(p.category)+'" class="category" style="font-size:11px;margin-right:6px;">'+esc(p.category)+'</a>':'';
          var age=p.posted_at?'<span style="font-size:11px;color:#888;">'+timeAgo(p.posted_at)+'</span>':'';
          h+='<div style="padding:8px 0;border-bottom:1px solid #f0f0f0;">'+cat+age+
             '<a href="'+link+'" style="font-size:13px;font-weight:600;display:block;line-height:1.4;margin-top:2px;color:#111;">'+esc(p.title)+'</a>'+
             '</div>';
        });
        el.innerHTML=h;
      })
      .catch(function(){});

    // Markets
    fetch('/markets', {headers:{'Accept':'application/json'}})
      .then(function(r){return r.json();})
      .then(function(d){
        var items=(d.data||[]).filter(function(i){return i.price>0;});
        var el=document.getElementById('preview-markets-content');
        if(!el) return;
        if(!items.length){el.innerHTML='<p style="color:#888;font-size:13px;">Prices loading…</p>';return;}
	var h='<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px;margin-bottom:4px;">';
        items.forEach(function(item){
          var chg='';
          if(item.change_24h){
            var sign=item.change_24h>=0?'+':'',color=item.change_24h>=0?'#28a745':'#dc3545';
            chg='<span style="font-size:11px;color:'+color+';">'+sign+item.change_24h.toFixed(1)+'%%</span>';
          }
	  h+='<div style="background:#f9f9f9;border-radius:6px;padding:8px 10px;text-align:center;">'+
             '<div style="font-size:11px;font-weight:700;color:#555;letter-spacing:.5px;">'+esc(item.symbol)+'</div>'+
             '<div style="font-size:15px;font-weight:800;">'+formatPrice(item.price)+' '+chg+'</div>'+
             '</div>';
        });
        h+='</div>';
        el.innerHTML=h;
      })
      .catch(function(){});

    // Video
    fetch('/video', {headers:{'Accept':'application/json'}})
      .then(function(r){return r.json();})
      .then(function(d){
        var channels=d.channels||{};
        var all=[];
        Object.keys(channels).forEach(function(ch){(channels[ch].videos||[]).forEach(function(v){all.push(v);});});
        all.sort(function(a,b){return new Date(b.published)-new Date(a.published);});
        var seenCh={};var filtered=[];
        for(var i=0;i<all.length&&filtered.length<4;i++){var ch=all[i].channel||'_';if(!seenCh[ch]){seenCh[ch]=true;filtered.push(all[i]);}}
        all=filtered;
        var el=document.getElementById('preview-video-content');
        if(!el) return;
        if(!all.length){el.innerHTML='<p style="color:#888;font-size:13px;">No videos yet.</p>';return;}
        var h='';
        all.forEach(function(v){
          var thumb=v.thumbnail?'<img src="'+esc(v.thumbnail)+'" style="width:80px;height:45px;object-fit:cover;border-radius:3px;flex-shrink:0;" loading="lazy">':'';
          var meta=(v.channel||'')+(v.published?' · '+timeAgo(v.published):'');
          h+='<div style="display:flex;gap:10px;padding:8px 0;border-bottom:1px solid #f0f0f0;align-items:flex-start;">'+
             thumb+
             '<div style="min-width:0;">'+
             '<a href="'+esc(v.url||'#')+'" style="font-size:13px;font-weight:600;display:block;line-height:1.3;color:#111;">'+esc(v.title)+'</a>'+
             '<div style="font-size:11px;color:#888;margin-top:2px;">'+esc(meta)+'</div>'+
             '</div></div>';
        });
        el.innerHTML=h;
      })
      .catch(function(){});

    // Web (Brave search preview — cached, public endpoint)
    fetch('/web/preview', {headers:{'Accept':'application/json'}})
      .then(function(r){return r.json();})
      .then(function(d){
        var items=d.results||[];
        var el=document.getElementById('preview-web-content');
        if(!el) return;
        if(!items.length){el.innerHTML='<p style="color:#888;font-size:13px;">No results yet.</p>';return;}
        var h='';
        items.forEach(function(r){
          var desc=r.description||'';
          if(desc.length>120) desc=desc.substring(0,120)+'…';
          // Strip HTML tags from description
          desc=desc.replace(/<[^>]*>/g,'');
          var age=r.age?' · '+esc(r.age):'';
          h+='<div style="padding:8px 0;border-bottom:1px solid #f0f0f0;">'+
             '<a href="'+esc(r.url||'#')+'" target="_blank" rel="noopener noreferrer" style="font-size:13px;font-weight:600;display:block;line-height:1.4;color:#111;">'+esc(r.title)+'</a>'+
             '<div style="font-size:12px;color:#555;margin-top:2px;line-height:1.4;word-wrap:break-word;overflow-wrap:break-word;">'+esc(desc)+'</div>'+
             '<div style="font-size:11px;color:#888;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">'+esc(r.url||'')+age+'</div>'+
             '</div>';
        });
        el.innerHTML=h;
      })
      .catch(function(){});

    // Blog (latest opinion)
    fetch('/blog', {headers:{'Accept':'application/json'}})
      .then(function(r){return r.json();})
      .then(function(posts){
        var el=document.getElementById('preview-blog-content');
        if(!el) return;
        if(!posts||!posts.length){el.innerHTML='<p style="color:#888;font-size:13px;">No posts yet.</p>';return;}
        // Find the latest opinion post, fallback to the latest post
        var opinion=null;
        for(var i=0;i<posts.length;i++){if((posts[i].tags||'').indexOf('opinion')!==-1){opinion=posts[i];break;}}
        if(!opinion) opinion=posts[0];
        var title=opinion.title||'Untitled';
        var content=opinion.content||'';
        // Strip markdown formatting for preview
        content=content.replace(/#{1,6}\s/g,'').replace(/\*\*/g,'').replace(/\[([^\]]+)\]\([^)]+\)/g,'$1').replace(/\n/g,' ');
        if(content.length>280) content=content.substring(0,280)+'…';
        var age=opinion.updated_at||opinion.created_at?'<span style="font-size:11px;color:#888;">'+timeAgo(opinion.updated_at||opinion.created_at)+'</span>':'';
        var tag=(opinion.tags||'').indexOf('opinion')!==-1?'<span style="font-size:11px;background:#f0f0f0;padding:2px 8px;border-radius:10px;margin-right:6px;">opinion</span>':'';
        el.innerHTML='<div style="padding:8px 0;">'+tag+age+
          '<a href="/post/'+esc(opinion.id)+'" style="font-size:15px;font-weight:700;display:block;line-height:1.4;margin-top:4px;color:#111;">'+esc(title)+'</a>'+
          '<p style="font-size:13px;color:#555;line-height:1.5;margin-top:6px;">'+esc(content)+'</p>'+
          '</div>';
      })
      .catch(function(){});

    // ── PWA install ───────────────────────────────────────────────────────────
    var deferredPrompt;
    if (navigator.serviceWorker) navigator.serviceWorker.register('/mu.js',{scope:'/'});
    window.addEventListener('beforeinstallprompt',function(e){
      e.preventDefault(); deferredPrompt=e;
      var btn=document.getElementById('install-pwa');
      if(btn) btn.style.display='inline-block';
    });
    var installBtn=document.getElementById('install-pwa');
    if(installBtn) installBtn.addEventListener('click',function(){
      if(!deferredPrompt) return;
      deferredPrompt.prompt();
      deferredPrompt.userChoice.then(function(){ deferredPrompt=null; installBtn.style.display='none'; });
    });
  </script>
  <footer style="text-align:center;padding:40px 20px;color:#888;font-size:14px;border-top:1px solid #eee;margin-top:60px;">
    <a href="/about" style="color:#888;text-decoration:none;margin:0 12px;">About</a>
    <a href="/docs" style="color:#888;text-decoration:none;margin:0 12px;">Docs</a>
    <a href="/api" style="color:#888;text-decoration:none;margin:0 12px;">API</a>
    <a href="/mcp" style="color:#888;text-decoration:none;margin:0 12px;">MCP</a>
    <a href="/plans" style="color:#888;text-decoration:none;margin:0 12px;">Plans</a>
    <a href="/login" style="color:#888;text-decoration:none;margin:0 12px;">Login</a>
  </footer>
  </body>
</html>`

// LandingHandler serves the public-facing landing page with live content previews.
// Preview content is fetched client-side from the public JSON API endpoints.
func LandingHandler(w http.ResponseWriter, r *http.Request) {
	html := fmt.Sprintf(landingTemplate, app.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() string {
	return news.Headlines()
}

func ChatCard() string {
	return `<div id="home-chat">
		<form id="home-chat-form" action="/chat" method="GET">
			<input type="text" name="prompt" placeholder="Ask a question" required>
			<button type="submit">Ask</button>
		</form>
	</div>`
}

func AgentCard() string {
	return `<div id="home-agent">
		<form id="home-agent-form" action="/agent" method="GET">
			<div style="display:flex;gap:8px;">
				<input type="text" name="prompt" placeholder="Ask the agent anything..." required style="flex:1;padding:8px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;">
				<button type="submit" style="padding:8px 16px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;cursor:pointer;">Ask</button>
			</div>
			<div style="display:flex;gap:8px;margin-top:6px;align-items:center;">
				<select name="model" style="padding:4px 8px;font-family:inherit;font-size:13px;border:1px solid #ddd;border-radius:4px;">
					<option value="standard">Fast</option>
					<option value="premium">Best</option>
				</select>
				<span style="flex:1;"></span>
				` + agent.ToolsDropdownHTML() + `
			</div>
		</form>
	</div>`
}

type Card struct {
	ID          string
	Title       string
	Icon        string // Optional icon image path (e.g. "/news.png")
	Column      string // "left" or "right"
	Position    int
	Link        string
	Content     func() string
	CachedHTML  string    // Cached rendered content
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last update timestamp
}

var (
	lastRefresh time.Time
	cacheMutex  sync.RWMutex
	cacheTTL    = 2 * time.Minute
)

type CardConfig struct {
	Left []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"left"`
	Right []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"right"`
}

var Cards []Card

func Load() {
	b, _ := f.ReadFile("cards.json")
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		fmt.Println("Error loading cards.json:", err)
		return
	}

	// Map of card types to their content functions
	cardFunctions := map[string]func() string{
		"agent":    AgentCard,
		"blog":     blog.Preview,
		"chat":     ChatCard,
		"news":     newsCard,
		"markets":  markets.MarketsHTML,
		"reminder": reminder.ReminderHTML,
		"video":    video.Latest,
		"apps":     apps.Preview,
	}

	// Build Cards array from config
	Cards = []Card{}

	for _, c := range config.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "left",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	for _, c := range config.Right {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "right",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	// Sort by column and position
	sort.Slice(Cards, func(i, j int) bool {
		if Cards[i].Column != Cards[j].Column {
			return Cards[i].Column < Cards[j].Column
		}
		return Cards[i].Position < Cards[j].Position
	})

	// Do initial refresh
	RefreshCards()

	// Subscribe to blog and apps update events
	go func() {
		sub := event.Subscribe("blog_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("apps_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
}

// RefreshCards updates card content and timestamps if content changed
func RefreshCards() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	now := time.Now()

	// Check if cache is still valid
	if now.Sub(lastRefresh) < cacheTTL {
		return
	}

	for i := range Cards {
		card := &Cards[i]

		// Get fresh content
		content := card.Content()

		// Calculate hash
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		// Only update if content changed
		if hash != card.ContentHash {
			card.CachedHTML = content
			card.ContentHash = hash
			card.UpdatedAt = now
		}
	}

	lastRefresh = now
}

// ForceRefresh forces an immediate cache refresh (for admin actions)
func ForceRefresh() {
	cacheMutex.Lock()
	lastRefresh = time.Time{} // Reset to zero to force refresh
	cacheMutex.Unlock()
	RefreshCards()
}

// RefreshHandler clears the last_visit cookie to show all cards again
func RefreshHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	cookie := &http.Cookie{
		Name:     "last_visit",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete cookie
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)

	// Redirect back to home
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Refresh cards if cache expired (2 minute TTL)
	RefreshCards()

	var b strings.Builder

	// Feed section — existing home cards below the agent
	var leftHTML []string
	var rightHTML []string

	for _, card := range Cards {
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		html := app.Card(card.ID, card.Title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, html)
		} else {
			rightHTML = append(rightHTML, html)
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n")))
	}

	// Use RenderHTMLWithLang directly to inject a body class that hides the page title,
	// keeping the agent prompt as the primary visual element.
	lang := app.GetUserLanguage(r)
	html := app.RenderHTMLWithLangAndBody("Home", "The home screen", b.String(), lang, ` class="page-home"`)
	w.Write([]byte(html))
}

// htmlEsc escapes HTML special characters.
func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
