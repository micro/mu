package server

import (
	"encoding/json"
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

func TestHostPaymentAvailability(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "test")
	for _, config := range []struct{ name, payTo, stripe string }{
		{"disabled", "", ""}, {"credits only", "", "test"}, {"x402", "0x123", ""},
	} {
		t.Run(config.name, func(t *testing.T) {
			t.Setenv("X402_PAY_TO", config.payTo)
			t.Setenv("STRIPE_SECRET_KEY", config.stripe)
			TestHostBrowserAndDiscovery(t)
			for _, path := range []string{"/", "/tools", "/pricing", "/llms.txt"} {
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
				body := w.Body.String()
				if config.payTo == "" {
					for _, promise := range []string{"pay per call with x402", "Pay per call with x402", "Priced calls return an HTTP 402", "Priced calls use HTTP 402", "$0.01 / call", "two ways to pay"} {
						if strings.Contains(body, promise) {
							t.Errorf("%s advertises disabled payments: %s", path, promise)
						}
					}
					if path == "/pricing" && !strings.Contains(body, "Direct x402 payments are not enabled") {
						t.Error("missing unavailable state")
					}
				} else if path == "/pricing" && !strings.Contains(body, "Priced calls return an HTTP 402") {
					t.Error("missing enabled payment option")
				}
				if path == "/pricing" && config.stripe != "" && !strings.Contains(body, "Monthly credits") {
					t.Error("disabled x402 hid credit subscriptions")
				}
			}
			r := httptest.NewRequest("GET", "https://m3o.test/pricing", nil)
			r.Header.Set("Accept", "application/json")
			w := httptest.NewRecorder()
			PricingHandler(w, r)
			var result struct {
				X402 bool `json:"x402"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.X402 != (config.payTo != "") {
				t.Error("incorrect JSON payment availability")
			}
		})
	}
}
