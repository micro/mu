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
	{
		ID:          "weather",
		Name:        "Weather",
		Description: "Weather forecast using mu.weather() with geolocation",
		Category:    "Data",
		HTML:        templateWeather,
	},
	{
		ID:          "markets",
		Name:        "Markets",
		Description: "Live crypto prices using mu.markets()",
		Category:    "Data",
		HTML:        templateMarkets,
	},
	{
		ID:          "news",
		Name:        "News",
		Description: "Latest news feed using mu.news()",
		Category:    "Data",
		HTML:        templateNews,
	},
	{
		ID:          "dashboard",
		Name:        "Dashboard",
		Description: "Markets + news + weather in a single view",
		Category:    "Composite",
		HTML:        templateDashboard,
	},
	{
		ID:          "search-hub",
		Name:        "Search Hub",
		Description: "Search the web + AI summarise results",
		Category:    "Composite",
		HTML:        templateSearchHub,
	},
	{
		ID:          "place-explorer",
		Name:        "Place Explorer",
		Description: "Find places nearby + local weather",
		Category:    "Composite",
		HTML:        templatePlaceExplorer,
	},
	{
		ID:          "portfolio",
		Name:        "Portfolio",
		Description: "Track crypto portfolio with live prices + news",
		Category:    "Composite",
		HTML:        templatePortfolio,
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

const templateWeather = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 480px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.current { padding: 20px; background: #f8f8f8; border-radius: 8px; margin-bottom: 16px; text-align: center; }
.temp { font-size: 48px; font-weight: 700; }
.desc { color: #666; margin-top: 4px; }
.details { display: flex; justify-content: center; gap: 24px; margin-top: 12px; font-size: 13px; color: #888; }
.forecast { display: flex; flex-direction: column; gap: 8px; }
.day { display: flex; justify-content: space-between; padding: 10px 12px; border: 1px solid #eee; border-radius: 6px; font-size: 14px; }
.loading { text-align: center; color: #999; padding: 32px; }
.error { color: #c00; padding: 16px; text-align: center; }
.fallback { margin-bottom: 16px; display: flex; gap: 8px; }
.fallback input { flex: 1; padding: 8px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; }
.fallback button { padding: 8px 16px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; }
</style>
</head>
<body>
<h2>Weather</h2>
<div class="fallback">
  <input type="text" id="city" placeholder="Enter city name...">
  <button onclick="searchCity()">Search</button>
</div>
<div id="content"><div class="loading">Getting your location...</div></div>
<script>
function showWeather(lat, lon) {
  document.getElementById('content').innerHTML = '<div class="loading">Loading forecast...</div>';
  mu.weather({lat: lat, lon: lon}).then(function(data) {
    if (!data || data.error || !data.forecast) { showError(data && data.error || 'No forecast data'); return; }
    var f = data.forecast;
    var c = f.Current;
    var html = '<div class="current">';
    html += '<div class="temp">' + Math.round(c.TempC) + '°C</div>';
    html += '<div class="desc">' + (c.Description || '') + '</div>';
    html += '<div class="details"><span>Feels ' + Math.round(c.FeelsLikeC) + '°C</span><span>Humidity ' + c.Humidity + '%</span>';
    if (c.WindKph) html += '<span>Wind ' + Math.round(c.WindKph) + ' km/h</span>';
    html += '</div></div>';
    if (f.DailyItems && f.DailyItems.length > 0) {
      html += '<div class="forecast">';
      f.DailyItems.forEach(function(d) {
        html += '<div class="day"><span>' + (d.Description || 'N/A') + '</span><span>' + Math.round(d.MaxTempC) + '° / ' + Math.round(d.MinTempC) + '°</span></div>';
      });
      html += '</div>';
    }
    document.getElementById('content').innerHTML = html;
  }).catch(function(e) { showError(e.message); });
}
function showError(msg) { document.getElementById('content').innerHTML = '<div class="error">' + msg + '</div>'; }
function searchCity() {
  var city = document.getElementById('city').value.trim();
  if (!city) return;
  mu.places.search({q: city, near: city}).then(function(data) {
    if (data && data.results && data.results.length > 0) {
      var p = data.results[0];
      showWeather(p.lat || p.latitude, p.lon || p.longitude);
    } else { showError('City not found'); }
  }).catch(function(e) { showError(e.message); });
}
// Try geolocation first
if (navigator.geolocation) {
  navigator.geolocation.getCurrentPosition(
    function(pos) { showWeather(pos.coords.latitude, pos.coords.longitude); },
    function() { document.getElementById('content').innerHTML = '<div class="loading">Enter a city above</div>'; }
  );
} else { document.getElementById('content').innerHTML = '<div class="loading">Enter a city above</div>'; }
</script>
</body>
</html>`

const templateMarkets = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 600px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.tabs { display: flex; gap: 8px; margin-bottom: 16px; }
.tabs button { padding: 6px 16px; border: 1px solid #e0e0e0; border-radius: 6px; background: #fff; cursor: pointer; font-family: inherit; font-size: 13px; }
.tabs button.active { background: #000; color: #fff; border-color: #000; }
.coin { display: flex; justify-content: space-between; align-items: center; padding: 12px; border: 1px solid #eee; border-radius: 6px; margin-bottom: 8px; }
.coin-name { font-weight: 600; }
.coin-price { font-variant-numeric: tabular-nums; }
.coin-change { font-size: 13px; font-weight: 500; }
.up { color: #16a34a; }
.down { color: #dc2626; }
.loading { text-align: center; color: #999; padding: 32px; }
.error { color: #c00; padding: 16px; text-align: center; }
</style>
</head>
<body>
<h2>Markets</h2>
<div class="tabs">
  <button class="active" onclick="load('crypto',this)">Crypto</button>
  <button onclick="load('futures',this)">Futures</button>
  <button onclick="load('commodities',this)">Commodities</button>
</div>
<div id="content"><div class="loading">Loading...</div></div>
<script>
function load(category, btn) {
  if (btn) { document.querySelectorAll('.tabs button').forEach(function(b){b.className='';}); btn.className='active'; }
  document.getElementById('content').innerHTML = '<div class="loading">Loading...</div>';
  mu.markets({category: category}).then(function(data) {
    if (!data || data.error) { showError(data && data.error || 'Failed to load'); return; }
    var items = data.data;
    if (!items || !items.length) { showError('No data available'); return; }
    var html = '';
    items.forEach(function(item) {
      var change = item.change_24h || 0;
      var cls = change >= 0 ? 'up' : 'down';
      var sign = change >= 0 ? '+' : '';
      var price = item.price >= 1 ? '$' + item.price.toLocaleString(undefined, {minimumFractionDigits:2, maximumFractionDigits:2}) : '$' + item.price.toFixed(4);
      html += '<div class="coin"><div><span class="coin-name">' + item.symbol + '</span></div><div style="text-align:right"><div class="coin-price">' + price + '</div><div class="coin-change ' + cls + '">' + sign + change.toFixed(2) + '%</div></div></div>';
    });
    document.getElementById('content').innerHTML = html;
  }).catch(function(e) { showError(e.message); });
}
function showError(msg) { document.getElementById('content').innerHTML = '<div class="error">' + msg + '</div>'; }
load('crypto', null);
</script>
</body>
</html>`

const templateNews = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 600px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.article { padding: 12px 0; border-bottom: 1px solid #eee; }
.article:last-child { border-bottom: none; }
.article h3 { font-size: 15px; font-weight: 600; margin-bottom: 4px; }
.article h3 a { color: #333; text-decoration: none; }
.article h3 a:hover { color: #000; }
.article p { font-size: 14px; color: #666; line-height: 1.5; margin-bottom: 4px; }
.article .meta { font-size: 12px; color: #999; }
.loading { text-align: center; color: #999; padding: 32px; }
.error { color: #c00; padding: 16px; text-align: center; }
</style>
</head>
<body>
<h2>News</h2>
<div id="content"><div class="loading">Loading...</div></div>
<script>
mu.news().then(function(data) {
  if (!data || data.error) { showError(data && data.error || 'Failed to load'); return; }
  var items = data.feed;
  if (!items || !items.length) { showError('No news available'); return; }
  var html = '';
  items.slice(0, 20).forEach(function(item) {
    html += '<div class="article">';
    html += '<h3><a href="' + (item.url || '#') + '" target="_blank">' + esc(item.title) + '</a></h3>';
    if (item.description) html += '<p>' + esc(item.description.slice(0, 200)) + '</p>';
    html += '<div class="meta">' + esc(item.category || '') + (item.published ? ' · ' + new Date(item.published).toLocaleDateString() : '') + '</div>';
    html += '</div>';
  });
  document.getElementById('content').innerHTML = html;
}).catch(function(e) { showError(e.message); });
function showError(msg) { document.getElementById('content').innerHTML = '<div class="error">' + msg + '</div>'; }
function esc(s) { var d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }
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

const templateDashboard = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 16px; background: #fff; color: #333; }
h2 { font-size: 18px; font-weight: 600; margin-bottom: 12px; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
@media (max-width: 768px) { .grid { grid-template-columns: 1fr; } }
.panel { border: 1px solid #eee; border-radius: 8px; padding: 16px; }
.panel h3 { font-size: 14px; font-weight: 600; margin-bottom: 10px; color: #555; }
.loading { color: #999; font-size: 13px; }
.error { color: #c00; font-size: 13px; }
.coin { display: flex; justify-content: space-between; padding: 6px 0; border-bottom: 1px solid #f5f5f5; font-size: 14px; }
.coin:last-child { border-bottom: none; }
.up { color: #16a34a; } .down { color: #dc2626; }
.article { padding: 8px 0; border-bottom: 1px solid #f5f5f5; }
.article:last-child { border-bottom: none; }
.article a { color: #333; text-decoration: none; font-size: 14px; font-weight: 500; }
.article a:hover { color: #000; }
.article .meta { font-size: 12px; color: #999; margin-top: 2px; }
.weather-current { text-align: center; padding: 12px 0; }
.weather-temp { font-size: 36px; font-weight: 700; }
.weather-desc { color: #666; font-size: 14px; }
.weather-details { display: flex; justify-content: center; gap: 16px; font-size: 12px; color: #888; margin-top: 8px; }
</style>
</head>
<body>
<h2>Dashboard</h2>
<div class="grid">
  <div class="panel" id="markets-panel">
    <h3>Markets</h3>
    <div class="loading">Loading prices...</div>
  </div>
  <div class="panel" id="weather-panel">
    <h3>Weather</h3>
    <div class="loading">Getting location...</div>
  </div>
  <div class="panel" style="grid-column: 1 / -1;" id="news-panel">
    <h3>News</h3>
    <div class="loading">Loading headlines...</div>
  </div>
</div>
<script>
function esc(s) { var d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }

// Markets
mu.markets({category: 'crypto'}).then(function(data) {
  if (!data || data.error || !data.data) { document.querySelector('#markets-panel .loading').className = 'error'; document.querySelector('#markets-panel .error').textContent = 'Failed to load'; return; }
  var html = '';
  data.data.slice(0, 8).forEach(function(item) {
    var change = item.change_24h || 0;
    var cls = change >= 0 ? 'up' : 'down';
    var sign = change >= 0 ? '+' : '';
    var price = item.price >= 1 ? '$' + item.price.toLocaleString(undefined, {maximumFractionDigits:2}) : '$' + item.price.toFixed(4);
    html += '<div class="coin"><span>' + item.symbol + '</span><span>' + price + ' <span class="' + cls + '">' + sign + change.toFixed(1) + '%</span></span></div>';
  });
  document.getElementById('markets-panel').innerHTML = '<h3>Markets</h3>' + html;
}).catch(function(e) { document.querySelector('#markets-panel .loading').textContent = e.message; });

// Weather
function loadWeather(lat, lon) {
  mu.weather({lat: lat, lon: lon}).then(function(data) {
    if (!data || data.error || !data.forecast) { document.querySelector('#weather-panel .loading').textContent = 'No forecast'; return; }
    var c = data.forecast.Current;
    var html = '<h3>Weather</h3><div class="weather-current">';
    html += '<div class="weather-temp">' + Math.round(c.TempC) + '°C</div>';
    html += '<div class="weather-desc">' + esc(c.Description || '') + '</div>';
    html += '<div class="weather-details"><span>Feels ' + Math.round(c.FeelsLikeC) + '°</span><span>' + c.Humidity + '% humidity</span></div>';
    html += '</div>';
    document.getElementById('weather-panel').innerHTML = html;
  }).catch(function(e) { document.querySelector('#weather-panel .loading').textContent = e.message; });
}
if (navigator.geolocation) {
  navigator.geolocation.getCurrentPosition(
    function(pos) { loadWeather(pos.coords.latitude, pos.coords.longitude); },
    function() { loadWeather(51.5, -0.12); } // Default London
  );
} else { loadWeather(51.5, -0.12); }

// News
mu.news().then(function(data) {
  if (!data || data.error || !data.feed) { document.querySelector('#news-panel .loading').textContent = 'Failed to load'; return; }
  var html = '<h3>News</h3>';
  data.feed.slice(0, 8).forEach(function(item) {
    html += '<div class="article"><a href="' + esc(item.url || '#') + '" target="_blank">' + esc(item.title) + '</a>';
    html += '<div class="meta">' + esc(item.category || '') + '</div></div>';
  });
  document.getElementById('news-panel').innerHTML = html;
}).catch(function(e) { document.querySelector('#news-panel .loading').textContent = e.message; });
</script>
</body>
</html>`

const templateSearchHub = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 640px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.search-bar { display: flex; gap: 8px; margin-bottom: 16px; }
.search-bar input { flex: 1; padding: 10px 14px; border: 1px solid #e0e0e0; border-radius: 6px; font-size: 15px; font-family: inherit; }
.search-bar button { padding: 10px 20px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; font-size: 15px; }
.search-bar button:disabled { background: #ccc; }
.summary { padding: 16px; background: #f8f8f8; border-radius: 8px; margin-bottom: 16px; line-height: 1.6; font-size: 14px; display: none; }
.summary h3 { font-size: 14px; font-weight: 600; margin-bottom: 8px; color: #555; }
.results { display: flex; flex-direction: column; gap: 12px; }
.result { padding: 12px; border: 1px solid #eee; border-radius: 6px; }
.result a { color: #333; text-decoration: none; font-weight: 500; font-size: 15px; }
.result a:hover { color: #000; }
.result p { font-size: 13px; color: #666; margin-top: 4px; line-height: 1.4; }
.result .url { font-size: 12px; color: #999; margin-top: 4px; }
.loading { text-align: center; color: #999; padding: 24px; display: none; }
</style>
</head>
<body>
<h2>Search Hub</h2>
<div class="search-bar">
  <input type="text" id="query" placeholder="Search anything..." onkeydown="if(event.key==='Enter')doSearch()">
  <button id="btn" onclick="doSearch()">Search</button>
</div>
<div class="summary" id="summary"></div>
<div class="loading" id="loading">Searching...</div>
<div class="results" id="results"></div>
<script>
function esc(s) { var d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }

function doSearch() {
  var q = document.getElementById('query').value.trim();
  if (!q) return;
  var btn = document.getElementById('btn');
  btn.disabled = true;
  document.getElementById('loading').style.display = 'block';
  document.getElementById('results').innerHTML = '';
  document.getElementById('summary').style.display = 'none';

  // Search platform content and web in parallel
  Promise.all([
    mu.search(q).catch(function() { return {results: []}; }),
    mu.get('/web?q=' + encodeURIComponent(q)).catch(function() { return {results: []}; })
  ]).then(function(responses) {
    var local = responses[0];
    var web = responses[1];
    var html = '';

    // Show local results
    var localItems = (local && local.results) || [];
    localItems.slice(0, 5).forEach(function(r) {
      html += '<div class="result"><a href="' + esc(r.url || '#') + '">' + esc(r.title || r.name || 'Result') + '</a>';
      if (r.description) html += '<p>' + esc(r.description.slice(0, 150)) + '</p>';
      html += '</div>';
    });

    // Show web results
    var webItems = (web && web.results) || [];
    webItems.slice(0, 5).forEach(function(r) {
      html += '<div class="result"><a href="' + esc(r.url || '#') + '" target="_blank">' + esc(r.title || 'Result') + '</a>';
      if (r.description) html += '<p>' + esc(r.description.slice(0, 150)) + '</p>';
      html += '<div class="url">' + esc(r.url || '') + '</div></div>';
    });

    document.getElementById('results').innerHTML = html || '<p style="color:#999">No results found.</p>';
    document.getElementById('loading').style.display = 'none';
    btn.disabled = false;

    // AI summary
    if (localItems.length > 0 || webItems.length > 0) {
      var context = (localItems.concat(webItems)).slice(0, 5).map(function(r) { return (r.title || '') + ': ' + (r.description || ''); }).join('\n');
      mu.ai('Based on these search results, give a brief 2-3 sentence summary answering: ' + q + '\n\nResults:\n' + context).then(function(answer) {
        var el = document.getElementById('summary');
        el.innerHTML = '<h3>AI Summary</h3>' + esc(typeof answer === 'string' ? answer : JSON.stringify(answer));
        el.style.display = 'block';
      });
    }
  });
}
</script>
</body>
</html>`

const templatePlaceExplorer = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 600px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.search-bar { display: flex; gap: 8px; margin-bottom: 16px; }
.search-bar input { flex: 1; padding: 10px 14px; border: 1px solid #e0e0e0; border-radius: 6px; font-size: 15px; font-family: inherit; }
.search-bar button { padding: 10px 20px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; }
.weather-bar { padding: 12px 16px; background: #f8f8f8; border-radius: 8px; margin-bottom: 16px; display: none; font-size: 14px; }
.weather-bar .temp { font-size: 24px; font-weight: 700; }
.place { padding: 12px; border: 1px solid #eee; border-radius: 6px; margin-bottom: 8px; }
.place h3 { font-size: 15px; font-weight: 600; margin-bottom: 4px; }
.place p { font-size: 13px; color: #666; }
.place .meta { font-size: 12px; color: #999; margin-top: 4px; }
.loading { text-align: center; color: #999; padding: 24px; }
.quick-tags { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 16px; }
.quick-tags button { padding: 4px 12px; border: 1px solid #e0e0e0; border-radius: 12px; background: #fff; cursor: pointer; font-size: 12px; font-family: inherit; }
.quick-tags button:hover { background: #f5f5f5; }
</style>
</head>
<body>
<h2>Place Explorer</h2>
<div class="search-bar">
  <input type="text" id="location" placeholder="Enter a city or address..." value="">
  <button onclick="explore()">Explore</button>
</div>
<div class="quick-tags">
  <button onclick="searchFor('coffee')">Coffee</button>
  <button onclick="searchFor('restaurant')">Food</button>
  <button onclick="searchFor('park')">Parks</button>
  <button onclick="searchFor('mosque')">Mosques</button>
  <button onclick="searchFor('gym')">Gyms</button>
  <button onclick="searchFor('library')">Libraries</button>
</div>
<div class="weather-bar" id="weather"></div>
<div id="results"></div>
<script>
function esc(s) { var d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }
var currentLocation = '';
var currentQuery = 'coffee';

function explore() {
  currentLocation = document.getElementById('location').value.trim();
  if (!currentLocation) return;
  loadWeather();
  searchFor(currentQuery);
}

function loadWeather() {
  mu.places.search({q: currentLocation, near: currentLocation}).then(function(data) {
    if (data && data.results && data.results.length > 0) {
      var p = data.results[0];
      var lat = p.lat || p.latitude;
      var lon = p.lon || p.longitude;
      if (lat && lon) {
        mu.weather({lat: lat, lon: lon}).then(function(w) {
          if (w && w.forecast && w.forecast.Current) {
            var c = w.forecast.Current;
            var el = document.getElementById('weather');
            el.innerHTML = '<span class="temp">' + Math.round(c.TempC) + '°C</span> · ' + esc(c.Description || '') + ' · Humidity ' + c.Humidity + '%';
            el.style.display = 'block';
          }
        });
      }
    }
  });
}

function searchFor(query) {
  currentQuery = query;
  if (!currentLocation) { document.getElementById('results').innerHTML = '<div class="loading">Enter a location above</div>'; return; }
  document.getElementById('results').innerHTML = '<div class="loading">Searching for ' + esc(query) + '...</div>';
  mu.places.search({q: query, near: currentLocation}).then(function(data) {
    if (!data || data.error) { document.getElementById('results').innerHTML = '<div class="loading">' + esc(data && data.error || 'No results') + '</div>'; return; }
    var places = data.results || [];
    if (places.length === 0) { document.getElementById('results').innerHTML = '<div class="loading">No places found</div>'; return; }
    var html = '';
    places.slice(0, 10).forEach(function(p) {
      html += '<div class="place"><h3>' + esc(p.name) + '</h3>';
      if (p.address) html += '<p>' + esc(p.address) + '</p>';
      var meta = [];
      if (p.rating) meta.push('Rating: ' + p.rating);
      if (p.distance) meta.push(p.distance);
      if (meta.length) html += '<div class="meta">' + esc(meta.join(' · ')) + '</div>';
      html += '</div>';
    });
    document.getElementById('results').innerHTML = html;
  }).catch(function(e) { document.getElementById('results').innerHTML = '<div class="loading">' + esc(e.message) + '</div>'; });
}
</script>
</body>
</html>`

const templatePortfolio = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif; padding: 24px; background: #fff; color: #333; max-width: 600px; margin: 0 auto; }
h2 { font-size: 20px; font-weight: 600; margin-bottom: 16px; }
.total { font-size: 32px; font-weight: 700; margin-bottom: 4px; }
.total-change { font-size: 14px; margin-bottom: 20px; }
.up { color: #16a34a; } .down { color: #dc2626; }
.holding { display: flex; justify-content: space-between; align-items: center; padding: 12px; border: 1px solid #eee; border-radius: 6px; margin-bottom: 8px; }
.holding-left { display: flex; align-items: center; gap: 12px; }
.holding-symbol { font-weight: 600; }
.holding-amount { font-size: 13px; color: #888; }
.holding-right { text-align: right; }
.holding-value { font-weight: 500; font-variant-numeric: tabular-nums; }
.holding-change { font-size: 13px; }
.add-form { display: flex; gap: 8px; margin-top: 16px; flex-wrap: wrap; }
.add-form input, .add-form select { padding: 8px 12px; border: 1px solid #e0e0e0; border-radius: 6px; font-family: inherit; font-size: 14px; }
.add-form button { padding: 8px 16px; background: #000; color: #fff; border: none; border-radius: 6px; cursor: pointer; font-family: inherit; }
.news-section { margin-top: 24px; }
.news-section h3 { font-size: 14px; font-weight: 600; color: #555; margin-bottom: 10px; }
.news-item { padding: 8px 0; border-bottom: 1px solid #f5f5f5; font-size: 14px; }
.news-item a { color: #333; text-decoration: none; }
.news-item a:hover { color: #000; }
.news-item .meta { font-size: 12px; color: #999; }
.loading { text-align: center; color: #999; padding: 24px; }
</style>
</head>
<body>
<h2>Portfolio</h2>
<div class="total" id="totalValue">$0.00</div>
<div class="total-change" id="totalChange"></div>
<div id="holdings"></div>
<div class="add-form">
  <select id="addSymbol"><option value="">Add coin...</option></select>
  <input type="number" id="addAmount" placeholder="Amount" step="any" style="width:100px;">
  <button onclick="addHolding()">Add</button>
</div>
<div class="news-section">
  <h3>Crypto News</h3>
  <div id="news"><div class="loading">Loading...</div></div>
</div>
<script>
function esc(s) { var d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }
var portfolio = JSON.parse(localStorage.getItem('mu_portfolio') || '[]');
var prices = {};

function savePortfolio() { localStorage.setItem('mu_portfolio', JSON.stringify(portfolio)); }

function render() {
  var html = '';
  var totalVal = 0, totalPrev = 0;
  portfolio.forEach(function(h, i) {
    var p = prices[h.symbol];
    if (!p) { html += '<div class="holding"><span>' + h.symbol + ': ' + h.amount + '</span><span>—</span></div>'; return; }
    var val = h.amount * p.price;
    var prevVal = h.amount * (p.price / (1 + (p.change_24h || 0) / 100));
    totalVal += val;
    totalPrev += prevVal;
    var cls = p.change_24h >= 0 ? 'up' : 'down';
    var sign = p.change_24h >= 0 ? '+' : '';
    html += '<div class="holding"><div class="holding-left"><div><div class="holding-symbol">' + h.symbol + '</div><div class="holding-amount">' + h.amount + '</div></div></div>';
    html += '<div class="holding-right"><div class="holding-value">$' + val.toLocaleString(undefined,{maximumFractionDigits:2}) + '</div>';
    html += '<div class="holding-change ' + cls + '">' + sign + (p.change_24h||0).toFixed(1) + '%</div></div></div>';
  });
  document.getElementById('holdings').innerHTML = html || '<p style="color:#999;font-size:14px">No holdings yet. Add some below.</p>';
  document.getElementById('totalValue').textContent = '$' + totalVal.toLocaleString(undefined,{maximumFractionDigits:2});
  var totalChange = totalPrev > 0 ? ((totalVal - totalPrev) / totalPrev * 100) : 0;
  var tcCls = totalChange >= 0 ? 'up' : 'down';
  var tcSign = totalChange >= 0 ? '+' : '';
  document.getElementById('totalChange').innerHTML = '<span class="' + tcCls + '">' + tcSign + totalChange.toFixed(2) + '% today</span>';
}

function addHolding() {
  var sym = document.getElementById('addSymbol').value;
  var amt = parseFloat(document.getElementById('addAmount').value);
  if (!sym || isNaN(amt) || amt <= 0) return;
  var existing = portfolio.find(function(h) { return h.symbol === sym; });
  if (existing) { existing.amount += amt; } else { portfolio.push({symbol: sym, amount: amt}); }
  savePortfolio();
  document.getElementById('addAmount').value = '';
  render();
}

// Load prices
mu.markets({category: 'crypto'}).then(function(data) {
  if (!data || !data.data) return;
  var sel = document.getElementById('addSymbol');
  data.data.forEach(function(item) {
    prices[item.symbol] = item;
    var opt = document.createElement('option');
    opt.value = item.symbol; opt.textContent = item.symbol + ' ($' + (item.price >= 1 ? item.price.toFixed(2) : item.price.toFixed(4)) + ')';
    sel.appendChild(opt);
  });
  render();
});

// Load news
mu.news().then(function(data) {
  if (!data || !data.feed) { document.getElementById('news').innerHTML = '<p style="color:#999">No news</p>'; return; }
  var crypto = data.feed.filter(function(a) { return (a.category || '').toLowerCase().indexOf('crypto') >= 0 || (a.title || '').toLowerCase().match(/bitcoin|crypto|eth|defi/); });
  if (crypto.length === 0) crypto = data.feed.slice(0, 5);
  var html = '';
  crypto.slice(0, 5).forEach(function(a) {
    html += '<div class="news-item"><a href="' + esc(a.url || '#') + '" target="_blank">' + esc(a.title) + '</a><div class="meta">' + esc(a.category || '') + '</div></div>';
  });
  document.getElementById('news').innerHTML = html;
});
</script>
</body>
</html>`
