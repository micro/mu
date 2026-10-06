package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type ipOwnerEntry struct {
	name    string
	expires time.Time
}

var ipOwners = struct {
	sync.Mutex
	entries map[string]ipOwnerEntry
}{entries: map[string]ipOwnerEntry{}}
var registryClient = &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
	if len(via) > 4 || r.URL.Scheme != "https" {
		return fmt.Errorf("invalid registry redirect")
	}
	switch r.URL.Hostname() {
	case "rdap.arin.net", "rdap.db.ripe.net", "rdap.apnic.net", "rdap.lacnic.net", "rdap.afrinic.net":
		return nil
	}
	return fmt.Errorf("unknown registry")
}}

// Report lookups share a deadline and a bounded cache. Only IP addresses go to
// the registry; message contents and account details never leave the server.
func reportIPOwners(report DMARCReport) map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := map[string]string{}
	pending := []string{}
	ipOwners.Lock()
	for _, r := range report.Records {
		ip := strings.TrimSpace(r.Row.SourceIP)
		if _, seen := result[ip]; seen {
			continue
		}
		result[ip] = ""
		if entry, ok := ipOwners.entries[ip]; ok && time.Now().Before(entry.expires) {
			result[ip] = entry.name
			continue
		}
		addr, err := netip.ParseAddr(ip)
		if err != nil || !addr.IsGlobalUnicast() || addr.IsPrivate() {
			continue
		}
		if len(pending) < 16 {
			pending = append(pending, ip)
		}
	}
	ipOwners.Unlock()
	type answer struct{ ip, name string }
	answers := make(chan answer, len(pending))
	slots := make(chan struct{}, 4)
	for _, ip := range pending {
		go func(ip string) {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				answers <- answer{ip, ""}
				return
			}
			defer func() { <-slots }()
			name := lookupIPOwner(ctx, registryClient, ip)
			ttl := 24 * time.Hour
			if name == "" {
				ttl = 10 * time.Minute
			}
			ipOwners.Lock()
			if len(ipOwners.entries) >= 1024 {
				ipOwners.entries = map[string]ipOwnerEntry{}
			}
			ipOwners.entries[ip] = ipOwnerEntry{name, time.Now().Add(ttl)}
			ipOwners.Unlock()
			answers <- answer{ip, name}
		}(ip)
	}
	for range pending {
		select {
		case a := <-answers:
			result[a.ip] = a.name
		case <-ctx.Done():
			return result
		}
	}
	return result
}

func lookupIPOwner(ctx context.Context, client *http.Client, ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rdap.arin.net/registry/ip/"+addr.String(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/rdap+json")
	rsp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return ""
	}
	var record struct {
		Name     string `json:"name"`
		Entities []struct {
			Roles []string          `json:"roles"`
			VCard []json.RawMessage `json:"vcardArray"`
		} `json:"entities"`
	}
	if json.NewDecoder(io.LimitReader(rsp.Body, 1<<20)).Decode(&record) != nil {
		return ""
	}
	for _, e := range record.Entities {
		registrant := false
		for _, role := range e.Roles {
			if role == "registrant" {
				registrant = true
			}
		}
		if !registrant || len(e.VCard) != 2 {
			continue
		}
		var fields [][]json.RawMessage
		if json.Unmarshal(e.VCard[1], &fields) != nil {
			continue
		}
		for _, f := range fields {
			if len(f) < 4 {
				continue
			}
			var key, value string
			json.Unmarshal(f[0], &key)
			json.Unmarshal(f[3], &value)
			if key == "fn" && strings.TrimSpace(value) != "" {
				return ownerLabel(value)
			}
		}
	}
	return ownerLabel(record.Name)
}

func ownerLabel(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > 120 {
		r = r[:120]
	}
	return string(r)
}
