// Load a page and use it, so an eval can ask whether it works rather than
// whether it parses.
//
// Fields are found by what they are called, not by where they are. Filling in
// order looked simpler and marked a correct page wrong: asked to add a people
// field, the model put it second, so 20 went into "Number of People" and the
// eval failed a page that was doing exactly what it was told. Two models asked
// for the same app will not agree on field order any more than they agree on
// selectors, and neither is a defect.
//
// So each value carries a word to find its field by — bill, tip, people — and
// that is matched against the label, the placeholder, the name and the id.
// Nothing here knows the markup, which is the point: the page being judged is
// one nobody specified.
//
// Usage: node browser.js <file> <json-array-of-[match,value]-pairs>
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
  await p.evaluate(pairs => {
    const inputs = [...document.querySelectorAll('input, select')]
      .filter(i => !['button', 'submit', 'checkbox', 'radio'].includes(i.type));

    // Everything this field is called, in one string to search.
    const describe = el => {
      const bits = [el.name, el.id, el.placeholder, el.getAttribute('aria-label') || ''];
      if (el.id) {
        const lab = document.querySelector('label[for="' + CSS.escape(el.id) + '"]');
        if (lab) bits.push(lab.textContent);
      }
      const wrapping = el.closest('label');
      if (wrapping) bits.push(wrapping.textContent);
      return bits.join(' ').toLowerCase();
    };

    const taken = new Set();
    for (const [match, value] of pairs) {
      const want = String(match).toLowerCase();
      const el = inputs.find(i => !taken.has(i) && describe(i).includes(want));
      if (!el) continue;
      taken.add(el);
      el.value = String(value);
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }
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
