const fs=require('fs'),assert=require('assert');
const {chromium}=require(process.env.MU_PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const input=JSON.parse(fs.readFileSync(0,'utf8'));
 const browser=await chromium.launch({executablePath:process.env.MU_LAYOUT_BROWSER,headless:true,args:['--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--no-zygote','--single-process']});
 const page=await browser.newPage();
 await page.route('**/*',route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/composition.css')return route.fulfill({contentType:'text/css',body:input.composition});
  if(u.pathname==='/mu.css')return route.fulfill({contentType:'text/css',body:input.css});
  const html=input.pages[u.pathname+u.search]||input.pages[u.pathname];
  if(html)return route.fulfill({contentType:'text/html; charset=utf-8',body:html});
  if(u.pathname==='/viewport.js')return route.fulfill({body:fs.readFileSync('../internal/app/html/viewport.js'),contentType:'application/javascript'});
  if(/\.(png|svg)$/.test(u.pathname)){const p='../internal/app/html'+u.pathname;if(fs.existsSync(p))return route.fulfill({body:fs.readFileSync(p),contentType:u.pathname.endsWith('.svg')?'image/svg+xml':'image/png'});}
  return route.fulfill({status:200,body:'',contentType:'text/plain'});
 });
 const gap=async(a,b)=>page.evaluate(([a,b])=>document.querySelector(b).getBoundingClientRect().top-document.querySelector(a).getBoundingClientRect().bottom,[a,b]);
 for(const width of [320,390,768,1024,1440])for(const collapsed of (width>900?[false,true]:[false])){
  await page.setViewportSize({width,height:900});
  for(const path of Object.keys(input.pages).filter(p=>!process.env.MU_LAYOUT_PATHS||process.env.MU_LAYOUT_PATHS.split(',').includes(p.split('?')[0]))){
   await page.goto('https://mu.test'+path);
   await page.evaluate(c=>document.body.classList.toggle('nav-collapsed',c),collapsed);
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`${path} overflows at ${width}`);
   // Exercise collapsed and revealed forms. Measure actual controls, not CSS
   // declarations, so a later rule or a surrounding grid cannot hide a regression.
   for(const revealed of [false,true]) {
    if(revealed) await page.locator('details').evaluateAll(es=>es.forEach(e=>e.open=true));
    const bad=await page.evaluate(()=>Array.from(document.querySelectorAll('#content input, #content select, #content textarea')).flatMap(e=>{
     if(['hidden','checkbox','radio','submit','button','range','color'].includes(e.type)||!e.getClientRects().length||e.closest('#chat-form,#mu-chat-form'))return [];
     const r=e.getBoundingClientRect(),p=e.parentElement.getBoundingClientRect();
     if(r.width<Math.min(e.tagName==='SELECT'?40:80,p.width-2)||r.right>innerWidth+1||r.left<0)return [{name:e.name||e.id,width:r.width,left:r.left,right:r.right}];
     return [];
    }));
    assert(!bad.length,`${path} squeezed/overflowing controls at ${width}, sidebar=${!collapsed}, revealed=${revealed}: ${JSON.stringify(bad)}`);
   }
   // Reset disclosures before the interaction-specific checks below.
   await page.locator('details').evaluateAll(es=>es.forEach(e=>e.open=false));
   if(path==='/sms') {
    assert(await page.locator('.sms-conversation').count()===2,'SMS and WhatsApp merged in list');
    assert(await page.locator('input[name=text]').count()===0,'reply boxes leaked onto list');
   }
   if(path.startsWith('/sms?id=')) {
    assert(await page.locator('input[name=channel]').inputValue()==='whatsapp','reply channel changed');
    assert(await page.locator('input[name=text]').count()===1,'expected one conversation composer');
    assert(!await page.locator('#content').textContent().then(t=>t.includes('Latest SMS message')),'other channel appeared in thread');
   }
   if(path==='/home')assert(await page.evaluate(()=>!window.muActiveAgent),'Home inherited another page agent');
   if(path==='/'||path==='/home')assert(await page.locator('.shortcuts,[data-shortcut]').count()===0,'shortcut panels remain');
   if(path==='/agent/new'||path==='/token'){
    const select=path==='/token'?'#tok-scope':'#b-scope',list=path==='/token'?'#tok-service-list':'#b-service-list';
    assert(await page.locator(list).isHidden());
    await page.selectOption(select,'select');
    assert(await page.locator(list).isVisible());
    const space=await gap(select,list);assert(space>=7&&space<=9,`${path} select gap ${space}`);
    const bottom=await gap(list,'.form-actions');assert(bottom>=15&&bottom<=17,`${path} actions gap ${bottom}`);
    const checks=page.locator(list+' input');await page.locator(list+' .choice').first().click();
    await page.selectOption(select,'all');assert(await page.locator(list).isHidden());
    await page.selectOption(select,'select');assert(await checks.first().isChecked());
   }
   if(path==='/chat'){
    await page.locator('#messages').evaluate(e=>e.innerHTML='<p>Long conversation</p>'.repeat(100));
    await page.waitForTimeout(100);
    const bounds=await page.evaluate(()=>{const form=document.getElementById('chat-form').getBoundingClientRect();const tabs=document.getElementById('tabs');const end=tabs&&getComputedStyle(tabs).display!=='none'?tabs.getBoundingClientRect().top:innerHeight;return {bottom:form.bottom,end,scroll:scrollY,height:document.documentElement.scrollHeight};});
    assert(bounds.bottom<=bounds.end-4,`room closed keyboard width=${width}: ${JSON.stringify(bounds)}`);
   }
   if(width<900&&(path==='/chat'||path==='/agent/micro')){
    const input=path==='/chat'?'#prompt':'#mu-chat-input';
    const composer=path==='/chat'?'#chat-form':'#mu-chat-form';
    const messages=path==='/chat'?'#messages':'#mu-chat-conv';
    await page.locator(messages).evaluate(e=>e.innerHTML='<p>Long conversation</p>'.repeat(100));
    await page.locator(input).focus();
    await page.evaluate(()=>{
     const viewport=new EventTarget();viewport.height=430;viewport.offsetTop=0;
     Object.defineProperty(window,'visualViewport',{configurable:true,value:viewport});
     window.dispatchEvent(new Event('resize'));
    });
    await page.waitForTimeout(100);
    const bottom=await page.locator(composer).evaluate(e=>e.getBoundingClientRect().bottom);

    assert(bottom<=430&&bottom>=410,`${path} at ${width}: keyboard composer bottom ${bottom}`);
    assert(await page.evaluate(()=>scrollY===0),`${path} page scrolled with keyboard`);
   }
   if(path==='/tasks'){
    for(const button of await page.locator('.task .form-actions button').all()){assert(await button.evaluate(e=>getComputedStyle(e).borderStyle!=='none'),'task action has no border');}
   }
   if(path==='/events'){
    await page.locator('.disclosure summary').click();
    assert(await gap('.disclosure summary','.disclosure form')>=15);
   }
   if(process.env.MU_LAYOUT_SHOTS){fs.mkdirSync(process.env.MU_LAYOUT_SHOTS,{recursive:true});await page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/'+(path.replace(/[^a-zA-Z0-9_-]/g,'-')||'landing')+'-'+width+'-'+collapsed+'.png',fullPage:true});}
  }
 }
 // Agent deep links must not change Home, including soft navigation.
 await page.goto('https://mu.test/agent/micro');
 await page.evaluate(()=>{window.muSeedAgent('another-agent');sessionStorage.setItem('mu_active_agent','another-agent');const a=document.createElement('a');a.href='/home';document.getElementById('content').appendChild(a);a.click();});
 await page.waitForURL('**/home');
 await page.waitForTimeout(100);
 assert(await page.evaluate(()=>window.muActiveAgent===''),'Home retained the named agent after navigation');
 // A stale guest conversation from an older deployment must not reappear.
 await page.goto('https://mu.test/');
 await page.evaluate(()=>sessionStorage.setItem('mu_chat_conv:landing','<div class="mu-user">stale question</div>'));
 await page.reload();assert(!await page.locator('#mu-chat-conv').textContent().then(t=>t.includes('stale question')));
 await browser.close();console.log('Desktop/sidebar and mobile form, reveal, spacing and overflow checks passed.');
})().catch(e=>{console.error(e);process.exit(1)});
