package apps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/auth"
)

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

// InjectSDK injects the SDK into HTML content
func InjectSDK(html, appID, appName, userID, userName string) string {
	sdk := GenerateSDK(appID, appName, userID, userName)
	
	// Try to inject before </head>
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx > 0 {
		return html[:idx] + sdk + html[idx:]
	}
	
	// Try to inject after <body>
	if idx := strings.Index(strings.ToLower(html), "<body"); idx > 0 {
		// Find the closing >
		closeIdx := strings.Index(html[idx:], ">")
		if closeIdx > 0 {
			insertPoint := idx + closeIdx + 1
			return html[:insertPoint] + sdk + html[insertPoint:]
		}
	}
	
	// Fallback: prepend to content
	return sdk + html
}

// HandleAPIRequest handles postMessage API requests from apps
func HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	// Get user session
	sess, _ := auth.GetSession(r)
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
