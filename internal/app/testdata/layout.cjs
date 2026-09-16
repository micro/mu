const fs=require('fs'),assert=require('assert');
const {chromium}=require(process.env.MU_PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const input=JSON.parse(fs.readFileSync(0,'utf8'));
 const browser=await chromium.launch({executablePath:process.env.MU_LAYOUT_BROWSER,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 try {
 const page=await browser.newPage();
 page.setDefaultTimeout(5000);
 await page.addInitScript(({resultHTML})=>{
  sessionStorage.clear();
  window.SpeechRecognition=class { start(){window.__dictation=this;this.onstart?.()} stop(){this.onend?.()} };
  const originalFetch=window.fetch.bind(window);
  window.fetch=(url,opts)=>{
   if(url==='/agent'&&opts?.method==='POST')window.__lastAgentBody=JSON.parse(opts.body);
   if(url==='/agent'&&opts?.method==='POST')return Promise.resolve(new Response(new ReadableStream({start(controller){
    const send=e=>controller.enqueue(new TextEncoder().encode('data: '+JSON.stringify(e)+'\n\n'));
    send({type:'flow_id',thread:'stream-fixture',flow_id:'flow-fixture'});
    send({type:'stream_token',text:'# Fruit names\n**Arabic** words'});
    setTimeout(()=>{send({type:'response',html:'<div class="markdown-content"><h2>Fruit names</h2><p><strong>Arabic</strong> words</p>'+('<p>A detailed explanation with more information.</p>'.repeat(JSON.parse(opts.body).prompt.includes('long')?45:0))+'</div>'+resultHTML,text:'# Fruit names\n**Arabic** words'});controller.close()},350);
   }}),{headers:{'Content-Type':'text/event-stream'}}));
   return originalFetch(url,opts);
  };
 },{resultHTML:input.resultHTML});
 const failures=[],audit=[];
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/*',route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/fixture.svg'||u.pathname.endsWith('/icon.svg'))return route.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#f2f5e9"/><circle cx="320" cy="180" r="90" fill="#d97a32"/></svg>'});
  if(u.hostname==='www.youtube.com')return route.fulfill({body:'<!doctype html><p>Video fixture</p>',contentType:'text/html'});
  if(u.pathname==='/mu.css')return route.fulfill({contentType:'text/css',body:input.css});
  if(['/mu.js','/shell.js','/viewport.js'].includes(u.pathname))return route.fulfill({body:fs.readFileSync('../internal/app/html'+u.pathname),contentType:'application/javascript'});
  const html=input.pages[u.pathname+u.search]||input.pages[u.pathname];
  if(html)return route.fulfill({contentType:'text/html; charset=utf-8',body:html});
  if(/\.(png|svg)$/.test(u.pathname)){const p='../internal/app/html'+u.pathname;if(fs.existsSync(p))return route.fulfill({body:fs.readFileSync(p),contentType:u.pathname.endsWith('.svg')?'image/svg+xml':'image/png'});}
  return route.fulfill({status:200,body:'',contentType:'text/plain'});
 });
 for(const width of (process.env.MU_LAYOUT_WIDTHS||'320,390,768,1024,1440').split(',').map(Number))for(const collapsed of (width>900?[false,true]:[false])){
  await page.setViewportSize({width,height:900});
  for(const path of Object.keys(input.pages).filter(p=>!process.env.MU_LAYOUT_PATHS||process.env.MU_LAYOUT_PATHS.split(',').includes(p.split('?')[0]))){
   console.log("Checking",path,width,collapsed);
   try {
   errors.length=0;
   await page.goto('https://mu.test'+path);await page.waitForTimeout(50);
   await page.evaluate(c=>document.body.classList.toggle('nav-collapsed',c),collapsed);
   if(['/about','/privacy','/pricing','/contact','/status'].includes(path))assert(await page.locator('.footer').isVisible(),'public footer missing');
   assert.equal(await page.locator('#mobile-nav').count(),0,'sidebar navigation duplicated in bottom bar');
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`${path} overflows at ${width}`);
   for(const revealed of [false,true]) {
    if(revealed)await page.locator('#content details').evaluateAll(es=>es.forEach(e=>e.open=true));
    const bad=await page.evaluate(()=>Array.from(document.querySelectorAll('#content input,#content select,#content textarea')).flatMap(e=>{
     if(['hidden','checkbox','radio','submit','button','range','color'].includes(e.type)||!e.getClientRects().length||e.closest('#chat-form,#mu-chat-form'))return [];
     const r=e.getBoundingClientRect(),p=e.parentElement.getBoundingClientRect();
     if(e.closest('.table-scroll'))return r.width<120?[{name:e.name,width:r.width}]:[];
     return r.width<Math.min(e.tagName==='SELECT'?40:80,p.width-2)||r.right>innerWidth+1||r.left<0?[{name:e.name||e.id,width:r.width,left:r.left,right:r.right}]:[];
    }));
    assert(!bad.length,`${path} controls at ${width}, collapsed=${collapsed}, revealed=${revealed}: ${JSON.stringify(bad)}`);
   }
   await page.locator('#content details').evaluateAll(es=>es.forEach(e=>e.open=false));
   if(path==='/chat')await page.locator('#messages').evaluate(e=>e.innerHTML='<div class="message"><span class="you">Sarah <span class="msg-when">Today, 10:30</span></span><p>Can we meet tomorrow afternoon?</p></div><div class="message"><span class="you">You <span class="msg-when">Today, 10:32</span></span><p>Yes, see you at two.</p></div>');
   const narrowTextareas=await page.locator('form.form:not(.form-inline) > textarea').evaluateAll(es=>es.filter(e=>e.getClientRects().length).filter(e=>{const f=e.parentElement,style=getComputedStyle(f);return e.getBoundingClientRect().width<f.clientWidth-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight)-2}).map(e=>e.name));
   assert(!narrowTextareas.length,`full-width form fields are narrow: ${narrowTextareas}`);
   if(path==='/chat?view=rooms') { const rows=page.locator('.room-row');assert(await rows.count()>0,'room list missing');assert.equal(await rows.first().evaluate(e=>getComputedStyle(e).display),'grid','rooms bypass shared list styling'); }
   if(path==='/archive') {
    const search=await page.locator('.search-bar').boundingBox(),next=await page.locator('.search-bar + *').boundingBox();
    assert(next.y-search.y-search.height>=16,'search section has no spacing beneath it');
   }
   if(path.includes('/blog/post')&&path.includes('edit=true')) {
    const boxes=await page.locator('.form-actions > *').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().height));
    assert(Math.max(...boxes)-Math.min(...boxes)<2,'button links and submit controls differ in height');
   }
   if(await page.locator('.action-menu').count()) {
    const menu=page.locator('.action-menu').first(),items=menu.locator('.action-menu-items');
    assert(!await items.isVisible(),'item actions should start collapsed');
    await menu.locator('summary').click();
    assert(await items.isVisible(),'item actions did not open');
    const box=await items.boundingBox();assert(box.x>=0&&box.x+box.width<=width,'item actions overflow viewport');
    const rows=await items.locator('a').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();return r.y+r.height/2}));
    assert(rows.length<2||rows[1]>rows[0],'item actions run together');
    await page.keyboard.press('Escape');assert(!await items.isVisible(),'escape did not close item actions');
   }
   if(path==='/signup')assert.equal(await page.locator('input[name=name]').count(),0,'signup still asks for a name');
   if(path==='/account') {
    assert(await page.locator('#content a[href="/token"]').count()>0,'API credentials missing');
    assert(await page.locator('#content a[href="/inbox/imap"]').count()>0,'Mail connection help missing');
    assert(await page.locator('#content a[href*="/oauth2/google/"]').count()>0,'Google connections missing');
   }
   if(path.startsWith('/agent?id='))assert.equal(await page.locator('.conversation-toolbar strong').textContent(),'Research','focused agent identity missing');
   if(path==='/services'){assert.equal(await page.locator('.view-switch,#service-feed').count(),0,'services has retired view tabs');assert.equal(await page.locator('#content a[href="/agents"]').count(),0,'Agents is not a service');}
   if(await page.locator('#reply-body').count()) {
    const editor=await page.locator('#reply-body').boundingBox(),form=await page.locator('#reply-body').evaluate(e=>e.closest('form').getBoundingClientRect().width);
    assert(editor.width>=form-2,'mail reply editor is not full width');
   }
   if(path==='/blog?write=true') {
    const colors=await page.locator('.form-actions > *').evaluateAll(es=>es.map(e=>getComputedStyle(e).backgroundColor));
    assert(colors.length>0&&colors.every(c=>c===colors[0]),'ordinary actions have inconsistent backgrounds');
   }
   if(path==='/login'||path==='/signup') {
    const box=await page.locator(path==='/login'?'#login':'#signup').boundingBox();assert(Math.abs(box.x+box.width/2-width/2)<2,'auth form off center');
    assert.equal(await page.locator('.oauth-btn').count(),1,'Google sign-in fixture missing');
    assert.equal(await page.locator('.oauth-btn').evaluate(e=>getComputedStyle(e).display),'flex','Google button unformatted');
   }
   if(await page.locator('.conversation-toolbar').count()&&await page.locator('.conversation-toolbar').isVisible()) {
    assert.equal(await page.locator('.conversation-toolbar a[href="/users"],#conversation-delete').count(),0,'private chat has directory or delete controls');
    const toolbar=await page.locator('.conversation-toolbar').boundingBox(),prompt=await page.locator('#mu-chat-form').boundingBox();
    assert(Math.abs(toolbar.x-prompt.x)<2&&Math.abs(toolbar.width-prompt.width)<2,'toolbar and prompt differ in alignment');
    const controls=await page.locator('.conversation-toolbar > *').evaluateAll(es=>es.filter(e=>e.getClientRects().length).map(e=>{const r=e.getBoundingClientRect();return r.y+r.height/2}));
    assert(Math.max(...controls)-Math.min(...controls)<2,'conversation toolbar wraps');
   }
   if(process.env.MU_LAYOUT_SHOTS&&[390,1440].includes(width)&&!collapsed){
    fs.mkdirSync(process.env.MU_LAYOUT_SHOTS,{recursive:true});await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/'+(path.replace(/[^a-zA-Z0-9_-]/g,'-')||'landing')+'-'+width+'-'+collapsed+'.png',fullPage:true});
   }
   if(path==='/'||path==='/agent/micro?new=1') {
    assert.equal(await page.locator('#mu-chat-input').count(),1,'one composer');
    assert.equal(await page.locator('.conversation-switcher,.conversation-menu').count(),0,'duplicate history controls');
    assert.equal(await page.locator('#nav .chat-sess,#nav-bookmarks').count(),0,'retired sidebar collections');

    assert.equal(await page.locator('#tabs,#home-personal,#home-feed').count(),0,'retired shell');
    const form=page.locator('#mu-chat-form');
    const empty=await form.boundingBox();
    if(path==='/')assert(empty.y>220&&empty.y<620,'public empty prompt is not centered');
    else {assert(900-empty.y-empty.height<=24,'signed-in prompt is not at the bottom');}
    await page.locator('#mu-chat-mic').click();const listening=await form.boundingBox();assert(Math.abs(empty.y-listening.y)<2,'dictation moved prompt');
    assert.equal(await page.locator('#mu-chat-mic').getAttribute('aria-pressed'),'true');
    if(process.env.MU_LAYOUT_SHOTS&&width===390&&path==='/agent/micro?new=1')await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/dictating.png'});
    await page.locator('#mu-chat-mic').click();
    await page.locator('#mu-chat-input').fill('Find fruit names');await page.locator('#mu-chat-form button[type=submit]').click();
    await page.waitForTimeout(100);assert(!await page.locator('.mu-agent').textContent().then(s=>s.includes('**')||s.includes('# Fruit')),'raw Markdown flashed');
    await page.waitForSelector('.mu-agent h2');
    assert.equal(await page.locator('.mu-agent strong').first().textContent(),'Arabic');
    if(process.env.MU_LAYOUT_SHOTS&&width===390&&path==='/agent/micro?new=1')await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/formatted-answer.png'});
    const assertQuestionAnchor=async()=>{
     assert(await page.locator('#mu-chat-conv').evaluate(c=>getComputedStyle(c).scrollbarWidth==='none'),'visible chat scrollbar');
     assert(await page.locator('#mu-chat-conv').evaluate(c=>Math.abs(c.querySelector('.mu-user').getBoundingClientRect().left-c.getBoundingClientRect().left)<2),'question not left aligned');
     const offset=await page.evaluate(()=>{const c=document.getElementById('mu-chat-conv'),q=c.querySelector('.mu-user:last-of-type')||c.querySelectorAll('.mu-user')[c.querySelectorAll('.mu-user').length-1];return q.getBoundingClientRect().top-c.getBoundingClientRect().top;});
     assert(Math.abs(offset-16)<3,`question is not anchored at transcript top: ${offset}`);
    };
    await page.waitForTimeout(50);await assertQuestionAnchor();
    const actions=page.locator('.result-card .form-actions').first();
    const actionStyle=await actions.locator('a,button').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return {x:r.x,right:r.right,h:r.height,color:s.color,underline:s.textDecorationLine};}));
    assert(actionStyle[1].x-actionStyle[0].right>=8,'result actions run together');
    assert.equal(actionStyle[0].h,actionStyle[1].h,'result action heights differ');
    assert.equal(actionStyle[0].color,actionStyle[1].color,'result action colors differ');
    assert.equal(actionStyle[0].underline,'none','result action looks like a prose link');
    await page.locator('#mu-chat-input').fill('Give me a long answer');await form.locator('button[type=submit]').click();
    await page.waitForSelector('.mu-agent:last-child h2');await page.waitForTimeout(50);await assertQuestionAnchor();
    await page.locator('#mu-chat-input').fill('Another long answer');await form.locator('button[type=submit]').click();
    await page.waitForTimeout(100);await page.locator('#mu-chat-conv').evaluate(e=>e.scrollTop=0);
    await page.waitForSelector('.mu-agent:last-child h2');await page.waitForTimeout(50);
    assert(await page.locator('#mu-chat-conv').evaluate(e=>e.scrollTop<2),'response overrode manual scrolling');
    if(path==='/agent/micro?new=1')assert.equal(await page.locator('#conversation-delete').count(),0,'sending recreated the deleted control');
    const before=await form.boundingBox();
    await page.locator('#mu-chat-conv').evaluate(e=>e.innerHTML='<p>A long answer</p>'.repeat(100));
    await page.waitForTimeout(50);const after=await form.boundingBox();
    assert(Math.abs(before.y-after.y)<2,'composer moved as conversation grew');
    assert(after.y+after.height<=900,'composer below viewport');
    const center=await page.locator('#mu-chat').evaluate(e=>{const r=e.getBoundingClientRect();const available=document.body.classList.contains('index-shell')||innerWidth<=900||document.body.classList.contains('nav-collapsed')?0:220;return Math.abs((r.left+r.right)/2-(available+innerWidth)/2)});
    assert(center<2,`conversation offset ${center}px at ${width}`);
    if(path==='/agent/micro?new=1'&&width>900) {
     for(let toggle=0;toggle<2;toggle++) {
      await page.locator('#menu-toggle').click();await page.waitForTimeout(50);
      const alignment=await page.evaluate(()=>{
       const nav=document.getElementById('nav-container').getBoundingClientRect(),form=document.getElementById('mu-chat-form').getBoundingClientRect(),brand=document.getElementById('brand').getBoundingClientRect();
       const left=document.body.classList.contains('nav-collapsed')?0:nav.right,target=(left+innerWidth)/2;
       return {prompt:Math.abs((form.left+form.right)/2-target),brand:Math.abs((brand.left+brand.right)/2-target)};
      });
      assert(alignment.prompt<2&&alignment.brand<2,`sidebar toggle alignment: ${JSON.stringify(alignment)}`);
     }
    }
    if(width<900){await page.evaluate(()=>{const v=new EventTarget();v.height=430;v.offsetTop=0;Object.defineProperty(window,'visualViewport',{configurable:true,value:v});window.dispatchEvent(new Event('resize'));});await page.waitForTimeout(70);const box=await form.boundingBox();assert(box.y+box.height<=430,`keyboard covers composer: ${JSON.stringify(box)}`);}
    assert(!errors.length,`conversation errors: ${errors.join(', ')}`);
   }
   if(path==='/components') {
    const styles=await page.evaluate(()=>{const s=e=>getComputedStyle(document.querySelector(e));return {border:s('.card').borderTopWidth,padding:s('.card').paddingLeft,preview:s('.collection-item').textDecorationLine,notice:s('.notice').backgroundColor,thumb:document.querySelector('.thumbnail img').getBoundingClientRect().width}});
    assert.equal(styles.border,'1px','cards have boundaries');assert.equal(styles.padding,'16px','cards have space');assert.equal(styles.preview,'none','collection previews are readable');assert.notEqual(styles.notice,'rgba(0, 0, 0, 0)','notice has a background');assert(styles.thumb<=320,'bounded thumbnails');
   }
   if(path.startsWith('/inbox?id=')) {
    const row=page.locator('.ib-reply');
    const boxes=await row.locator('a.btn,button').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return {x:r.x,y:r.y,right:r.right,height:r.height,bg:s.backgroundColor};}));
    const inline=page.locator('#inbox-reply');
    assert(boxes.length>=1,'assign fixture missing');
    if(await inline.count()) {
     const before=page.url();await inline.locator('summary').click();
     assert(await inline.locator('textarea').isVisible(),'inline reply did not expand');
     assert.equal(page.url(),before,'reply navigated away');
     const field=await inline.locator('textarea').boundingBox();const panel=await inline.boundingBox();
     assert(field.width>=panel.width-4,'inline reply is not full width');
     await inline.locator('summary').click();
    } else {
     assert(boxes.length>=2,'reply fixture missing');
     assert(Math.abs(boxes[0].height-boxes[1].height)<2,'inbox actions differ in size');
     assert(boxes[1].y>boxes[0].y||boxes[1].x-boxes[0].right>=8,'inbox actions run together');
    }
    assert(boxes.every(b=>b.bg===boxes[0].bg&&b.bg!=='rgba(0, 0, 0, 0)'),'inbox actions lack consistent contrast');
    assert(await page.locator('.ib-from,.ib-msg .you').first().evaluate(e=>getComputedStyle(e).display==='flex'),'thread sender and time run together');
    await page.locator('.ib-assign-open').click();
    const dialog=page.locator('#ib-assign');assert(await dialog.isVisible(),'assign dialog failed to open');
    const dr=await dialog.boundingBox();assert(dr.x>=0&&dr.x+dr.width<=width,'assign dialog overflows');
    const ask=await dialog.locator('textarea').boundingBox();assert(ask.width>=dr.width-52,'assignment field is too narrow');
    await page.keyboard.press('Escape');
   }
   if(path==='/admin/server') {
    assert.equal(await page.locator('.detail-row').first().evaluate(e=>getComputedStyle(e).display),'grid','connect styling leaked into status rows');
    assert.equal(await page.locator('.metric-grid .metric').count(),5,'server metrics missing');
    const metrics=await page.locator('.metric').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().width));
    assert(metrics.every(w=>w>=140),'server metrics compressed');
   }
   if(path==='/admin/oauth') {
    const scroller=page.locator('.table-scroll');
    assert(await scroller.evaluate(e=>e.scrollWidth>=760&&e.getBoundingClientRect().right<=innerWidth),'OAuth table is squeezed or exceeds viewport');
    assert(await page.locator('input[name=redirect_uri]').first().evaluate(e=>e.getBoundingClientRect().width>=160),'OAuth redirect field is too narrow');
    await scroller.evaluate(e=>e.scrollLeft=e.scrollWidth);
    const remove=page.getByRole('button',{name:'Remove'}).first();const r=await remove.boundingBox();
    assert(r.x>=0&&r.x+r.width<=width,'OAuth remove action cannot be reached by scrolling');
    await scroller.evaluate(e=>e.scrollLeft=0);
   }
   if(path==='/admin/moderate') {
    const card=page.locator('#flagged-content .card').first();assert(await card.isVisible(),'moderation fixture empty');
    assert(await card.locator('.form-actions').evaluate(e=>parseFloat(getComputedStyle(e).gap)>=8),'moderation actions lack spacing');
   }
   if(path==='/inbox') {
    assert.equal(await page.locator('article.message').count(),0,'inbox opened a priority item');
    assert(await page.locator('.ib-row').count()>1,'inbox is not a list');
    const first=page.locator('.ib-row').first();await first.hover();
    assert.equal(await first.evaluate(e=>getComputedStyle(e).textDecorationLine),'none','row hover underlines text');
    const labels=await page.locator('.ib-kind').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().x));
    assert(labels.every(x=>Math.abs(x-labels[0])<1),'type labels move between rows');
    assert((await first.boundingBox()).height<110,'inbox rows are too tall');
   }
   if(path==='/maps') {
    const map=await page.locator('#map').boundingBox();
    assert(map.height>=280&&map.height<=560,'map viewport is not bounded');
    const tiles=await page.locator('.map-tile').evaluateAll(es=>es.map(e=>({w:e.getBoundingClientRect().width,h:e.getBoundingClientRect().height,pos:getComputedStyle(e).position})));
    assert(tiles.length>0&&tiles.every(t=>t.w===256&&t.h===256&&t.pos==='absolute'),'map tiles lost native pixel geometry');
   }
   if(path==='/docs') {
    const actions=await page.locator('.form-actions a').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().toJSON()));
    if(actions.length>=2)assert(actions[1].x-actions[0].right>=8||actions[1].y>actions[0].y,'New and Import touch');
   }
   if(path==='/web'||path==='/video') {
    const formID=path==='/web'?'web-search':'video-search', key=path==='/web'?'mu_recent_web_searches':'mu_recent_video_searches';
    await page.evaluate(({key})=>localStorage.setItem(key,JSON.stringify(['bread & butter','Bread  & butter','<img src=x>'])),{key});
    await page.reload();
    await page.evaluate(({formID})=>document.getElementById(formID).addEventListener('submit',e=>{e.preventDefault();window.submittedQuery=new FormData(e.target).get('q')||new FormData(e.target).get('query');}),{formID});
    const recent=page.locator('[data-recent-searches]');
    assert.equal(await recent.locator('img').count(),0,'recent search became markup');
    assert.equal(await recent.locator('button').count(),4,'recent search duplicates remain');
    await recent.getByRole('button',{name:'bread & butter',exact:true}).focus();await page.keyboard.press('Enter');
    assert.equal(await page.evaluate(()=>window.submittedQuery),'bread & butter','recent search did not submit');
    await recent.getByRole('button',{name:'Remove recent search: bread & butter',exact:true}).click();
    assert.equal(await recent.locator('button').count(),2,'remove did not update recent searches');
   }
   if(path==='/chat'){
    await page.locator('#messages').evaluate(e=>e.innerHTML='<p>Room message</p>'.repeat(100));await page.waitForTimeout(70);
    const box=await page.locator('#chat-form').boundingBox();assert(box.y+box.height<=900,`room composer below viewport at ${width}`);
   }
   audit.push({path,width,collapsed,controls:await page.locator('#content button,#content a.btn,#content a.mini-btn').evaluateAll(es=>es.filter(e=>e.getClientRects().length).map(e=>{const s=getComputedStyle(e);return {text:e.textContent.trim().slice(0,40),class:e.className,h:e.getBoundingClientRect().height,pad:s.padding,font:s.fontSize}}))});
   } catch(e) { failures.push(path+' at '+width+': '+e.message); }
  }
 }

 {
 // Reload an older selection even when the server initially renders a newer thread.
 await page.goto('https://mu.test/agent/micro');
 const selectedConfig=await page.locator('#conversation-config').textContent().then(JSON.parse);
 await page.route('**/agent/micro?session=older-fixture',route=>route.fulfill({contentType:'application/json',body:JSON.stringify({id:'older-fixture',html:'<div class="mu-user">An older selected question</div><div class="mu-agent">Its original answer</div>',pending:false,agent:'research',agentName:'Research',storageNS:selectedConfig.storageNS})}));
 await page.evaluate(scope=>history.replaceState({muConversation:{scope,id:'older-fixture'}},''),selectedConfig.selectionScope);
 await page.reload();
 await page.getByText('An older selected question',{exact:true}).waitFor();
 assert.equal(new URL(page.url()).search,'','selection leaked into URL');
 assert.equal(await page.locator('.conversation-toolbar strong').textContent(),'Research','restored named agent identity missing');
 assert(await page.locator('.conversation-toolbar').isVisible(),'restored identity hidden');
 await page.locator('#mu-chat-input').fill('Continue this discussion');
 await page.locator('#mu-chat-form button[type=submit]').click();
 assert.equal(await page.evaluate(()=>window.__lastAgentBody.context_id),'older-fixture','reply switched to newer conversation');
 await page.waitForSelector('.mu-agent h2');
 // A deliberately empty conversation must also survive a reload.
 await page.evaluate(scope=>history.replaceState({muConversation:{scope,id:''}},''),selectedConfig.selectionScope);
 await page.reload();
 assert.equal(await page.locator('#mu-chat-conv').textContent(),'','empty selection reopened latest thread');
 assert.equal(await page.locator('#mu-chat-form').evaluate(f=>f.inert),false);
 }
 if(process.env.MU_LAYOUT_AUDIT)fs.writeFileSync(process.env.MU_LAYOUT_AUDIT,JSON.stringify(audit));
 assert(!failures.length,failures.join('\n'));
 } finally {await browser.close();}
 console.log('All service pages fit; conversation composers remain centered and stable on mobile and desktop.');
})().catch(e=>{console.error(e);process.exitCode=1});
