package mail

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type ownerTransport func(*http.Request) (*http.Response, error)

func (f ownerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestIPOwnerRegistryAndValidation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: ownerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "rdap.arin.net" {
			t.Fatal("unexpected registry")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"GOOGLE","entities":[{"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Google LLC"]]]}]}`)), Header: make(http.Header)}, nil
	})}
	if got := lookupIPOwner(context.Background(), client, "209.85.220.41"); got != "Google LLC" {
		t.Fatal(got)
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "::1", "169.254.169.254", "https://example.com"} {
		if lookupIPOwner(context.Background(), client, ip) != "" {
			t.Fatal(ip)
		}
	}
	if calls != 1 {
		t.Fatal("invalid IP reached registry")
	}
}
func TestDMARCShowsCachedOwnerSafely(t *testing.T) {
	ipOwners.Lock()
	old := ipOwners.entries
	ipOwners.entries = map[string]ipOwnerEntry{"209.85.220.41": {name: "Google <LLC>", expires: time.Now().Add(time.Hour)}}
	ipOwners.Unlock()
	defer func() { ipOwners.Lock(); ipOwners.entries = old; ipOwners.Unlock() }()
	output := renderDMARCReport(`<feedback><record><row><source_ip>209.85.220.41</source_ip><count>1</count></row></record></feedback>`)
	if !strings.Contains(output, "Network owner") || !strings.Contains(output, "Google &lt;LLC&gt;") {
		t.Fatal(output)
	}
}
