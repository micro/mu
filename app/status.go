package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

var startTime = time.Now()

// StatusCheck represents a single status check result
type StatusCheck struct {
	Name    string `json:"name"`
	Status  bool   `json:"status"`
	Details string `json:"details,omitempty"`
}

// StatusResponse represents the full status response
type StatusResponse struct {
	Healthy   bool          `json:"healthy"`
	Uptime    string        `json:"uptime"`
	GoVersion string        `json:"go_version"`
	Memory    MemoryStatus  `json:"memory"`
	Services  []StatusCheck `json:"services"`
	Config    []StatusCheck `json:"config"`
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

// StatusHandler handles the /status endpoint
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	// Build status response
	status := buildStatus()

	// Check format
	if r.URL.Query().Get("format") == "json" || WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	// Render HTML
	html := renderStatusHTML(status)
	w.Write([]byte(RenderHTML("Status", "Server status and health checks", html)))
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

	// Check YouTube API
	youtubeConfigured := os.Getenv("YOUTUBE_API_KEY") != ""
	services = append(services, StatusCheck{
		Name:   "YouTube API",
		Status: youtubeConfigured,
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

	// Check DNS records
	if mailDomain != "" && mailDomain != "localhost" && mailSelector != "" {
		dkimDNS := checkDKIMDNS(mailSelector, mailDomain)
		config = append(config, StatusCheck{
			Name:   "DKIM DNS Record",
			Status: dkimDNS,
		})

		spfDNS := checkSPFDNS(mailDomain)
		config = append(config, StatusCheck{
			Name:   "SPF DNS Record",
			Status: spfDNS,
		})
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
		Services: services,
		Config:   config,
	}
}

func formatDKIMDetails(enabled bool, domain, selector string) string {
	if !enabled {
		return "Not configured"
	}
	return fmt.Sprintf("%s (selector: %s)", domain, selector)
}

func checkLLMConfig() (provider string, configured bool) {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-3-5-sonnet-20241022"
		}
		return fmt.Sprintf("Anthropic (%s)", model), true
	}
	if os.Getenv("FANAR_API_KEY") != "" {
		return "Fanar", true
	}
	if os.Getenv("OLLAMA_API_URL") != "" {
		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			model = "default"
		}
		return fmt.Sprintf("Ollama (%s)", model), true
	}
	return "Not configured", false
}

func maskDomain(domain string) string {
	if domain == "" || domain == "localhost" {
		return domain
	}
	return domain
}

func checkDKIMDNS(selector, domain string) bool {
	record := fmt.Sprintf("%s._domainkey.%s", selector, domain)
	txts, err := net.LookupTXT(record)
	if err != nil {
		return false
	}
	for _, txt := range txts {
		if strings.Contains(txt, "v=DKIM1") {
			return true
		}
	}
	return false
}

func checkSPFDNS(domain string) bool {
	txts, err := net.LookupTXT(domain)
	if err != nil {
		return false
	}
	for _, txt := range txts {
		if strings.Contains(txt, "v=spf1") {
			return true
		}
	}
	return false
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

	sb.WriteString(`<style>
.status-page { max-width: 600px; }
.status-header { display: flex; align-items: center; gap: 10px; margin-bottom: 30px; }
.status-ok { color: #4caf50; }
.status-error { color: #f44336; }
.status-warn { color: #ff9800; }
.status-section { margin-bottom: 30px; }
.status-section h3 { border-bottom: 1px solid #eee; padding-bottom: 10px; margin-bottom: 15px; }
.status-item { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #f5f5f5; }
.status-item:last-child { border-bottom: none; }
.status-name { font-weight: 500; }
.status-value { display: flex; align-items: center; gap: 8px; }
.status-icon { font-size: 16px; }
.status-details { color: #666; font-size: 13px; }
.system-info { display: grid; grid-template-columns: repeat(2, 1fr); gap: 15px; }
.system-info-item { background: #f9f9f9; padding: 15px; border-radius: 8px; }
.system-info-label { font-size: 12px; color: #666; text-transform: uppercase; }
.system-info-value { font-size: 18px; font-weight: 600; margin-top: 5px; }
</style>`)

	sb.WriteString(`<div class="status-page">`)

	// Header
	sb.WriteString(fmt.Sprintf(`<div class="status-header">
<span class="%s status-icon">%s</span>
<span style="font-size: 18px;">%s</span>
</div>`, statusClass, statusIcon, statusText))

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
<div class="system-info-label">Goroutines</div>
<div class="system-info-value">` + fmt.Sprintf("%d", status.Memory.Goroutines) + `</div>
</div>
<div class="system-info-item">
<div class="system-info-label">Go Version</div>
<div class="system-info-value">` + strings.TrimPrefix(status.GoVersion, "go") + `</div>
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
