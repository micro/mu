package browser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProxyRejectsInternalDestinations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address, stop, err := startProxy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	proxy, _ := url.Parse(address)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
	for _, target := range []string{"http://127.0.0.1/", "http://localhost/", "http://[::1]/", "https://127.0.0.1/", "http://169.254.169.254/", "http://10.0.0.1/"} {
		response, e := client.Get(target)
		if e == nil {
			response.Body.Close()
			if response.StatusCode < 400 {
				t.Fatalf("allowed %s", target)
			}
		}
	}
}

func TestProxyRejectsInvalidTargets(t *testing.T) {
	// Exercise the handler's redirect follow-up independently of external DNS.
	address, stop, err := startProxy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("accepted origin-form proxy request")
	}
	for _, host := range []string{"127.0.0.1:80", "localhost:443", "[::1]:80", "169.254.169.254:80", "example.com:22"} {
		if conn, e := publicDial(context.Background(), "tcp", host); e == nil {
			conn.Close()
			t.Fatalf("allowed %s", host)
		}
	}
}

func TestRemoteBrowserFailsClosed(t *testing.T) {
	t.Setenv("BROWSER_URL", "http://127.0.0.1:9222")
	if Configured() {
		t.Fatal("remote browser enabled")
	}
	if _, err := checked("https://example.com"); err == nil {
		t.Fatal("accepted remote browser")
	}
}

// Keep a real HTTP request/response fixture for proxy rejection, without opening
// any production service or cloud metadata endpoint.
func TestProxyNeverTouchesPrivateServer(t *testing.T) {
	touched := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { touched = true; io.WriteString(w, "private") }))
	defer target.Close()
	address, stop, err := startProxy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	p, _ := url.Parse(address)
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(p)}}
	r, err := c.Get(target.URL)
	if err == nil {
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "private") && r.StatusCode == 200 {
			t.Fatal("private content returned")
		}
	}
	if touched {
		t.Fatal("private server contacted")
	}
}

func TestDialPinsCheckedAddressAndRejectsMixedDNS(t *testing.T) {
	called := ""
	dial := func(_ context.Context, _ string, address string) (net.Conn, error) {
		called = address
		return nil, fmt.Errorf("test dial")
	}
	resolve := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	_, _ = dialPublic(context.Background(), "tcp", "public.example:443", resolve, dial)
	if called != "93.184.216.34:443" {
		t.Fatalf("DNS re-resolution possible: %s", called)
	}
	called = ""
	mixed := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := dialPublic(context.Background(), "tcp", "public.example:443", mixed, dial); err == nil || called != "" {
		t.Fatal("mixed private/public DNS was dialed")
	}
}
