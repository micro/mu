const fs=require('fs'),assert=require('assert');
const {chromium}=require(process.env.MU_PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const input=JSON.parse(fs.readFileSync(0,'utf8'));
 const browser=await chromium.launch({executablePath:process.env.MU_LAYOUT_BROWSER,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 try {
 const page=await browser.newPage();
 page.setDefaultTimeout(5000);
 await page.addInitScript(()=>{
  sessionStorage.clear();
  window.SpeechRecognition=class { start(){window.__dictation=this;this.onstart?.()} stop(){this.onend?.()} };
  const originalFetch=window.fetch.bind(window);
  window.fetch=(url,opts)=>{
   if(url==='/agent'&&opts?.method==='POST')return Promise.resolve(new Response(new ReadableStream({start(controller){
    const send=e=>controller.enqueue(new TextEncoder().encode('data: '+JSON.stringify(e)+'\n\n'));
    send({type:'flow_id',thread:'stream-fixture',flow_id:'flow-fixture'});
    send({type:'stream_token',text:'# Fruit names\n**Arabic** words'});
    setTimeout(()=>{send({type:'response',html:'<div class="markdown-content"><h2>Fruit names</h2><p><strong>Arabic</strong> words</p></div>',text:'# Fruit names\n**Arabic** words'});controller.close()},350);
   }}),{headers:{'Content-Type':'text/event-stream'}}));
   return originalFetch(url,opts);
  };
 });
 const failures=[];
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/*',route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/services'&&route.request().headers().accept?.includes('application/json'))return route.fulfill({contentType:'application/json',body:JSON.stringify({html:input.feed,loading:false})});
  if(u.pathname==='/fixture.svg'||u.pathname.endsWith('/icon.svg'))return route.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#f2f5e9"/><circle cx="320" cy="180" r="90" fill="#d97a32"/></svg>'});
  if(u.hostname==='www.youtube.com')return route.fulfill({body:'<!doctype html><p>Video fixture</p>',contentType:'text/html'});
  if(u.pathname==='/composition.css')return route.fulfill({contentType:'text/css',body:input.composition});
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
   if(await page.locator('#mobile-nav').count()) {
    assert.deepEqual(await page.locator('#mobile-nav a').allTextContents(),['Home','Inbox','Work','Services']);
    assert.equal(await page.locator('#mobile-nav').isVisible(),width<=900);
   }
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`${path} overflows at ${width}`);
   for(const revealed of [false,true]) {
    if(revealed)await page.locator('#content details').evaluateAll(es=>es.forEach(e=>e.open=true));
    const bad=await page.evaluate(()=>Array.from(document.querySelectorAll('#content input,#content select,#content textarea')).flatMap(e=>{
     if(['hidden','checkbox','radio','submit','button','range','color'].includes(e.type)||!e.getClientRects().length||e.closest('#chat-form,#mu-chat-form'))return [];
     const r=e.getBoundingClientRect(),p=e.parentElement.getBoundingClientRect();
     return r.width<Math.min(e.tagName==='SELECT'?40:80,p.width-2)||r.right>innerWidth+1||r.left<0?[{name:e.name||e.id,width:r.width,left:r.left,right:r.right}]:[];
    }));
    assert(!bad.length,`${path} controls at ${width}, collapsed=${collapsed}, revealed=${revealed}: ${JSON.stringify(bad)}`);
   }
   await page.locator('#content details').evaluateAll(es=>es.forEach(e=>e.open=false));
   if(path==='/chat')await page.locator('#messages').evaluate(e=>e.innerHTML='<div class="message"><span class="you">Sarah <span class="msg-when">Today, 10:30</span></span><p>Can we meet tomorrow afternoon?</p></div><div class="message"><span class="you">You <span class="msg-when">Today, 10:32</span></span><p>Yes, see you at two.</p></div>');
   const narrowTextareas=await page.locator('form.form:not(.form-inline) > textarea').evaluateAll(es=>es.filter(e=>e.getClientRects().length).filter(e=>{const f=e.parentElement,style=getComputedStyle(f);return e.getBoundingClientRect().width<f.clientWidth-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight)-2}).map(e=>e.name));
   assert(!narrowTextareas.length,`full-width form fields are narrow: ${narrowTextareas}`);
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
    const rows=await items.locator('a').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().y));
    assert(rows.length<2||rows[1]>rows[0],'item actions run together');
    await page.keyboard.press('Escape');assert(!await items.isVisible(),'escape did not close item actions');
   }
   if(path==='/account') {
    const links=page.locator('[aria-label="Connection settings"]');
    assert.deepEqual(await links.locator('a').allTextContents(),['API credentials','Mail settings']);
    const previous=await links.evaluate(e=>e.previousElementSibling.getBoundingClientRect().bottom),box=await links.boundingBox();
    assert(box.y-previous<=24,'account connections have excessive spacing');
   }
   if(path==='/blog?write=true') {
    const colors=await page.locator('.form-actions > *').evaluateAll(es=>es.map(e=>getComputedStyle(e).backgroundColor));
    assert.notEqual(colors[0],colors[1],'primary and secondary actions look identical');
   }
   if(path==='/login'||path==='/signup') {
    const box=await page.locator(path==='/login'?'#login':'#signup').boundingBox();assert(Math.abs(box.x+box.width/2-width/2)<2,'auth form off center');
    assert.equal(await page.locator('.oauth-btn').count(),1,'Google sign-in fixture missing');
    assert.equal(await page.locator('.oauth-btn').evaluate(e=>getComputedStyle(e).display),'flex','Google button unformatted');
   }
   if(path==='/?new=1'&&width>900&&!collapsed) {
    assert(await page.locator('#nav .chat-sess').count()>=30,'long history fixture missing');
    const bottom=await page.locator('#nav-services').boundingBox();assert(bottom.y+bottom.height<900,'history pushed destinations off screen');
    assert(await page.locator('.nav-history .chat-sess-list').evaluate(e=>e.scrollHeight>e.clientHeight),'history must scroll independently');
    assert((await page.locator('#nav .chat-sess').first().boundingBox()).height>=40,'history rows collapsed');
   }
   if(await page.locator('.conversation-toolbar').count()) {
    const toolbar=await page.locator('.conversation-toolbar').boundingBox(),prompt=await page.locator('#mu-chat-form').boundingBox();
    assert(Math.abs(toolbar.x-prompt.x)<2&&Math.abs(toolbar.width-prompt.width)<2,'toolbar and prompt differ in alignment');
    const controls=await page.locator('.conversation-toolbar > *').evaluateAll(es=>es.filter(e=>e.getClientRects().length).map(e=>e.getBoundingClientRect().y));
    assert(Math.max(...controls)-Math.min(...controls)<2,'conversation toolbar wraps');
   }
   if(process.env.MU_LAYOUT_SHOTS&&[390,1440].includes(width)&&!collapsed){
    fs.mkdirSync(process.env.MU_LAYOUT_SHOTS,{recursive:true});await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/'+(path.replace(/[^a-zA-Z0-9_-]/g,'-')||'landing')+'-'+width+'-'+collapsed+'.png',fullPage:true});
   }
   if(path==='/'||path==='/?new=1') {
    assert.equal(await page.locator('#mu-chat-input').count(),1,'one composer');
    if(path==='/?new=1'&&(width<=900||collapsed)){
     await page.locator('.conversation-switcher summary').click();
     assert(await page.locator('.conversation-menu .chat-sess').count()>0,'history lists conversations');
     assert(await page.locator('.conversation-menu').isVisible(),'history visible');
     if(process.env.MU_LAYOUT_SHOTS&&width===390)await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/history-open.png'});
     await page.keyboard.press('Escape');
     assert(!await page.locator('.conversation-menu').isVisible(),'escape closes history');
    }

    assert.equal(await page.locator('#tabs,#home-personal,#home-feed').count(),0,'retired shell');
    const form=page.locator('#mu-chat-form');
    const empty=await form.boundingBox();
    if(path==='/')assert(empty.y>220&&empty.y<620,'public empty prompt is not centered');
    else {const nav=await page.locator('#mobile-nav').isVisible()?await page.locator('#mobile-nav').boundingBox():null;const bottom=nav?nav.y:900;assert(bottom-empty.y-empty.height<=24,'signed-in prompt is not at the bottom');}
    await page.locator('#mu-chat-mic').click();const listening=await form.boundingBox();assert(Math.abs(empty.y-listening.y)<2,'dictation moved prompt');
    assert.equal(await page.locator('#mu-chat-mic').getAttribute('aria-pressed'),'true');
    if(process.env.MU_LAYOUT_SHOTS&&width===390&&path==='/?new=1')await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/dictating.png'});
    await page.locator('#mu-chat-mic').click();
    await page.locator('#mu-chat-input').fill('Find fruit names');await page.locator('#mu-chat-form button[type=submit]').click();
    await page.waitForTimeout(100);assert(!await page.locator('.mu-agent').textContent().then(s=>s.includes('**')||s.includes('# Fruit')),'raw Markdown flashed');
    await page.waitForSelector('.mu-agent h2');
    assert.equal(await page.locator('.mu-agent strong').textContent(),'Arabic');
    if(process.env.MU_LAYOUT_SHOTS&&width===390&&path==='/?new=1')await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/formatted-answer.png'});
    if(path==='/?new=1')assert.equal(await page.locator('#conversation-delete').count(),1,'new conversation has no delete action');
    const before=await form.boundingBox();
    await page.locator('#mu-chat-conv').evaluate(e=>e.innerHTML='<p>A long answer</p>'.repeat(100));
    await page.waitForTimeout(50);const after=await form.boundingBox();
    assert(Math.abs(before.y-after.y)<2,'composer moved as conversation grew');
    const nav=await page.locator('#mobile-nav').count()?await page.locator('#mobile-nav').boundingBox():null;
    assert(after.y+after.height<=(nav?nav.y:900),'navigation covers composer');
    const center=await page.locator('#mu-chat').evaluate(e=>{const r=e.getBoundingClientRect();const available=document.body.classList.contains('index-shell')||innerWidth<=900||document.body.classList.contains('nav-collapsed')?0:220;return Math.abs((r.left+r.right)/2-(available+innerWidth)/2)});
    assert(center<2,`conversation offset ${center}px at ${width}`);
    if(width<900){await page.evaluate(()=>{const v=new EventTarget();v.height=430;v.offsetTop=0;Object.defineProperty(window,'visualViewport',{configurable:true,value:v});window.dispatchEvent(new Event('resize'));});await page.waitForTimeout(70);const box=await form.boundingBox();assert(box.y+box.height<=430,`keyboard covers composer: ${JSON.stringify(box)}`);}
    assert(!errors.length,`conversation errors: ${errors.join(', ')}`);
   }
   if(path==='/components') {
    const styles=await page.evaluate(()=>{const s=e=>getComputedStyle(document.querySelector(e));return {border:s('.card').borderTopWidth,padding:s('.card').paddingLeft,preview:s('.collection-item').textDecorationLine,notice:s('.notice').backgroundColor,thumb:document.querySelector('.thumbnail img').getBoundingClientRect().width}});
    assert.equal(styles.border,'1px','cards have boundaries');assert.equal(styles.padding,'16px','cards have space');assert.equal(styles.preview,'none','collection previews are readable');assert.notEqual(styles.notice,'rgba(0, 0, 0, 0)','notice has a background');assert(styles.thumb<=320,'bounded thumbnails');
   }
   if(path==='/inbox')assert.equal(await page.locator('article.message').count(),1,'priority should show one communication');
   if(path==='/services?view=feed'){await page.waitForSelector('#service-feed .section-card');assert.equal(await page.locator('#mu-chat-input').count(),0);}
   if(path==='/chat'){
    await page.locator('#messages').evaluate(e=>e.innerHTML='<p>Room message</p>'.repeat(100));await page.waitForTimeout(70);
    const box=await page.locator('#chat-form').boundingBox();assert(box.y+box.height<=900,`room composer below viewport at ${width}`);
   }
   } catch(e) { failures.push(path+' at '+width+': '+e.message); }
  }
 }
 assert(!failures.length,failures.join('\n'));
 } finally {await browser.close();}
 console.log('All service pages fit; conversation composers remain centered and stable on mobile and desktop.');
})().catch(e=>{console.error(e);process.exitCode=1});
