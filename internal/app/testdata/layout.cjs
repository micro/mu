const fs=require('fs'),assert=require('assert');
const {chromium}=require(process.env.MU_PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const input=JSON.parse(fs.readFileSync(0,'utf8'));
 const browser=await chromium.launch({executablePath:process.env.MU_LAYOUT_BROWSER,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 try{
 const page=await browser.newPage();page.setDefaultTimeout(5000);
 let calls=[],models=0,lastThread='';const errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/*',async route=>{
  const r=route.request(),u=new URL(r.url());
  if(u.pathname==='/command'){
   const body=r.postDataJSON();calls.push(body.command);
   if(body.command.startsWith('admin config set'))return route.fulfill({contentType:'application/json',body:JSON.stringify({html:'<p>Saved.</p>'})});
   return route.fulfill({contentType:'application/json',body:JSON.stringify(input.responses[body.command]||{assistant:true})});
  }
  if(u.pathname==='/agent'&&r.method()==='POST'){
   models++;lastThread=r.postDataJSON().context_id;
   return route.fulfill({contentType:'text/event-stream',body:'data: '+JSON.stringify({type:'flow_id',thread:'conversation-fixture'})+'\n\ndata: '+JSON.stringify({type:'response',html:'<h2>Answer</h2><p>A formatted response.</p>'})+'\n\n'});
  }
  const html=input.pages[u.pathname+u.search]||input.pages[u.pathname];
  if(html)return route.fulfill({contentType:'text/html',body:html});
  return route.fulfill({status:404,body:''});
 });
 function screenshot(name,width){if(!process.env.MU_LAYOUT_SHOTS)return;fs.mkdirSync(process.env.MU_LAYOUT_SHOTS,{recursive:true});return page.screenshot({path:process.env.MU_LAYOUT_SHOTS+'/'+name+'-'+width+'.png',fullPage:true});}
 for(const width of [320,390,768,1440]){
  await page.setViewportSize({width,height:900});
  for(const path of Object.keys(input.pages)){
   calls=[];models=0;await page.goto('https://mu.test'+path);
   assert.equal(await page.locator('main').count(),1,'one content surface');
   assert.equal(await page.locator('#nav-container,#menu-toggle').count(),0,'old navigation remains');
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page overflows: '+path);
   assert.equal(calls.length,0,'arrival executed a command');assert.equal(models,0,'arrival called a model');
   assert.equal(await page.locator('footer a[href="/privacy"]').count(),1,'footer missing');
   if(['/','/home'].includes(path)){
    assert.equal(await page.locator('#command-input').count(),1);
    const main=await page.locator('main').boundingBox(),form=await page.locator('#command-form').boundingBox();assert(Math.abs(main.width-form.width)<2,'composer width drift');
    for(const command of ['help','inbox','brief','admin logs']){
     await page.locator('#command-input').fill(command);await page.locator('#send').click();await page.waitForFunction(()=>!document.querySelector('#send').disabled);
     assert.equal(models,0,'simple command called model');
    }
    await page.locator('#command-input').fill('admin config set OPENAI_API_KEY hidden-test-value');await page.locator('#send').click();await page.waitForFunction(()=>!document.querySelector('#send').disabled);
    assert(!(await page.locator('#responses').textContent()).includes('hidden-test-value'),'credential echoed');
    await page.locator('#command-input').fill('Compare the results');await page.locator('#send').click();await page.waitForFunction(()=>!document.querySelector('#send').disabled);
    assert.equal(models,1);assert(await page.locator('.answer h2').count()>0,'formatted answer missing');assert(lastThread,'service conversation lost');
    const send=await page.locator('#send').boundingBox();assert(send.width<130,'oversized button');
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'results overflow');
    await screenshot(path==='/'?'command-results':'account-results',width);
   }else if(path.startsWith('/?session=')){
    assert((await page.locator('#responses').textContent()).includes('My saved question'),'saved conversation missing');
   }else{await screenshot(path.slice(1),width);}
   assert.equal(errors.length,0,errors.join('\n'));
  }
 }
 console.log('Command, conversation, admin, authentication and footer layouts passed at 320, 390, 768 and 1440 pixels.');
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exit(1)});
