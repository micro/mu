package apps

import (
	"time"

	"mu/internal/app"

	"github.com/google/uuid"
)

// helloWorldHTML is the complete page for the built-in "Hello World" app — a
// self-contained raw-mode example with no dependencies.
const helloWorldHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Hello World</title>
<style>
  body{font-family:system-ui,-apple-system,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;background:#fafafa;color:#111}
  .card{text-align:center;padding:40px}
  h1{font-size:32px;margin:0 0 8px}
  p{color:#666;margin:0 0 20px}
  button{font:inherit;padding:8px 18px;border:1px solid #111;border-radius:8px;background:#111;color:#fff;cursor:pointer}
  button:hover{opacity:.85}
  #out{margin-top:16px;color:#333;min-height:20px}
</style>
</head>
<body>
  <div class="card">
    <h1>Hello, World 👋</h1>
    <p>A minimal Mu app.</p>
    <button id="btn">Say hello</button>
    <div id="out"></div>
  </div>
  <script>
    var n=0;
    document.getElementById('btn').addEventListener('click',function(){
      n++;
      document.getElementById('out').textContent='Hello #'+n+' — '+new Date().toLocaleTimeString();
    });
  </script>
</body>
</html>`

// ensureBuiltins makes sure every built-in ("mu"-authored) app exists. It runs
// on every startup — not just first run — so a newly-added built-in appears on
// existing instances too. It only fills gaps: it never overwrites a user's app,
// or a user's in-place edits to a built-in. Think of these as the apps that ship
// with the OS; you can still build and run your own on top.
func ensureBuiltins() {
	added := 0
	mutex.Lock()
	for _, a := range builtinApps() {
		if _, exists := apps[a.Slug]; !exists {
			apps[a.Slug] = a
			added++
		}
	}
	mutex.Unlock()
	if added > 0 {
		save()
		app.Log("apps", "Added %d built-in apps", added)
	}
}

// builtinApps returns the definitions of the apps that ship with every instance.
func builtinApps() []*App {
	seeds := []struct {
		Slug        string
		Name        string
		Description string
		Tags        string
		TemplateID  string
		Icon        string
	}{
		{
			Slug:        "timer",
			Name:        "Timer",
			Description: "Countdown timer with start, pause, and reset",
			Tags:        "productivity, timer",
			TemplateID:  "timer",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <circle cx="16" cy="18" r="11" fill="none" stroke="#555" stroke-width="2"/>
  <line x1="16" y1="18" x2="16" y2="11" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <line x1="16" y1="18" x2="21" y2="18" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <line x1="16" y1="5" x2="16" y2="7" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <line x1="13" y1="4" x2="19" y2="4" stroke="#555" stroke-width="2" stroke-linecap="round"/>
</svg>`,
		},
		{
			Slug:        "calculator",
			Name:        "Calculator",
			Description: "Simple calculator with basic arithmetic operations",
			Tags:        "tools, calculator",
			TemplateID:  "calculator",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <rect x="6" y="3" width="20" height="26" rx="3" fill="none" stroke="#555" stroke-width="2"/>
  <rect x="9" y="6" width="14" height="6" rx="1" fill="none" stroke="#555" stroke-width="1.5"/>
  <circle cx="11" cy="17" r="1.5" fill="#555"/>
  <circle cx="16" cy="17" r="1.5" fill="#555"/>
  <circle cx="21" cy="17" r="1.5" fill="#555"/>
  <circle cx="11" cy="22" r="1.5" fill="#555"/>
  <circle cx="16" cy="22" r="1.5" fill="#555"/>
  <circle cx="21" cy="22" r="1.5" fill="#555"/>
</svg>`,
		},
		{
			Slug:        "unit-converter",
			Name:        "Unit Converter",
			Description: "Convert between units — temperature, weight, distance",
			Tags:        "tools, converter",
			TemplateID:  "converter",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <polyline points="8,12 12,8 16,12" fill="none" stroke="#555" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
  <line x1="12" y1="8" x2="12" y2="24" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <polyline points="16,20 20,24 24,20" fill="none" stroke="#555" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
  <line x1="20" y1="8" x2="20" y2="24" stroke="#555" stroke-width="2" stroke-linecap="round"/>
</svg>`,
		},
		{
			Slug:        "flashcards",
			Name:        "Flashcards",
			Description: "Study flashcards — click to flip, arrow keys to navigate",
			Tags:        "education, study",
			TemplateID:  "flashcards",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <rect x="3" y="7" width="20" height="16" rx="2" fill="none" stroke="#555" stroke-width="2" transform="rotate(-5 13 15)"/>
  <rect x="7" y="8" width="20" height="16" rx="2" fill="none" stroke="#555" stroke-width="2"/>
  <line x1="12" y1="14" x2="22" y2="14" stroke="#555" stroke-width="1.5" stroke-linecap="round"/>
  <line x1="12" y1="18" x2="19" y2="18" stroke="#555" stroke-width="1.5" stroke-linecap="round"/>
</svg>`,
		},
		{
			Slug:        "bookmarks",
			Name:        "Bookmarks",
			Description: "Save links privately or publicly — an example app on mu.db and mu.web.fetch",
			Tags:        "productivity, bookmarks, example",
			TemplateID:  "bookmarks",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <path d="M9 5h14a1 1 0 0 1 1 1v21l-8-5-8 5V6a1 1 0 0 1 1-1z" fill="none" stroke="#555" stroke-width="2" stroke-linejoin="round"/>
</svg>`,
		},
		{
			Slug:        "notes",
			Name:        "Notes",
			Description: "Quick notes that save automatically",
			Tags:        "productivity, notes",
			TemplateID:  "notes",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <path d="M8 4h12l4 4v20a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z" fill="none" stroke="#555" stroke-width="2"/>
  <polyline points="20,4 20,8 24,8" fill="none" stroke="#555" stroke-width="2" stroke-linejoin="round"/>
  <line x1="10" y1="14" x2="22" y2="14" stroke="#555" stroke-width="1.5" stroke-linecap="round"/>
  <line x1="10" y1="18" x2="22" y2="18" stroke="#555" stroke-width="1.5" stroke-linecap="round"/>
  <line x1="10" y1="22" x2="17" y2="22" stroke="#555" stroke-width="1.5" stroke-linecap="round"/>
</svg>`,
		},
		{
			Slug:        "habit-tracker",
			Name:        "Habit Tracker",
			Description: "Track daily habits with a simple counter",
			Tags:        "productivity, habits",
			TemplateID:  "tracker",
			Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <rect x="4" y="6" width="24" height="22" rx="3" fill="none" stroke="#555" stroke-width="2"/>
  <line x1="4" y1="12" x2="28" y2="12" stroke="#555" stroke-width="2"/>
  <line x1="10" y1="6" x2="10" y2="3" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <line x1="22" y1="6" x2="22" y2="3" stroke="#555" stroke-width="2" stroke-linecap="round"/>
  <polyline points="10,19 13,22 18,16" fill="none" stroke="#555" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`,
		},
	}

	now := time.Now()
	out := make([]*App, 0, len(seeds)+1)

	// A minimal raw-HTML app — the "hello world" of the apps platform. Unlike the
	// template-backed seeds above, it ships its own complete HTML page, so it also
	// serves as the simplest possible example of a raw-mode app.
	out = append(out, &App{
		ID:          uuid.New().String(),
		Slug:        "hello-world",
		Name:        "Hello World",
		Description: "A minimal example app — the hello world of Mu apps",
		AuthorID:    "mu",
		Author:      "mu",
		Icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32">
  <circle cx="16" cy="16" r="12" fill="none" stroke="#555" stroke-width="2"/>
  <ellipse cx="16" cy="16" rx="5" ry="12" fill="none" stroke="#555" stroke-width="2"/>
  <line x1="4" y1="16" x2="28" y2="16" stroke="#555" stroke-width="2"/>
</svg>`,
		Mode:      "raw",
		HTML:      helloWorldHTML,
		Tags:      "example, hello-world",
		Public:    true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, s := range seeds {
		t := GetTemplate(s.TemplateID)
		if t == nil {
			continue
		}
		out = append(out, &App{
			ID:          uuid.New().String(),
			Slug:        s.Slug,
			Name:        s.Name,
			Description: s.Description,
			AuthorID:    "mu",
			Author:      "mu",
			Icon:        s.Icon,
			HTML:        t.HTML,
			Tags:        s.Tags,
			Public:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	return out
}
