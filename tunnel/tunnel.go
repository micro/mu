// Package tunnel provides a web-based proxy for browsing through the server
package tunnel

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/app"
)

var client = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Don't follow redirects - we'll rewrite them
		return http.ErrUseLastResponse
	},
}

// Handler serves the tunnel page and proxies requests
func Handler(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")

	if targetURL == "" {
		// Show the tunnel form
		showForm(w, r, "")
		return
	}

	// Validate and normalize URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		showForm(w, r, "Invalid URL")
		return
	}

	// Block localhost/internal IPs
	if isBlockedHost(parsed.Host) {
		showForm(w, r, "Cannot proxy internal addresses")
		return
	}

	// Fetch the target
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		showForm(w, r, "Failed to create request")
		return
	}

	// Set headers to look like a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-GB,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		showForm(w, r, fmt.Sprintf("Failed to fetch: %v", err))
		return
	}
	defer resp.Body.Close()

	// Handle redirects
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location != "" {
			// Make absolute if relative
			if !strings.HasPrefix(location, "http") {
				base, _ := url.Parse(targetURL)
				ref, _ := url.Parse(location)
				location = base.ResolveReference(ref).String()
			}
			http.Redirect(w, r, "/tunnel?url="+url.QueryEscape(location), http.StatusFound)
			return
		}
	}

	contentType := resp.Header.Get("Content-Type")

	// For non-HTML content, just proxy it directly
	if !strings.Contains(contentType, "text/html") {
		w.Header().Set("Content-Type", contentType)
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			reader, _ = gzip.NewReader(resp.Body)
		}
		io.Copy(w, reader)
		return
	}

	// Read and rewrite HTML
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			showForm(w, r, "Failed to decompress response")
			return
		}
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		showForm(w, r, "Failed to read response")
		return
	}

	// Show page with toolbar and link interceptor
	showProxiedPage(w, r, targetURL, string(body))
}

func showForm(w http.ResponseWriter, r *http.Request, errMsg string) {
	var errorDiv string
	if errMsg != "" {
		errorDiv = fmt.Sprintf(`<div class="error">%s</div>`, errMsg)
	}

	content := fmt.Sprintf(`
		<div class="tunnel-form">
			<h2>Web Tunnel</h2>
			<p class="desc">Browse through this server. Traffic exits from the UK.</p>
			%s
			<form method="get" action="/tunnel">
				<input type="text" name="url" placeholder="Enter URL (e.g., example.com)" autocomplete="off" autofocus>
				<button type="submit">Go</button>
			</form>
		</div>
	`, errorDiv)

	pageHTML := app.Page(app.PageOpts{
		Content: content,
	})
	w.Write([]byte(app.RenderHTML("Tunnel", "Browse through the UK", pageHTML)))
}

func showProxiedPage(w http.ResponseWriter, r *http.Request, targetURL string, content string) {
	parsedURL, _ := url.Parse(targetURL)
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host

	// Inject script to intercept link clicks
	interceptScript := fmt.Sprintf(`<script data-tunnel="true">
(function() {
	const tunnelBase = '/tunnel?url=';
	const originalBase = '%s';
	
	function resolveURL(href) {
		if (!href) return null;
		// Skip special protocols
		if (href.startsWith('javascript:') || href.startsWith('mailto:') || 
		    href.startsWith('tel:') || href.startsWith('#') || href.startsWith('data:')) {
			return null;
		}
		// Already absolute
		if (href.startsWith('http://') || href.startsWith('https://')) {
			return href;
		}
		// Protocol-relative
		if (href.startsWith('//')) {
			return 'https:' + href;
		}
		// Absolute path
		if (href.startsWith('/')) {
			return originalBase + href;
		}
		// Relative path - resolve against current URL
		const currentPath = '%s';
		const lastSlash = currentPath.lastIndexOf('/');
		const basePath = lastSlash > 0 ? currentPath.substring(0, lastSlash + 1) : currentPath + '/';
		return basePath + href;
	}
	
	// Intercept all link clicks
	document.addEventListener('click', function(e) {
		let target = e.target;
		while (target && target.tagName !== 'A') {
			target = target.parentElement;
		}
		if (!target) return;
		
		// Get the raw href attribute, not the resolved one
		const rawHref = target.getAttribute('href');
		if (!rawHref) return;
		
		const resolved = resolveURL(rawHref);
		if (!resolved) return;
		
		e.preventDefault();
		e.stopPropagation();
		window.location.href = tunnelBase + encodeURIComponent(resolved);
	}, true);
	
	// Intercept form submissions
	document.addEventListener('submit', function(e) {
		const form = e.target;
		if (!form.method || form.method.toLowerCase() === 'get') {
			e.preventDefault();
			const formData = new FormData(form);
			const params = new URLSearchParams(formData).toString();
			const rawAction = form.getAttribute('action') || '';
			const resolved = resolveURL(rawAction) || '%s';
			const fullUrl = resolved + (resolved.includes('?') ? '&' : '?') + params;
			window.location.href = tunnelBase + encodeURIComponent(fullUrl);
		}
	}, true);
})();
</script>`, baseURL, targetURL, targetURL)

	// Inject the script before </body> or at end
	if idx := strings.LastIndex(strings.ToLower(content), "</body>"); idx != -1 {
		content = content[:idx] + interceptScript + content[idx:]
	} else {
		content = content + interceptScript
	}

	// Add toolbar at top
	toolbar := fmt.Sprintf(`
<div id="mu-tunnel-toolbar" style="position:fixed;top:0;left:0;right:0;height:44px;background:#1a1a1a;display:flex;align-items:center;padding:0 12px;gap:12px;z-index:2147483647;font-family:system-ui,-apple-system,sans-serif;box-shadow:0 2px 8px rgba(0,0,0,0.3);">
	<a href="/tunnel" style="color:#888;text-decoration:none;font-size:20px;line-height:1;">&times;</a>
	<form method="get" action="/tunnel" style="flex:1;display:flex;gap:8px;margin:0;">
		<input type="text" name="url" value="%s" style="flex:1;padding:8px 12px;border:1px solid #333;border-radius:4px;background:#0a0a0a;color:#fff;font-size:14px;outline:none;">
		<button type="submit" style="padding:8px 16px;border:none;border-radius:4px;background:#333;color:#fff;cursor:pointer;font-size:14px;">Go</button>
	</form>
</div>
<style>#mu-tunnel-toolbar + * { margin-top: 44px !important; } body { padding-top: 44px !important; }</style>`, targetURL)

	// Inject toolbar after <body> tag
	bodyIdx := strings.Index(strings.ToLower(content), "<body")
	if bodyIdx != -1 {
		// Find the closing > of the body tag
		closeIdx := strings.Index(content[bodyIdx:], ">")
		if closeIdx != -1 {
			insertPos := bodyIdx + closeIdx + 1
			content = content[:insertPos] + toolbar + content[insertPos:]
		}
	} else {
		content = toolbar + content
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(content))
}

func isBlockedHost(host string) bool {
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	host = strings.ToLower(host)

	blocked := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"::1",
		"10.",
		"192.168.",
		"172.16.", "172.17.", "172.18.", "172.19.",
		"172.20.", "172.21.", "172.22.", "172.23.",
		"172.24.", "172.25.", "172.26.", "172.27.",
		"172.28.", "172.29.", "172.30.", "172.31.",
	}

	for _, b := range blocked {
		if host == b || strings.HasPrefix(host, b) {
			return true
		}
	}

	return false
}
