package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/service"
	"mu/internal/tool"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/places"
	"mu/service/text"
	"mu/service/weather"
	"mu/service/web"
)

func TestHostBrowserAndDiscovery(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	for _, spec := range []service.Spec{web.Spec, weather.Spec, markets.Spec, news.Spec, places.Spec, text.Spec} {
		if _, ok := service.SpecFor(spec.Name); !ok {
			if err := service.Register(spec); err != nil {
				t.Fatal(err)
			}
		}
	}
	tool.DeriveTools()
	for _, path := range []string{"/", "/tools", "/tools/web_search", "/pricing", "/mcp"} {
		r := httptest.NewRequest("GET", "https://m3o.test"+path, nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		if path == "/" {
			X402IndexHandler(w, r)
		} else if path == "/pricing" {
			PricingHandler(w, r)
		} else {
			http.DefaultServeMux.ServeHTTP(w, r)
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `class="x402-site"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if path == "/" && strings.Count(w.Body.String(), `class="directory-row directory-content"`) != 6 {
			t.Fatal("landing must link six registered tools")
		}
	}
	r := httptest.NewRequest("GET", "https://m3o.test/", nil)
	w := httptest.NewRecorder()
	X402IndexHandler(w, r)
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || !strings.Contains(w.Body.String(), "https://m3o.test/mcp") {
		t.Fatal("plain discovery changed")
	}
	for _, path := range []string{"/tools?q=weather_forecast&service=weather", "/tools?q=no-such-tool-xyz"} {
		r = httptest.NewRequest("GET", "https://m3o.test"+path, nil)
		r.Header.Set("Accept", "text/html")
		w = httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(w, r)
		if strings.Contains(path, "weather") {
			if !strings.Contains(w.Body.String(), `href="/tools/weather_forecast"`) || strings.Contains(w.Body.String(), `href="/tools/web_search"`) {
				t.Fatal("tool filters did not intersect")
			}
		} else if !strings.Contains(w.Body.String(), "No matching tools") {
			t.Fatal("missing empty state")
		}
	}
	r = httptest.NewRequest("GET", "https://micro.test/tools", nil)
	w = httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), `class="x402-site"`) {
		t.Fatal("host theme leaked into consumer")
	}
}
