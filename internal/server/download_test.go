package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAndroidDownloadOnlyOffersStableReleaseAsset(t *testing.T) {
	for _, tc := range []struct {
		name, body       string
		available, valid bool
	}{
		{"signed", `{"tag_name":"v3.0.0","assets":[{"name":"mu-android.apk","browser_download_url":"https://github.com/micro/mu/releases/download/v3.0.0/mu-android.apk"}]}`, true, true},
		{"unsigned", `{"tag_name":"v3.0.0","assets":[{"name":"mu-android-unsigned.apk","browser_download_url":"https://github.com/micro/mu/releases/download/v3.0.0/mu-android-unsigned.apk"}]}`, false, true},
		{"preview", `{"tag_name":"v3.0.0","prerelease":true,"assets":[{"name":"mu-android.apk","browser_download_url":"https://github.com/micro/mu/releases/download/v3.0.0/mu-android.apk"}]}`, false, true},
		{"foreign", `{"tag_name":"v3.0.0","assets":[{"name":"mu-android.apk","browser_download_url":"https://example.org/mu-android.apk"}]}`, false, true},
		{"wrong tag", `{"tag_name":"v3.0.0","assets":[{"name":"mu-android.apk","browser_download_url":"https://github.com/micro/mu/releases/download/v2.0.0/mu-android.apk"}]}`, false, true},
		{"invalid", "not json", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.body)) }))
			defer server.Close()
			got, err := fetchAndroidRelease(context.Background(), server.Client(), server.URL)
			if (err == nil) != tc.valid || (got.URL != "") != tc.available {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}
func TestDownloadPageAndRedirect(t *testing.T) {
	old := currentAndroidRelease()
	defer func() { androidDownload.Lock(); androidDownload.release = old; androidDownload.Unlock() }()
	for _, available := range []bool{false, true} {
		release := androidRelease{Checked: true}
		if available {
			release.URL = "https://github.com/micro/mu/releases/download/v3.0.0/mu-android.apk"
			release.Version = "v3.0.0"
		}
		androidDownload.Lock()
		androidDownload.release = release
		androidDownload.Unlock()
		for _, path := range []string{"/download", "/download/android"} {
			w := httptest.NewRecorder()
			DownloadHandler(w, httptest.NewRequest("GET", path, nil))
			if available && path == "/download/android" {
				if w.Code != 303 || w.Header().Get("Location") != release.URL {
					t.Fatal("wrong download redirect")
				}
				continue
			}
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			for _, text := range []string{"iPhone and iPad", "Mu Web", "Open Mu Web", "setup guide", "/app/"} {
				if !strings.Contains(w.Body.String(), text) {
					t.Fatalf("missing %s", text)
				}
			}
			if strings.Contains(w.Body.String(), ">Download for Android</a>") != available {
				t.Fatal("installer advertised incorrectly")
			}
		}
	}
	w := httptest.NewRecorder()
	DownloadHandler(w, httptest.NewRequest("POST", "/download/android", nil))
	if w.Code != 405 {
		t.Fatal("accepted POST")
	}
}

func TestWebReleaseUsesItsOwnVerifiedAsset(t *testing.T) {
	for _, url := range []string{"https://github.com/micro/mu/releases/download/v3.0.0/mu-web.zip", "https://example.org/mu-web.zip", "https://github.com/micro/mu/releases/download/v3.0.0/mu-android.apk"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"tag_name":"v3.0.0","assets":[{"name":"mu-web.zip","browser_download_url":"` + url + `"}]}`))
		}))
		got, err := fetchAndroidRelease(context.Background(), server.Client(), server.URL)
		server.Close()
		valid := url == "https://github.com/micro/mu/releases/download/v3.0.0/mu-web.zip"
		if err != nil || (got.WebURL != "") != valid || got.URL != "" {
			t.Fatalf("web asset: %+v %v", got, err)
		}
	}
}

func TestDesktopDownloadsOnlyUseExpectedReleaseAssets(t *testing.T) {
	for _, filename := range []string{"mu-windows-x64-setup.exe", "mu-macos-arm64.dmg", "mu-macos-x64.dmg", "mu-linux-amd64.deb", "mu-linux-x86_64.AppImage"} {
		for _, host := range []string{"https://github.com/micro/mu/releases/download/v3.0.0/", "https://example.com/"} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"tag_name":"v3.0.0","assets":[{"name":"` + filename + `","browser_download_url":"` + host + filename + `"}]}`))
			}))
			got, err := fetchAndroidRelease(context.Background(), server.Client(), server.URL)
			server.Close()
			if err != nil || (got.Desktop[filename] != "") != strings.HasPrefix(host, "https://github.com/") {
				t.Fatalf("%s %s: %v", filename, host, err)
			}
		}
	}
}
