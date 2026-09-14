const fs=require('fs'),assert=require('assert');
const {chromium}=require(process.env.MU_PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const input=JSON.parse(fs.readFileSync(0,'utf8'));
 const browser=await chromium.launch({executablePath:process.env.MU_LAYOUT_BROWSER,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 try {
 const page=await browser.newPage();
 page.setDefaultTimeout(5000);
 const failures=[];
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/*',route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/services'&&route.request().headers().accept?.includes('application/json'))return route.fulfill({contentType:'application/json',body:JSON.stringify({html:'<section class="section-card">Cached service feed</section>',loading:false})});
  if(u.hostname==='www.youtube.com')return route.fulfill({body:'<!doctype html><p>Video fixture</p>',contentType:'text/html'});
  if(u.pathname==='/composition.css')return route.fulfill({contentType:'text/css',body:input.composition});
  if(u.pathname==='/mu.css')return route.fulfill({contentType:'text/css',body:input.css});
  if(['/mu.js','/shell.js','/viewport.js'].includes(u.pathname))return route.fulfill({body:fs.readFileSync('../internal/app/html'+u.pathname),contentType:'application/javascript'});
  const html=input.pages[u.pathname+u.search]||input.pages[u.pathname];
  if(html)return route.fulfill({contentType:'text/html; charset=utf-8',body:html});
  if(/\.(png|svg)$/.test(u.pathname)){const p='../internal/app/html'+u.pathname;if(fs.existsSync(p))return route.fulfill({body:fs.readFileSync(p),contentType:u.pathname.endsWith('.svg')?'image/svg+xml':'image/png'});}
  return route.fulfill({status:200,body:'',contentType:'text/plain'});
 });
 for(const width of [320,390,768,1024,1440])for(const collapsed of (width>900?[false,true]:[false])){
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
   if(path==='/'||path==='/?new=1') {
    assert.equal(await page.locator('#mu-chat-input').count(),1,'one composer');
    assert.equal(await page.locator('#tabs,#home-personal,#home-feed').count(),0,'retired shell');
    const form=page.locator('#mu-chat-form'),before=await form.boundingBox();
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
   if(path==='/inbox')assert.equal(await page.locator('article.message').count(),1,'priority should show one communication');
   if(path==='/services?view=feed'){await page.waitForSelector('#service-feed .section-card');assert.equal(await page.locator('#mu-chat-input').count(),0);}
   if(path==='/chat'){
    await page.locator('#messages').evaluate(e=>e.innerHTML='<p>Room message</p>'.repeat(100));await page.waitForTimeout(70);
    const box=await page.locator('#chat-form').boundingBox();assert(box.y+box.height<=900,`room composer below viewport at ${width}`);
   }
   if(process.env.MU_LAYOUT_SHOTS&&['/','/?new=1','/services','/services?view=feed','/work','/inbox','/agents','/mail','/video'].includes(path)){
    fs.mkdirSync(process.env.MU_LAYOUT_SHOTS,{recursive:true});await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/'+(path.replace(/[^a-zA-Z0-9_-]/g,'-')||'landing')+'-'+width+'-'+collapsed+'.png',fullPage:true});
   }
   } catch(e) { failures.push(path+' at '+width+': '+e.message); }
  }
 }
 assert(!failures.length,failures.join('\n'));
 } finally {await browser.close();}
 console.log('All service pages fit; conversation composers remain centered and stable on mobile and desktop.');
})().catch(e=>{console.error(e);process.exitCode=1});
