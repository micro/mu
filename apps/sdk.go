package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/auth"
)

// Theme CSS variables injected into every app
const ThemeCSS = `
<style id="mu-theme">
:root {
  /* Mu Design Tokens (prefixed) */
  --mu-card-border: #e8e8e8;
  --mu-card-background: #ffffff;
  --mu-hover-background: #fafafa;
  --mu-divider: #f0f0f0;
  --mu-border-color: #f0f0f0;
  --mu-text-primary: #1a1a1a;
  --mu-text-secondary: #555;
  --mu-text-muted: #888;
  --mu-accent-color: #0d7377;
  --mu-accent-blue: #007bff;
  
  /* Non-prefixed aliases (for convenience) */
  --accent-color: #0d7377;
  --accent-blue: #007bff;
  --text-primary: #1a1a1a;
  --text-secondary: #555;
  --text-muted: #888;
  --border-color: #f0f0f0;
  
  --mu-spacing-xs: 4px;
  --mu-spacing-sm: 8px;
  --mu-spacing-md: 16px;
  --mu-spacing-lg: 24px;
  --mu-spacing-xl: 32px;
  
  --mu-border-radius: 6px;
  --mu-shadow-sm: 0 1px 3px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.06);
  --mu-shadow-md: 0 4px 6px rgba(0,0,0,0.04), 0 2px 4px rgba(0,0,0,0.06);
  --mu-transition-fast: 0.15s ease;
  
  --mu-font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
</style>
`

// SDK is the JavaScript SDK injected into every app
// Uses fetch to call the parent API (works with sandboxed iframes)
const SDK = `
<script>
(function() {
  // Mu SDK v1
  const appId = '%s';
  const appName = '%s';
  const userId = '%s';
  const userName = '%s';
  const apiBase = window.location.origin;
  
  async function call(method, params) {
    try {
      const resp = await fetch(apiBase + '/apps/api', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          appId: appId,
          method: method,
          params: params || {}
        })
      });
      const data = await resp.json();
      if (data.error) {
        throw new Error(data.error);
      }
      return data.result;
    } catch (err) {
      console.error('Mu SDK error:', method, err);
      throw err;
    }
  }
  
  window.mu = {
    // App context
    app: {
      id: appId,
      name: appName
    },
    
    // User context
    user: {
      id: userId || null,
      name: userName || null,
      loggedIn: !!userId
    },
    
    // Database API
    db: {
      async get(key) {
        return call('db.get', { key });
      },
      async set(key, value) {
        return call('db.set', { key, value });
      },
      async delete(key) {
        return call('db.delete', { key });
      },
      async list() {
        return call('db.list', {});
      },
      async quota() {
        return call('db.quota', {});
      }
    },
    
    // Fetch proxy (bypasses CORS restrictions)
    async fetch(url, options = {}) {
      const result = await call('fetch', { url, method: options.method || 'GET' });
      return {
        ok: result.status >= 200 && result.status < 300,
        status: result.status,
        statusText: result.statusText,
        text: async () => result.text,
        json: async () => JSON.parse(result.text),
        headers: result.headers
      };
    },
    
    // Theme utilities
    theme: {
      // Get CSS variable value (supports both 'accent-color' and 'accentColor')
      get(name) {
        // Convert camelCase to kebab-case
        const kebabName = name.replace(/([A-Z])/g, '-$1').toLowerCase();
        // Try prefixed first, then non-prefixed
        let value = getComputedStyle(document.documentElement).getPropertyValue('--mu-' + kebabName).trim();
        if (!value) {
          value = getComputedStyle(document.documentElement).getPropertyValue('--' + kebabName).trim();
        }
        return value;
      }
    },
    
    // Cache API (client-side localStorage with TTL)
    cache: {
      _prefix: 'mu_cache_' + appId + '_',
      
      async get(key) {
        try {
          const item = localStorage.getItem(this._prefix + key);
          if (!item) return null;
          const { value, expires } = JSON.parse(item);
          if (expires && Date.now() > expires) {
            localStorage.removeItem(this._prefix + key);
            return null;
          }
          return value;
        } catch (e) {
          return null;
        }
      },
      
      async set(key, value, options = {}) {
        const item = {
          value,
          expires: options.ttl ? Date.now() + (options.ttl * 1000) : null
        };
        localStorage.setItem(this._prefix + key, JSON.stringify(item));
        return true;
      },
      
      async delete(key) {
        localStorage.removeItem(this._prefix + key);
        return true;
      },
      
      async clear() {
        const prefix = this._prefix;
        Object.keys(localStorage)
          .filter(k => k.startsWith(prefix))
          .forEach(k => localStorage.removeItem(k));
        return true;
      }
    }
  };
  
  // Log SDK ready
  console.log('Mu SDK ready', { app: mu.app, user: mu.user });
})();
</script>
`

// GenerateSDK creates the SDK script for a specific app and user
func GenerateSDK(appID, appName, userID, userName string) string {
	return fmt.Sprintf(SDK, appID, appName, userID, userName)
}

// InjectSDK injects the SDK and theme CSS into HTML content
func InjectSDK(html, appID, appName, userID, userName string) string {
	sdk := GenerateSDK(appID, appName, userID, userName)
	injection := ThemeCSS + sdk

	// Try to inject before </head>
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx > 0 {
		return html[:idx] + injection + html[idx:]
	}

	// Try to inject after <body>
	if idx := strings.Index(strings.ToLower(html), "<body"); idx > 0 {
		// Find the closing >
		closeIdx := strings.Index(html[idx:], ">")
		if closeIdx > 0 {
			insertPoint := idx + closeIdx + 1
			return html[:insertPoint] + injection + html[insertPoint:]
		}
	}

	// Fallback: prepend to content
	return injection + html
}

// HandleAPIRequest handles postMessage API requests from apps
func HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	// Get user session
	sess, _ := auth.TrySession(r)
	var userID string
	if sess != nil {
		userID = sess.Account
	}

	// Parse request
	var req struct {
		AppID  string                 `json:"appId"`
		Method string                 `json:"method"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid request",
		})
		return
	}

	// Require login for all API calls
	if userID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Login required",
		})
		return
	}

	// Verify app exists
	app := GetApp(req.AppID)
	if app == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "App not found",
		})
		return
	}

	// Route method
	var result interface{}
	var err error

	switch req.Method {
	case "db.get":
		key, _ := req.Params["key"].(string)
		if key == "" {
			err = fmt.Errorf("key required")
		} else {
			result, err = DBGet(req.AppID, userID, key)
		}

	case "db.set":
		key, _ := req.Params["key"].(string)
		value := req.Params["value"]
		if key == "" {
			err = fmt.Errorf("key required")
		} else {
			err = DBSet(req.AppID, userID, key, value)
			if err == nil {
				result = true
			}
		}

	case "db.delete":
		key, _ := req.Params["key"].(string)
		if key == "" {
			err = fmt.Errorf("key required")
		} else {
			err = DBDelete(req.AppID, userID, key)
			if err == nil {
				result = true
			}
		}

	case "db.list":
		result, err = DBList(req.AppID, userID)

	case "db.quota":
		used, limit, e := DBQuota(req.AppID, userID)
		err = e
		if err == nil {
			result = map[string]int64{"used": used, "limit": limit}
		}

	case "fetch":
		url, _ := req.Params["url"].(string)
		method, _ := req.Params["method"].(string)
		if method == "" {
			method = "GET"
		}
		if url == "" {
			err = fmt.Errorf("url required")
		} else {
			result, err = ProxyFetch(url, method)
		}

	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": result,
		})
	}
}

// ProxyFetch fetches a URL server-side to bypass CORS
func ProxyFetch(url, method string) (map[string]interface{}, error) {
	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %v", err)
	}

	// Set a reasonable user agent
	req.Header.Set("User-Agent", "Mu/1.0 (mu.xyz proxy)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %v", err)
	}
	defer resp.Body.Close()

	// Read body (limit to 1MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}

	// Convert headers to simple map
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return map[string]interface{}{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"text":       string(body),
		"headers":    headers,
	}, nil
}
