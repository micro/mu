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
	Desktop map[string]string
	WebURL  string
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
	result := androidRelease{Checked: true, Desktop: map[string]string{}}
	if release.Draft || release.Prerelease || release.Tag == "" {
		return result, nil
	}
	for _, asset := range release.Assets {
		// Release CI only gives this name to a release-signed APK. Never offer
		// unsigned or preview builds as the consumer installer.
		desktop := map[string]bool{"mu-windows-x64-setup.exe": true, "mu-macos-arm64.dmg": true, "mu-macos-x64.dmg": true, "mu-linux-x86_64.AppImage": true, "mu-linux-amd64.deb": true}
		if asset.Name != "mu-android.apk" && asset.Name != "mu-web.zip" && !desktop[asset.Name] {
			continue
		}
		u, err := url.Parse(asset.URL)
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		if u.Path != "/micro/mu/releases/download/"+release.Tag+"/"+asset.Name || strings.Contains(release.Tag, "/") {
			continue
		}
		if desktop[asset.Name] {
			result.Desktop[asset.Name] = asset.URL
		} else if asset.Name == "mu-web.zip" {
			result.WebURL = asset.URL
		} else {
			result.URL = asset.URL
		}
		result.Version = release.Tag
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
		android = `<div class="section-body"><div class="form-actions">` + app.ActionLink("/download/android", "Download for Android") + `<span class="action-note">` + html.EscapeString(release.Version) + ` · APK</span></div><p>Open the downloaded file and allow installation from your browser when Android asks. Updates are installed from this page over your existing app.</p></div>`
	}
	desktopHTML := ""
	for _, platform := range []struct {
		name   string
		assets []struct{ file, label string }
	}{
		{"Windows", []struct{ file, label string }{{"mu-windows-x64-setup.exe", "Download for Windows"}}},
		{"Mac", []struct{ file, label string }{{"mu-macos-arm64.dmg", "Apple silicon"}, {"mu-macos-x64.dmg", "Intel"}}},
		{"Linux", []struct{ file, label string }{{"mu-linux-amd64.deb", "Download .deb"}, {"mu-linux-x86_64.AppImage", "Download AppImage"}}},
	} {
		links := ""
		for _, asset := range platform.assets {
			if url := release.Desktop[asset.file]; url != "" {
				links += app.ActionLink(url, asset.label)
			}
		}
		if links != "" {
			desktopHTML += `<section class="page-section"><h2>Mu for ` + platform.name + `</h2><div class="form-actions">` + links + `</div></section>`
		}
	}
	body := `<h1>Mu apps</h1><p>Use Mu in your browser or download the app for your device.</p>
<section class="page-section" id="web"><h2>Mu Web</h2><p>Talk to your agents in your browser, on your phone, tablet or computer.</p><div class="form-actions">` + app.ActionLink("/app/", "Open Mu Web") + `</div></section>
<section class="page-section" id="android"><h2>Mu for Android</h2>` + android + `</section>
` + desktopHTML + `<section class="page-section" id="iphone"><h2>iPhone and iPad</h2><p>Use <a href="/app/">Mu Web</a> in Safari. To keep Home, Inbox, Agents and Services on your Home Screen, open <a href="/home">Mu Home</a>, tap Share, then Add to Home Screen.</p></section>
<p class="text-muted">Want to run Mu yourself? See the <a href="/install">setup guide</a>.</p>`
	_, acc := auth.TrySession(r)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, app.ConsoleHTML("Downloads", body, acc, "/download"))
}
