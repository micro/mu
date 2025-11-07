package apps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"

	"github.com/google/uuid"
)

var mutex sync.RWMutex
var userApps = map[string][]App{}   // userID -> apps
var publicApps = []App{}            // public apps list
var appsById = map[string]App{}     // appID -> app

const PageTemplate = `
<div id="apps">
  <h1>My Apps</h1>
  <div id="create-app">
    <h3>Create New App</h3>
    <form id="app-form" onsubmit="event.preventDefault(); createApp(this);">
      <input id="app-name" name="name" placeholder="App Name" required>
      <br>
      <textarea id="app-prompt" name="prompt" placeholder="Describe your app (e.g., 'A todo list with drag and drop')" rows="4" style="width: calc(100%% - 60px); padding: 10px; border-radius: 5px; border: 1px solid darkgrey; margin-top: 10px;" required></textarea>
      <br>
      <label><input type="checkbox" name="public"> Make this app public</label>
      <br>
      <button>Generate App</button>
    </form>
  </div>
  <div id="app-list">
    <h3>Your Apps</h3>
    %s
  </div>
  <div id="public-apps">
    <h3>Public Apps</h3>
    %s
  </div>
</div>

<script>
function createApp(form) {
  const formData = new FormData(form);
  const data = {};
  for (let [key, value] of formData.entries()) {
    if (key === 'public') {
      data[key] = form.elements['public'].checked;
    } else {
      data[key] = value;
    }
  }

  fetch('/apps', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  })
  .then(response => response.json())
  .then(result => {
    console.log('Success:', result);
    window.location.href = '/apps/' + result.id;
  })
  .catch(error => {
    console.error('Error:', error);
    alert('Failed to create app: ' + error);
  });
}

function viewApp(appId) {
  window.location.href = '/apps/' + appId;
}
</script>
`

const AppViewTemplate = `
<div id="app-view">
  <div style="margin-bottom: 20px;">
    <a href="/apps">← Back to Apps</a>
  </div>
  <h1>%s</h1>
  <p>%s</p>
  <div style="margin-bottom: 20px;">
    <button onclick="editApp()">Edit</button>
    <button onclick="deleteApp()">Delete</button>
    <span style="margin-left: 20px;">Uses: %d</span>
  </div>
  <div id="app-preview" style="border: 1px solid #ccc; border-radius: 5px; padding: 20px; background: white;">
    <iframe id="preview-frame" style="width: 100%%; height: 600px; border: none;" srcdoc="%s"></iframe>
  </div>
</div>

<script>
function editApp() {
  alert('Edit functionality coming soon');
}

function deleteApp() {
  if (confirm('Are you sure you want to delete this app?')) {
    fetch('/apps/%s', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' }
    })
    .then(response => {
      if (response.ok) {
        window.location.href = '/apps';
      }
    })
    .catch(error => {
      console.error('Error:', error);
      alert('Failed to delete app');
    });
  }
}
</script>
`

func init() {
	// Load apps from disk
	b, _ := data.Load("apps.json")
	var allApps []App
	json.Unmarshal(b, &allApps)
	
	for _, a := range allApps {
		appsById[a.ID] = a
		userApps[a.UserID] = append(userApps[a.UserID], a)
		if a.Public {
			publicApps = append(publicApps, a)
		}
	}
}

func save() error {
	var allApps []App
	for _, a := range appsById {
		allApps = append(allApps, a)
	}
	return data.SaveJSON("apps.json", allApps)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Get session
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}

	// Handle different routes
	path := r.URL.Path
	
	if path == "/apps" && r.Method == "GET" {
		handleList(w, r, sess)
		return
	}
	
	if path == "/apps" && r.Method == "POST" {
		handleCreate(w, r, sess)
		return
	}
	
	// Handle /apps/{id} routes
	if len(path) > 6 && path[:6] == "/apps/" {
		appID := path[6:]
		
		if r.Method == "GET" {
			handleView(w, r, sess, appID)
			return
		}
		
		if r.Method == "DELETE" {
			handleDelete(w, r, sess, appID)
			return
		}
	}
	
	http.Error(w, "not found", 404)
}

func handleList(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	mutex.RLock()
	myApps := userApps[sess.Account]
	pubApps := publicApps
	mutex.RUnlock()

	// Format user apps list
	var myAppsList string
	if len(myApps) == 0 {
		myAppsList = "<p>No apps yet. Create your first app above!</p>"
	} else {
		for _, a := range myApps {
			myAppsList += fmt.Sprintf(`
<div class="card" style="cursor: pointer;" onclick="viewApp('%s')">
  <h4>%s</h4>
  <p>%s</p>
  <span class="text">Created: %s | Uses: %d</span>
</div>
`, a.ID, a.Name, a.Description, a.Created.Format("2006-01-02"), a.UseCount)
		}
	}

	// Format public apps list
	var pubAppsList string
	if len(pubApps) == 0 {
		pubAppsList = "<p>No public apps yet.</p>"
	} else {
		for _, a := range pubApps {
			// Don't show user's own apps in public section
			if a.UserID == sess.Account {
				continue
			}
			pubAppsList += fmt.Sprintf(`
<div class="card" style="cursor: pointer;" onclick="viewApp('%s')">
  <h4>%s</h4>
  <p>%s</p>
  <span class="text">By: %s | Uses: %d</span>
</div>
`, a.ID, a.Name, a.Description, a.UserID, a.UseCount)
		}
		if pubAppsList == "" {
			pubAppsList = "<p>No public apps from other users yet.</p>"
		}
	}

	page := fmt.Sprintf(PageTemplate, myAppsList, pubAppsList)
	html := app.RenderHTML("Apps", "Create and manage your mini apps", page)
	w.Write([]byte(html))
}

func handleCreate(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	var req struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		Public bool   `json:"public"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}

	if req.Name == "" || req.Prompt == "" {
		http.Error(w, "name and prompt required", 400)
		return
	}

	// Generate app using LLM
	generated, err := GenerateApp(req.Prompt, "")
	if err != nil {
		http.Error(w, "failed to generate app", 500)
		return
	}

	// Create new app
	appID := uuid.New().String()
	now := time.Now()
	
	newApp := App{
		ID:          appID,
		UserID:      sess.Account,
		Name:        req.Name,
		Description: generated["description"],
		HTML:        generated["html"],
		CSS:         generated["css"],
		JS:          generated["js"],
		Prompt:      req.Prompt,
		Public:      req.Public,
		Created:     now,
		Updated:     now,
		UseCount:    0,
	}

	mutex.Lock()
	appsById[appID] = newApp
	userApps[sess.Account] = append(userApps[sess.Account], newApp)
	if newApp.Public {
		publicApps = append(publicApps, newApp)
	}
	save()
	mutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": appID})
}

func handleView(w http.ResponseWriter, r *http.Request, sess *auth.Session, appID string) {
	mutex.RLock()
	a, exists := appsById[appID]
	mutex.RUnlock()

	if !exists {
		http.Error(w, "app not found", 404)
		return
	}

	// Check access: owner or public app
	if a.UserID != sess.Account && !a.Public {
		http.Error(w, "unauthorized", 403)
		return
	}

	// Increment use count
	mutex.Lock()
	a.UseCount++
	appsById[appID] = a
	save()
	mutex.Unlock()

	// Build the complete HTML document
	fullHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>%s</style>
</head>
<body>
%s
<script>%s</script>
</body>
</html>`, a.Name, a.CSS, a.HTML, a.JS)

	// Escape for iframe srcdoc
	escapedHTML := escapeHTML(fullHTML)

	page := fmt.Sprintf(AppViewTemplate, a.Name, a.Description, a.UseCount, escapedHTML, appID)
	html := app.RenderHTML(a.Name, a.Description, page)
	w.Write([]byte(html))
}

func handleDelete(w http.ResponseWriter, r *http.Request, sess *auth.Session, appID string) {
	mutex.Lock()
	defer mutex.Unlock()

	a, exists := appsById[appID]
	if !exists {
		http.Error(w, "app not found", 404)
		return
	}

	// Only owner can delete
	if a.UserID != sess.Account {
		http.Error(w, "unauthorized", 403)
		return
	}

	// Remove from maps
	delete(appsById, appID)
	
	// Remove from user apps
	apps := userApps[sess.Account]
	for i, ua := range apps {
		if ua.ID == appID {
			userApps[sess.Account] = append(apps[:i], apps[i+1:]...)
			break
		}
	}

	// Remove from public apps
	if a.Public {
		for i, pa := range publicApps {
			if pa.ID == appID {
				publicApps = append(publicApps[:i], publicApps[i+1:]...)
				break
			}
		}
	}

	save()
	w.WriteHeader(http.StatusOK)
}

func escapeHTML(s string) string {
	// Basic HTML escaping for iframe srcdoc
	s = fmt.Sprintf("%q", s)
	// Remove the quotes added by %q
	if len(s) >= 2 {
		s = s[1 : len(s)-1]
	}
	return s
}
