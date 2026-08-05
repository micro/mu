const { chromium } = require('/opt/node22/lib/node_modules/playwright');
(async () => {
  const b = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' });
  const ctx = await b.newContext({ viewport:{width:390,height:800}, isMobile:true, hasTouch:true });
  const p = await ctx.newPage();
  await p.goto('http://localhost:8080/login',{waitUntil:'domcontentloaded'});
  await p.fill('#id','secondone'); await p.fill('#secret','testpass1234');
  await p.click('form button >> nth=0'); await p.waitForTimeout(1500);
  await p.goto('http://localhost:8080/home',{waitUntil:'domcontentloaded'});
  await p.waitForTimeout(800);
  await p.click('#menu-toggle'); await p.waitForTimeout(700);
  console.log(JSON.stringify(await p.evaluate(() => {
    const nc=document.getElementById('nav-container'), nb=document.querySelector('.nav-bottom');
    const r=nc.getBoundingClientRect(), rb=nb.getBoundingClientRect();
    const last=[...document.querySelectorAll('#nav > a')].pop().getBoundingClientRect();
    return {panelH:Math.round(r.height), lastItemBottom:Math.round(last.bottom),
            navBottomTop:Math.round(rb.top), navBottomBottom:Math.round(rb.bottom),
            gap:Math.round(rb.top-last.bottom), viewportH:window.innerHeight};
  })));
  await p.screenshot({ path:'mob-sidebar.png' });
  await b.close();
})();
