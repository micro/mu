package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
)

var startTime = time.Now()

// CacheStatsFunc is injected by ai package to avoid import cycle
var CacheStatsFunc func() (hits, misses, readTokens, creationTokens int)

// StatusCheck represents a single status check result
type StatusCheck struct {
	Name    string `json:"name"`
	Status  bool   `json:"status"`
	Details string `json:"details,omitempty"`
}

// StatusResponse represents the full status response
type StatusResponse struct {
	Healthy     bool          `json:"healthy"`
	Uptime      string        `json:"uptime"`
	GoVersion   string        `json:"go_version"`
	Memory      MemoryStatus  `json:"memory"`
	Disk        DiskStatus    `json:"disk"`
	Services    []StatusCheck `json:"services"`
	Config      []StatusCheck `json:"config"`
	OnlineUsers int           `json:"online_users"`
	IndexStats  IndexStatus   `json:"index"`
}

// DiskStatus represents disk usage
type DiskStatus struct {
	UsedGB  float64 `json:"used_gb"`
	TotalGB float64 `json:"total_gb"`
	Percent float64 `json:"percent"`
}

// IndexStatus represents search index status
type IndexStatus struct {
	Entries int `json:"entries"`
}

// MemoryStatus represents memory usage
type MemoryStatus struct {
	Alloc      uint64 `json:"alloc_mb"`
	Sys        uint64 `json:"sys_mb"`
	NumGC      uint32 `json:"num_gc"`
	Goroutines int    `json:"goroutines"`
}

// DKIMStatusFunc is set by main to avoid import cycle
var DKIMStatusFunc func() (enabled bool, domain, selector string)

// DigestStatusFunc is set by main to report digest status
var DigestStatusFunc func() (ok bool, details string)

// ServiceHealth represents a public-facing service health check
type ServiceHealth struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
	Path   string `json:"path,omitempty"`
}

// PublicStatusResponse is the public status page response
type PublicStatusResponse struct {
	Healthy  bool            `json:"healthy"`
	Services []ServiceHealth `json:"services"`
}

// HealthCheckFunc is set by main to run service health checks (avoids import cycles)
var HealthCheckFunc func() []ServiceHealth

// StatusHandler handles the public /status endpoint
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	status := checkPublicStatus()

	if r.URL.Query().Get("format") == "json" || WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	Respond(w, r, Response{Title: "Status", Description: "Service status",
		HTML: renderPublicStatusHTML(status)})
}

func checkPublicStatus() PublicStatusResponse {
	var services []ServiceHealth
	if HealthCheckFunc != nil {
		services = HealthCheckFunc()
	}
	healthy := true
	for _, s := range services {
		if !s.Status {
			healthy = false
			break
		}
	}
	return PublicStatusResponse{
		Healthy:  healthy,
		Services: services,
	}
}

func renderPublicStatusHTML(status PublicStatusResponse) string {
	var sb strings.Builder

	statusText := "All systems operational"
	statusClass := "status-ok"
	if !status.Healthy {
		statusText = "Some services are experiencing issues"
		statusClass = "status-error"
	}

	sb.WriteString(`<div class="status-page">`)

	// Header
	sb.WriteString(fmt.Sprintf(`<div class="status-header">
<span class="status-icon %s" style="font-size:24px;">●</span>
<span style="font-size:18px;">%s</span>
</div>`, statusClass, statusText))

	// Services
	sb.WriteString(`<div class="status-section">`)
	for _, svc := range status.Services {
		icon := "●"
		class := "status-ok"
		if !svc.Status {
			class = "status-error"
		}
		pathAttr := ""
		if svc.Path != "" {
			pathAttr = fmt.Sprintf(` data-path="%s"`, svc.Path)
		}
		sb.WriteString(fmt.Sprintf(`<div class="status-item"%s>
<span class="status-name">%s</span>
<span class="status-value"><span class="status-latency"></span><span class="status-icon %s">%s</span></span>
</div>`, pathAttr, svc.Name, class, icon))
	}
	sb.WriteString(`</div>`)

	// Client-side latency checks
	sb.WriteString(`<script>
document.querySelectorAll('.status-item[data-path]').forEach(function(el) {
  var path = el.getAttribute('data-path');
  var span = el.querySelector('.status-latency');
  var start = performance.now();
  fetch(path, {method:'HEAD',cache:'no-store'}).then(function() {
    var ms = Math.round(performance.now() - start);
    span.textContent = ms + 'ms';
    span.className = 'status-latency status-details';
  }).catch(function() {
    span.textContent = 'error';
    span.className = 'status-latency status-details';
  });
});
</script>`)

	sb.WriteString(`</div>`)
	return sb.String()
}

// RenderInternalStatusHTML returns the internal status HTML for embedding in the admin server page
func RenderInternalStatusHTML() string {
	status := buildStatus()
	return renderStatusHTML(status)
}

func buildStatus() StatusResponse {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	services := []StatusCheck{}
	config := []StatusCheck{}

	// Check DKIM
	if DKIMStatusFunc != nil {
		enabled, domain, selector := DKIMStatusFunc()
		services = append(services, StatusCheck{
			Name:    "DKIM Signing",
			Status:  enabled,
			Details: formatDKIMDetails(enabled, domain, selector),
		})
	}

	// Check SMTP server (port configured)
	smtpPort := os.Getenv("MAIL_PORT")
	if smtpPort == "" {
		smtpPort = "2525"
	}
	services = append(services, StatusCheck{
		Name:    "SMTP Server",
		Status:  smtpPort != "",
		Details: fmt.Sprintf("Port %s", smtpPort),
	})

	// Check LLM provider
	llmProvider, llmConfigured := checkLLMConfig()
	services = append(services, StatusCheck{
		Name:    "LLM Provider",
		Status:  llmConfigured,
		Details: llmProvider,
	})

	// Add cache stats if Anthropic is configured (stats injected via CacheStatsFunc)
	if os.Getenv("ANTHROPIC_API_KEY") != "" && CacheStatsFunc != nil {
		hits, misses, readTokens, _ := CacheStatsFunc()
		total := hits + misses
		var cacheDetails string
		if total > 0 {
			hitRate := float64(hits) / float64(total) * 100
			cacheDetails = fmt.Sprintf("%.0f%% hit rate (%d/%d), %dk tokens saved", hitRate, hits, total, readTokens/1000)
		} else {
			cacheDetails = "No requests yet"
		}
		services = append(services, StatusCheck{
			Name:    "Prompt Cache",
			Status:  true,
			Details: cacheDetails,
		})
	}

	// Check YouTube API
	youtubeConfigured := os.Getenv("YOUTUBE_API_KEY") != ""
	services = append(services, StatusCheck{
		Name:   "YouTube API",
		Status: youtubeConfigured,
	})

	// Check Google Places API
	googleConfigured := os.Getenv("GOOGLE_API_KEY") != ""
	services = append(services, StatusCheck{
		Name:   "Google Places API",
		Status: googleConfigured,
	})

	// Check Payments. Credits are the one thing a caller pays in, so this
	// reports whether they can be bought rather than enumerating rails.
	stripeConfigured := os.Getenv("STRIPE_SECRET_KEY") != "" && os.Getenv("STRIPE_PUBLISHABLE_KEY") != ""
	paymentsConfigured := stripeConfigured
	quotaMode := "Unlimited (self-hosted)"
	if stripeConfigured {
		quotaMode = "Credits, topped up by card"
	}
	services = append(services, StatusCheck{
		Name:    "Payments",
		Status:  paymentsConfigured,
		Details: quotaMode,
	})

	// Check Daily Digest
	if DigestStatusFunc != nil {
		ok, details := DigestStatusFunc()
		services = append(services, StatusCheck{
			Name:    "Daily Digest",
			Status:  ok,
			Details: details,
		})
	}

	// Check Search Index
	indexStats := data.GetStats()
	searchStatus := indexStats.TotalEntries > 0
	searchDetails := fmt.Sprintf("FTS5 (%d entries)", indexStats.TotalEntries)
	services = append(services, StatusCheck{
		Name:    "Search",
		Status:  searchStatus,
		Details: searchDetails,
	})

	// Configuration checks
	mailDomain := os.Getenv("MAIL_DOMAIN")
	config = append(config, StatusCheck{
		Name:    "MAIL_DOMAIN",
		Status:  mailDomain != "" && mailDomain != "localhost",
		Details: maskDomain(mailDomain),
	})

	mailSelector := os.Getenv("MAIL_SELECTOR")
	config = append(config, StatusCheck{
		Name:    "MAIL_SELECTOR",
		Status:  mailSelector != "",
		Details: mailSelector,
	})

	// Check DNS records — from the cache, never on the request. See dnsAnswer.
	if mailDomain != "" && mailDomain != "localhost" && mailSelector != "" {
		config = append(config, dnsStatus("DKIM DNS Record",
			mailSelector+"._domainkey."+mailDomain, "v=DKIM1"))
		config = append(config, dnsStatus("SPF DNS Record", mailDomain, "v=spf1"))
	}

	// Calculate overall health
	healthy := true
	for _, s := range services {
		if s.Name == "DKIM Signing" && !s.Status {
			healthy = false
		}
		if s.Name == "LLM Provider" && !s.Status {
			healthy = false
		}
	}

	// Get disk usage
	diskUsed, diskTotal, diskPercent := getDiskUsage()

	return StatusResponse{
		Healthy:   healthy,
		Uptime:    formatUptime(time.Since(startTime)),
		GoVersion: runtime.Version(),
		Memory: MemoryStatus{
			Alloc:      m.Alloc / 1024 / 1024,
			Sys:        m.Sys / 1024 / 1024,
			NumGC:      m.NumGC,
			Goroutines: runtime.NumGoroutine(),
		},
		Disk: DiskStatus{
			UsedGB:  float64(diskUsed) / 1024 / 1024 / 1024,
			TotalGB: float64(diskTotal) / 1024 / 1024 / 1024,
			Percent: diskPercent,
		},
		Services:    services,
		Config:      config,
		OnlineUsers: auth.OnlineCount(),
		IndexStats: IndexStatus{
			Entries: indexStats.TotalEntries,
		},
	}
}

func formatDKIMDetails(enabled bool, domain, selector string) string {
	if !enabled {
		return "Not configured"
	}
	return fmt.Sprintf("%s (selector: %s)", domain, selector)
}

// LLMStatus is set by main.go to internal/ai's own view of the model: which
// provider a question would go to, and whether it is answering. A hook because
// internal/ai imports this package, so the dependency cannot run the other way.
var LLMStatus func() (string, bool)

func checkLLMConfig() (provider string, configured bool) {
	if LLMStatus != nil {
		return LLMStatus()
	}
	return "Not configured", false
}

// getDiskUsage returns disk usage for the data directory
func getDiskUsage() (used, total uint64, percent float64) {
	dir := os.ExpandEnv("$HOME/.mu/data")

	// Try to get disk stats using syscall
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, 0, 0
	}

	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free
	percent = float64(used) / float64(total) * 100
	return
}

func maskDomain(domain string) string {
	if domain == "" || domain == "localhost" {
		return domain
	}
	return domain
}

// A DNS lookup is not a page render.
//
// Two questions were asked live on every draw of /status and /admin/server: is
// the DKIM record published, and is there an SPF record. `net.LookupTXT` has no
// deadline, and against a resolver that is not answering it retries until the
// stub gives up — seconds each, twice, on a page whose other work is
// microseconds. That is what a twenty-five second /admin/server was, and
// /status is public, so anyone could hold a request open that long.
//
// A published DNS record changes about once in the life of an instance, so the
// answer keeps for dnsTTL and a stale one is refreshed behind the page rather
// than in front of it. Nothing here ever blocks: the first ask on a cold
// process starts the lookup and says so, and the next render has the answer.
const (
	dnsTTL     = 15 * time.Minute
	dnsTimeout = 5 * time.Second
)

type dnsAnswerState struct {
	ok      bool
	checked time.Time // zero until the first lookup returns
	asking  bool
}

var (
	dnsMu      sync.Mutex
	dnsAnswers = map[string]*dnsAnswerState{}
)

// dnsAnswer reports whether a TXT record containing want is published at
// question, and whether that has been established yet.
func dnsAnswer(question, want string) (ok, known bool) {
	dnsMu.Lock()
	defer dnsMu.Unlock()

	a := dnsAnswers[question]
	if a == nil {
		a = &dnsAnswerState{}
		dnsAnswers[question] = a
	}
	if !a.asking && time.Since(a.checked) > dnsTTL {
		a.asking = true
		go lookupDNS(question, want)
	}
	return a.ok, !a.checked.IsZero()
}

// lookupDNS asks, with a deadline, and records what came back.
func lookupDNS(question, want string) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()

	found := false
	txts, err := net.DefaultResolver.LookupTXT(ctx, question)
	if err == nil {
		for _, txt := range txts {
			if strings.Contains(txt, want) {
				found = true
				break
			}
		}
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()
	a := dnsAnswers[question]
	if a == nil {
		return
	}
	a.ok = found
	a.checked = time.Now()
	a.asking = false
}

// dnsStatus is one of those answers as a row on the status page. An answer that
// has not come back yet says so rather than reporting a missing record, because
// "not checked" and "not published" are different things and only one of them
// is somebody's problem.
func dnsStatus(name, question, want string) StatusCheck {
	ok, known := dnsAnswer(question, want)
	c := StatusCheck{Name: name, Status: ok}
	if !known {
		c.Details = "checking…"
	}
	return c
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func renderStatusHTML(status StatusResponse) string {
	var sb strings.Builder

	// Overall status
	statusIcon := "✓"
	statusClass := "status-ok"
	statusText := "Healthy"
	if !status.Healthy {
		statusIcon = "✗"
		statusClass = "status-error"
		statusText = "Issues Detected"
	}

	sb.WriteString(`<div class="status-page">`)

	// Header
	sb.WriteString(fmt.Sprintf(`<div class="status-header">
<span class="%s status-icon">%s</span>
<span style="font-size: 18px;">%s</span>
</div>`, statusClass, statusIcon, statusText))

	// Disk warning
	diskWarning := ""
	if status.Disk.Percent > 90 {
		diskWarning = ` style="color: #f44336;"`
	} else if status.Disk.Percent > 75 {
		diskWarning = ` style="color: #ff9800;"`
	}

	// System Info
	sb.WriteString(`<div class="status-section">
<h3>System</h3>
<div class="system-info">
<div class="system-info-item">
<div class="system-info-label">Uptime</div>
<div class="system-info-value">` + status.Uptime + `</div>
</div>
<div class="system-info-item">
<div class="system-info-label">Memory</div>
<div class="system-info-value">` + fmt.Sprintf("%dMB / %dMB", status.Memory.Alloc, status.Memory.Sys) + `</div>
</div>
<div class="system-info-item">
<div class="system-info-label">Disk</div>
<div class="system-info-value"` + diskWarning + `>` + fmt.Sprintf("%.1fGB / %.1fGB (%.0f%%)", status.Disk.UsedGB, status.Disk.TotalGB, status.Disk.Percent) + `</div>
</div>
<div class="system-info-item">
<div class="system-info-label">Online Users</div>
<div class="system-info-value">` + fmt.Sprintf("%d", status.OnlineUsers) + `</div>
</div>
<div class="system-info-item">
<div class="system-info-label">Index Entries</div>
<div class="system-info-value">` + fmt.Sprintf("%d", status.IndexStats.Entries) + `</div>
</div>
</div>
</div>`)

	// Services
	sb.WriteString(`<div class="status-section">
<h3>Services</h3>`)
	for _, svc := range status.Services {
		icon := "✓"
		class := "status-ok"
		if !svc.Status {
			icon = "✗"
			class = "status-error"
		}
		details := ""
		if svc.Details != "" {
			details = fmt.Sprintf(`<span class="status-details">%s</span>`, svc.Details)
		}
		sb.WriteString(fmt.Sprintf(`<div class="status-item">
<span class="status-name">%s</span>
<span class="status-value">%s<span class="status-icon %s">%s</span></span>
</div>`, svc.Name, details, class, icon))
	}
	sb.WriteString(`</div>`)

	// Configuration
	sb.WriteString(`<div class="status-section">
<h3>Configuration</h3>`)
	for _, cfg := range status.Config {
		icon := "✓"
		class := "status-ok"
		if !cfg.Status {
			icon = "✗"
			class = "status-error"
		}
		details := ""
		if cfg.Details != "" {
			details = fmt.Sprintf(`<span class="status-details">%s</span>`, cfg.Details)
		}
		sb.WriteString(fmt.Sprintf(`<div class="status-item">
<span class="status-name">%s</span>
<span class="status-value">%s<span class="status-icon %s">%s</span></span>
</div>`, cfg.Name, details, class, icon))
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`</div>`)

	return sb.String()
}
