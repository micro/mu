package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

type androidRelease struct {
	URL     string
	Version string
	Checked bool
}

var androidDownload struct {
	sync.RWMutex
	release androidRelease
}
var downloadOnce sync.Once

// Refresh outside page requests: opening the landing never waits on GitHub.
func startDownloadRefresh() {
	downloadOnce.Do(func() {
		go func() {
			client := &http.Client{Timeout: 10 * time.Second}
			for {
				release, err := fetchAndroidRelease(context.Background(), client, "https://api.github.com/repos/micro/mu/releases/latest")
				if err == nil {
					androidDownload.Lock()
					androidDownload.release = release
					androidDownload.Unlock()
				}
				time.Sleep(30 * time.Minute)
			}
		}()
	})
}
func currentAndroidRelease() androidRelease {
	androidDownload.RLock()
	defer androidDownload.RUnlock()
	return androidDownload.release
}
func fetchAndroidRelease(ctx context.Context, client *http.Client, endpoint string) (androidRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return androidRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Mu-downloads")
	res, err := client.Do(req)
	if err != nil {
		return androidRelease{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return androidRelease{}, fmt.Errorf("release lookup returned %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return androidRelease{}, err
	}
	if len(body) > 1<<20 {
		return androidRelease{}, fmt.Errorf("release response too large")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return androidRelease{}, err
	}
	result := androidRelease{Checked: true}
	if release.Draft || release.Prerelease || release.Tag == "" {
		return result, nil
	}
	for _, asset := range release.Assets {
		// Release CI only gives this name to a release-signed APK. Never offer
		// unsigned or preview builds as the consumer installer.
		if asset.Name != "mu-android.apk" {
			continue
		}
		u, err := url.Parse(asset.URL)
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		if u.Path != "/micro/mu/releases/download/"+release.Tag+"/mu-android.apk" || strings.Contains(release.Tag, "/") {
			continue
		}
		result.URL = asset.URL
		result.Version = release.Tag
		break
	}
	return result, nil
}

func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	release := currentAndroidRelease()
	if r.URL.Path == "/download/android" && release.URL != "" {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, release.URL, http.StatusSeeOther)
		return
	}
	if r.URL.Path != "/download" && r.URL.Path != "/download/android" {
		http.NotFound(w, r)
		return
	}
	android := `<p>A signed Android download is not available yet. You can use the assistant in your browser.</p>`
	if !release.Checked {
		android = `<p>We could not confirm an Android download yet. Please check again shortly, or use the assistant in your browser.</p>`
	}
	if release.URL != "" {
		android = `<div class="form-actions">` + app.ActionLink("/download/android", "Download for Android") + `<span class="action-note">` + html.EscapeString(release.Version) + ` · APK</span></div><p>Open the downloaded file and allow installation from your browser when Android asks. Updates are installed from this page over your existing app.</p>`
	}
	body := `<h1>Use your assistant</h1><p>Open it in your browser or install it on your device.</p><div class="form-actions">` + app.ActionLink("/assistant", "Open assistant") + `</div>
<section class="page-section" id="android"><h2>Android</h2>` + android + `</section>
<section class="page-section" id="iphone"><h2>iPhone and iPad</h2><p>Open this site in Safari, tap Share, then Add to Home Screen. Enable Open as Web App if offered. No App Store download is needed.</p></section>
<section class="page-section" id="desktop"><h2>Desktop</h2><p>Use the assistant in your browser on Linux, macOS or Windows. Native desktop clients are not available yet.</p></section>
<section class="page-section"><h2>Run Mu yourself</h2><p>Host the runtime and web console on your own server. The Linux and macOS release binaries are server software, not desktop assistant apps.</p><div class="form-actions">` + app.ActionLink("/install", "Self-host Mu") + app.ActionLink("/developers", "Developer tools") + `</div></section>`
	_, acc := auth.TrySession(r)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, app.ConsoleHTML("Downloads", body, acc, "/download"))
}
