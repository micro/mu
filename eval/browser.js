// Load a page and use it, so an eval can ask whether it works rather than
// whether it parses.
//
// Deliberately blunt about how it drives the page: it fills every input in
// order with the values it was given, fires the events a person's typing would
// fire, clicks any buttons, and hands back what the page then says. Nothing
// here knows the ids or the structure, because the point is to judge a page
// nobody specified the markup of — two models asked for a tip calculator will
// not agree on a single selector, and an eval that demands one is testing
// whether they guessed the same names.
//
// Usage: node browser.js <file> <json-array-of-values>
const path = require('path');
const root = process.env.MU_EVAL_BROWSER;
const { chromium } = require(path.join(root, 'node_modules', 'playwright'));

(async () => {
  const file = process.argv[2];
  const values = JSON.parse(process.argv[3] || '[]');
  const b = await chromium.launch({
    executablePath: process.env.MU_EVAL_CHROMIUM || '/opt/pw-browsers/chromium',
  });
  const p = await b.newPage();

  const errors = [];
  p.on('pageerror', e => errors.push(String(e)));
  p.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });

  await p.goto('file://' + file, { waitUntil: 'load' });
  await p.evaluate(vals => {
    const inputs = [...document.querySelectorAll('input, select')]
      .filter(i => !['button', 'submit', 'checkbox', 'radio'].includes(i.type));
    inputs.forEach((el, i) => {
      if (i >= vals.length) return;
      el.value = String(vals[i]);
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
    });
    // Some pages compute on a button rather than on input.
    document.querySelectorAll('button, input[type=submit]').forEach(bt => {
      const t = (bt.textContent || bt.value || '').toLowerCase();
      if (!/reset|clear|delete|remove/.test(t)) bt.click();
    });
  }, values);
  await p.waitForTimeout(250);

  const text = await p.evaluate(() => document.body.innerText.replace(/\s+/g, ' ').trim());
  console.log(JSON.stringify({ text, errors }));
  await b.close();
})().catch(e => {
  console.log(JSON.stringify({ text: '', errors: ['harness: ' + String(e)] }));
  process.exit(0);
});
