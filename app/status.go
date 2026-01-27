package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"mu/auth"
	"mu/data"
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
	UsedGB    float64 `json:"used_gb"`
	TotalGB   float64 `json:"total_gb"`
	Percent   float64 `json:"percent"`
}

// IndexStatus represents search index status
type IndexStatus struct {
	Entries    int  `json:"entries"`
	Embeddings int  `json:"embeddings"`
	VectorSearch bool `json:"vector_search"`
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
	// Quick health check endpoint
	if r.URL.Query().Get("quick") == "1" {
		w.Header().Set("Content-Type", "application/json")
		status := buildStatus()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"healthy": status.Healthy,
			"online":  status.OnlineUsers,
		})
		return
	}

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

	// Check Crypto Wallet/Payments
	walletSeedExists := os.Getenv("WALLET_SEED") != "" || fileExists(getWalletSeedPath())
	quotaMode := "Unlimited (self-hosted)"
	if walletSeedExists {
		quotaMode = "Pay-as-you-go (crypto)"
	}
	services = append(services, StatusCheck{
		Name:    "Payments",
		Status:  walletSeedExists,
		Details: quotaMode,
	})

	// Check Vector Search
	indexStats := data.GetStats()
	vectorMode := "Keyword (fallback)"
	if indexStats.EmbeddingsEnabled {
		vectorMode = fmt.Sprintf("Vector (%d embeddings)", indexStats.EmbeddingCount)
	}
	services = append(services, StatusCheck{
		Name:    "Search",
		Status:  indexStats.EmbeddingsEnabled,
		Details: vectorMode,
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
		OnlineUsers: auth.GetOnlineCount(),
		IndexStats: IndexStatus{
			Entries:      indexStats.TotalEntries,
			Embeddings:   indexStats.EmbeddingCount,
			VectorSearch: indexStats.EmbeddingsEnabled,
		},
	}
}

func formatDKIMDetails(enabled bool, domain, selector string) string {
	if !enabled {
		return "Not configured"
	}
	return fmt.Sprintf("%s (selector: %s)", domain, selector)
}

func checkLLMConfig() (provider string, configured bool) {
	var providers []string
	
	if os.Getenv("FANAR_API_KEY") != "" {
		providers = append(providers, "Fanar (default)")
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-haiku-4"
		}
		providers = append(providers, fmt.Sprintf("Anthropic/%s", model))
	}
	if os.Getenv("MODEL_API_URL") != "" || os.Getenv("OLLAMA_API_URL") != "" {
		model := os.Getenv("MODEL_NAME")
		if model == "" {
			model = "llama3.2"
		}
		providers = append(providers, fmt.Sprintf("Ollama/%s", model))
	}
	
	if len(providers) == 0 {
		return "Not configured", false
	}
	return strings.Join(providers, ", "), true
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getWalletSeedPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "wallet.seed"
	}
	return homeDir + "/.mu/keys/wallet.seed"
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
