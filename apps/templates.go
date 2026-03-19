package apps

// Template represents a starter template for building apps.
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	HTML        string `json:"html"`
}

// Templates is the list of built-in starter templates.
var Templates = []Template{
	{
		ID:          "blank",
		Name:        "Blank",
		Description: "Empty canvas — start from scratch",
		Category:    "Other",
		HTML:        templateBlank,
	},
	{
		ID:          "timer",
		Name:        "Timer",
		Description: "Countdown timer with start/pause/reset",
		Category:    "Productivity",
		HTML:        templateTimer,
	},
	{
		ID:          "calculator",
		Name:        "Calculator",
		Description: "Simple calculator with basic operations",
		Category:    "Tools",
		HTML:        templateCalculator,
	},
	{
		ID:          "tracker",
		Name:        "Tracker",
		Description: "Track items with a counter — habits, streaks, anything",
		Category:    "Productivity",
		HTML:        templateTracker,
	},
	{
		ID:          "converter",
		Name:        "Converter",
		Description: "Convert between units — temperature, weight, distance",
		Category:    "Tools",
		HTML:        templateConverter,
	},
	{
		ID:          "flashcards",
		Name:        "Flashcards",
		Description: "Study flashcards — click to flip, arrow keys to navigate",
		Category:    "Education",
		HTML:        templateFlashcards,
	},
	{
		ID:          "notes",
		Name:        "Notes",
		Description: "Quick notes with local persistence via the SDK",
		Category:    "Productivity",
		HTML:        templateNotes,
	},
	{
		ID:          "ai-tool",
		Name:        "AI Tool",
		Description: "An AI-powered tool using mu.ai() — summariser, translator, etc.",
		Category:    "Tools",
		HTML:        templateAITool,
	},
}

// GetTemplate returns a template by ID.
func GetTemplate(id string) *Template {
	for i := range Templates {
		if Templates[i].ID == id {
			return &Templates[i]
		}
	}
	return nil
}

// All templates use mu's card styling conventions:
// - Clean, minimal design
// - System font stack (inherited from mu.css)
// - Card-like containers with subtle borders and shadows
// - Consistent spacing and border-radius

const templateBlank = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 16px; color: #333; background: #fff; }
h1 { font-size: 20px; font-weight: 600; margin-bottom: 12px; }
p { color: #666; line-height: 1.5; }
</style>
</head>
<body>
<h1>My App</h1>
<p>Start building here.</p>
</body>
</html>`

const templateTimer = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; color: #333; background: #fff; display: flex; flex-direction: column; align-items: center; }
.timer { font-size: 64px; font-weight: 700; margin: 32px 0; font-variant-numeric: tabular-nums; }
.controls { display: flex; gap: 12px; }
button { padding: 10px 24px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 15px; font-family: inherit; }
button:hover { background: #f5f5f5; }
button.primary { background: #000; color: #fff; border-color: #000; }
button.primary:hover { background: #333; }
.presets { margin-top: 24px; display: flex; gap: 8px; }
.presets button { padding: 6px 16px; font-size: 13px; }
</style>
</head>
<body>
<h2>Timer</h2>
<div class="timer" id="display">25:00</div>
<div class="controls">
  <button class="primary" id="toggle" onclick="toggle()">Start</button>
  <button onclick="reset()">Reset</button>
</div>
<div class="presets">
  <button onclick="setTime(5)">5m</button>
  <button onclick="setTime(15)">15m</button>
  <button onclick="setTime(25)">25m</button>
  <button onclick="setTime(60)">60m</button>
</div>
<script>
var seconds = 1500, interval = null, running = false;
function render() {
  var m = Math.floor(seconds / 60), s = seconds % 60;
  document.getElementById('display').textContent = (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s;
}
function toggle() {
  if (running) { clearInterval(interval); running = false; document.getElementById('toggle').textContent = 'Start'; }
  else { interval = setInterval(function() { if (--seconds <= 0) { clearInterval(interval); running = false; seconds = 0; render(); document.getElementById('toggle').textContent = 'Start'; } render(); }, 1000); running = true; document.getElementById('toggle').textContent = 'Pause'; }
}
function reset() { clearInterval(interval); running = false; seconds = 1500; render(); document.getElementById('toggle').textContent = 'Start'; }
function setTime(m) { clearInterval(interval); running = false; seconds = m * 60; render(); document.getElementById('toggle').textContent = 'Start'; }
render();
</script>
</body>
</html>`

const templateCalculator = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; display: flex; justify-content: center; }
.calc { width: 280px; }
.display { background: #f8f8f8; border: 1px solid #e0e0e0; border-radius: 6px; padding: 16px; text-align: right; font-size: 28px; font-weight: 600; margin-bottom: 12px; min-height: 60px; word-break: break-all; font-variant-numeric: tabular-nums; }
.grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
.grid button { padding: 14px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 18px; font-family: inherit; }
.grid button:hover { background: #f5f5f5; }
.grid button.op { background: #f0f0f0; font-weight: 600; }
.grid button.eq { background: #000; color: #fff; }
.grid button.eq:hover { background: #333; }
.grid button.wide { grid-column: span 2; }
</style>
</head>
<body>
<div class="calc">
  <div class="display" id="display">0</div>
  <div class="grid">
    <button onclick="clear_()">C</button><button class="op" onclick="input('(')">(</button><button class="op" onclick="input(')')">)</button><button class="op" onclick="input('/')">÷</button>
    <button onclick="input('7')">7</button><button onclick="input('8')">8</button><button onclick="input('9')">9</button><button class="op" onclick="input('*')">×</button>
    <button onclick="input('4')">4</button><button onclick="input('5')">5</button><button onclick="input('6')">6</button><button class="op" onclick="input('-')">−</button>
    <button onclick="input('1')">1</button><button onclick="input('2')">2</button><button onclick="input('3')">3</button><button class="op" onclick="input('+')">+</button>
    <button class="wide" onclick="input('0')">0</button><button onclick="input('.')">.</button><button class="eq" onclick="calc()">=</button>
  </div>
</div>
<script>
var expr = '';
function input(v) { expr += v; document.getElementById('display').textContent = expr; }
function clear_() { expr = ''; document.getElementById('display').textContent = '0'; }
function calc() { try { var r = Function('"use strict"; return (' + expr + ')')(); document.getElementById('display').textContent = r; expr = String(r); } catch(e) { document.getElementById('display').textContent = 'Error'; expr = ''; } }
</script>
</div>
</body>
</html>`

const templateTracker = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.item { display: flex; align-items: center; justify-content: space-between; padding: 12px; border: 1px solid #e0e0e0; border-radius: 6px; margin-bottom: 8px; }
.item-name { font-weight: 500; }
.item-count { font-size: 24px; font-weight: 700; font-variant-numeric: tabular-nums; }
.item-controls { display: flex; gap: 8px; align-items: center; }
.item-controls button { width: 32px; height: 32px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 18px; display: flex; align-items: center; justify-content: center; }
.item-controls button:hover { background: #f5f5f5; }
.add-form { display: flex; gap: 8px; margin-top: 16px; }
.add-form input { flex: 1; padding: 8px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; }
.add-form button { padding: 8px 16px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; }
</style>
</head>
<body>
<h2>Tracker</h2>
<div id="items"></div>
<div class="add-form">
  <input type="text" id="name" placeholder="Add item to track...">
  <button onclick="addItem()">Add</button>
</div>
<script>
var items = [{ name: 'Water (glasses)', count: 0 }, { name: 'Exercise (minutes)', count: 0 }];
function render() {
  var html = '';
  items.forEach(function(item, i) {
    html += '<div class="item"><span class="item-name">' + item.name + '</span><div class="item-controls"><button onclick="dec(' + i + ')">-</button><span class="item-count">' + item.count + '</span><button onclick="inc(' + i + ')">+</button></div></div>';
  });
  document.getElementById('items').innerHTML = html;
}
function inc(i) { items[i].count++; render(); }
function dec(i) { if (items[i].count > 0) items[i].count--; render(); }
function addItem() { var n = document.getElementById('name'); if (n.value.trim()) { items.push({ name: n.value.trim(), count: 0 }); n.value = ''; render(); } }
render();
</script>
</body>
</html>`

const templateConverter = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 400px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.tabs { display: flex; gap: 8px; margin-bottom: 20px; flex-wrap: wrap; }
.tabs button { padding: 6px 16px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 13px; font-family: inherit; }
.tabs button.active { background: #000; color: #fff; border-color: #000; }
.field { margin-bottom: 12px; }
.field label { display: block; font-size: 13px; color: #666; margin-bottom: 4px; }
.field input { width: 100%; padding: 10px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-size: 16px; font-family: inherit; }
.result { padding: 16px; background: #f8f8f8; border-radius: 6px; font-size: 20px; font-weight: 600; text-align: center; margin-top: 8px; }
</style>
</head>
<body>
<h2>Converter</h2>
<div class="tabs" id="tabs"></div>
<div id="fields"></div>
<div class="result" id="result">—</div>
<script>
var conversions = {
  'Temp': { from: '°C', to: '°F', convert: function(v) { return (v * 9/5 + 32).toFixed(1) + ' °F'; }, reverse: function(v) { return ((v - 32) * 5/9).toFixed(1) + ' °C'; } },
  'Weight': { from: 'kg', to: 'lb', convert: function(v) { return (v * 2.20462).toFixed(2) + ' lb'; }, reverse: function(v) { return (v / 2.20462).toFixed(2) + ' kg'; } },
  'Distance': { from: 'km', to: 'mi', convert: function(v) { return (v * 0.621371).toFixed(2) + ' mi'; }, reverse: function(v) { return (v / 0.621371).toFixed(2) + ' km'; } },
  'Volume': { from: 'L', to: 'gal', convert: function(v) { return (v * 0.264172).toFixed(2) + ' gal'; }, reverse: function(v) { return (v / 0.264172).toFixed(2) + ' L'; } }
};
var current = 'Temp';
function init() {
  var tabsHTML = '';
  Object.keys(conversions).forEach(function(k) {
    tabsHTML += '<button class="' + (k === current ? 'active' : '') + '" onclick="switchTab(\'' + k + '\')">' + k + '</button>';
  });
  document.getElementById('tabs').innerHTML = tabsHTML;
  var c = conversions[current];
  document.getElementById('fields').innerHTML = '<div class="field"><label>From (' + c.from + ')</label><input type="number" id="fromVal" oninput="convert()" placeholder="Enter value"></div>';
}
function switchTab(k) { current = k; init(); document.getElementById('result').textContent = '—'; }
function convert() { var v = parseFloat(document.getElementById('fromVal').value); if (isNaN(v)) { document.getElementById('result').textContent = '—'; return; } document.getElementById('result').textContent = conversions[current].convert(v); }
init();
</script>
</body>
</html>`

const templateFlashcards = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; display: flex; flex-direction: column; align-items: center; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.card { width: 320px; min-height: 200px; border: 1px solid #e0e0e0; border-radius: 6px; display: flex; align-items: center; justify-content: center; padding: 32px; cursor: pointer; text-align: center; font-size: 18px; transition: box-shadow 0.15s; user-select: none; }
.card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
.card.flipped { background: #f8f8f8; }
.nav { display: flex; gap: 12px; margin-top: 16px; align-items: center; }
.nav button { padding: 8px 20px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-family: inherit; }
.nav button:hover { background: #f5f5f5; }
.counter { color: #999; font-size: 13px; }
.hint { margin-top: 12px; font-size: 13px; color: #999; }
</style>
</head>
<body>
<h2>Flashcards</h2>
<div class="card" id="card" onclick="flip()"></div>
<div class="nav">
  <button onclick="prev()">← Prev</button>
  <span class="counter" id="counter"></span>
  <button onclick="next()">Next →</button>
</div>
<p class="hint">Click card to flip · Arrow keys to navigate</p>
<script>
var cards = [
  { q: 'What is the speed of light?', a: '299,792,458 m/s' },
  { q: 'What is the chemical symbol for gold?', a: 'Au' },
  { q: 'Who wrote "1984"?', a: 'George Orwell' },
  { q: 'What is the largest planet?', a: 'Jupiter' },
  { q: 'What year was the web invented?', a: '1989 (Tim Berners-Lee)' }
];
var idx = 0, flipped = false;
function render() {
  var el = document.getElementById('card');
  el.textContent = flipped ? cards[idx].a : cards[idx].q;
  el.className = 'card' + (flipped ? ' flipped' : '');
  document.getElementById('counter').textContent = (idx + 1) + ' / ' + cards.length;
}
function flip() { flipped = !flipped; render(); }
function next() { idx = (idx + 1) % cards.length; flipped = false; render(); }
function prev() { idx = (idx - 1 + cards.length) % cards.length; flipped = false; render(); }
document.addEventListener('keydown', function(e) { if (e.key === 'ArrowRight') next(); if (e.key === 'ArrowLeft') prev(); if (e.key === ' ') { e.preventDefault(); flip(); } });
render();
</script>
</body>
</html>`

const templateNotes = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<script src="/apps/sdk.js"></script>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
textarea { width: 100%; min-height: 300px; padding: 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; line-height: 1.6; resize: vertical; }
.toolbar { display: flex; justify-content: space-between; align-items: center; margin-top: 12px; }
button { padding: 8px 20px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; }
.status { font-size: 13px; color: #999; }
</style>
</head>
<body>
<h2>Notes</h2>
<textarea id="editor" placeholder="Start typing..."></textarea>
<div class="toolbar">
  <span class="status" id="status"></span>
  <button onclick="saveNotes()">Save</button>
</div>
<script>
var editor = document.getElementById('editor');
var status = document.getElementById('status');

// Load saved notes on start
mu.store.get('notes').then(function(val) {
  if (val) editor.value = val;
  status.textContent = 'Loaded';
}).catch(function() { status.textContent = 'Ready'; });

function saveNotes() {
  status.textContent = 'Saving...';
  mu.store.set('notes', editor.value).then(function() {
    status.textContent = 'Saved';
  }).catch(function(e) { status.textContent = 'Error: ' + e.message; });
}

// Auto-save every 30 seconds
setInterval(function() { if (editor.value) saveNotes(); }, 30000);
</script>
</body>
</html>`

const templateAITool = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<script src="/apps/sdk.js"></script>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 4px; }
.desc { color: #666; font-size: 14px; margin-bottom: 16px; }
textarea { width: 100%; min-height: 120px; padding: 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; resize: vertical; margin-bottom: 12px; }
button { padding: 10px 24px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; font-size: 15px; }
button:disabled { background: #ccc; cursor: not-allowed; }
.output { margin-top: 16px; padding: 16px; background: #f8f8f8; border-radius: 6px; line-height: 1.6; white-space: pre-wrap; min-height: 60px; }
.output.loading { color: #999; }
</style>
</head>
<body>
<h2>AI Tool</h2>
<p class="desc">Enter text and let AI process it — summarise, translate, explain, or anything else.</p>
<textarea id="input" placeholder="Paste text here..."></textarea>
<div style="display:flex;gap:8px;flex-wrap:wrap;">
  <button onclick="run('Summarise this concisely')">Summarise</button>
  <button onclick="run('Translate this to French')">→ French</button>
  <button onclick="run('Explain this simply')">Explain</button>
  <button onclick="run(prompt('Custom instruction:'))">Custom</button>
</div>
<div class="output" id="output">Results will appear here.</div>
<script>
function run(instruction) {
  if (!instruction) return;
  var text = document.getElementById('input').value;
  if (!text.trim()) { alert('Please enter some text first.'); return; }
  var out = document.getElementById('output');
  out.className = 'output loading';
  out.textContent = 'Thinking...';
  mu.ai(instruction + ':\n\n' + text).then(function(r) {
    out.className = 'output';
    out.textContent = typeof r === 'string' ? r : JSON.stringify(r);
  }).catch(function(e) {
    out.className = 'output';
    out.textContent = 'Error: ' + e.message;
  });
}
</script>
</body>
</html>`
