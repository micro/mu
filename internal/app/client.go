package app

// Who a request is from, when nobody has signed in.
//
// Two questions that look like one. "Which machine is this" is what a rate
// limit needs to stop a loop, and "which person is this" is what a fair share
// needs — and an IP answers the first badly and the second not at all.
//
// # The header was trusted from anyone
//
// ClientIP reads X-Forwarded-For, which is right behind a reverse proxy and
// wrong everywhere else: the header is set by whoever sent the request. An
// instance reachable from the internet took a client's word for its own
// address, so every limit keyed on it was one header away from unlimited —
// the guest allowance, and the three-signups-per-address rule that is the only
// thing standing between a hundred welcome grants and one script.
//
// So the header is believed when the hop it came from is one we put there, and
// otherwise ignored. The default is that a private or loopback peer is a proxy
// and a public one is not, which is the shape of every ordinary deployment:
// nginx or Caddy on the same host or the same network. A load balancer with a
// public address needs naming — TRUSTED_PROXY, a comma-separated list of CIDRs
// — and an operator behind Cloudflare who does not set it gets their proxy's
// address for everybody, which is wrong in the safe direction.
//
// # And an address is not a person
//
// A cafe, a university, a phone network behind carrier-grade NAT: one address,
// hundreds of people. A ration per address is not a ration per person there —
// it is a race, and the first loop wins it for everybody. The other way round
// too: one person on a phone changes address between the train and the office
// and gets a fresh allowance for doing nothing.
//
// So a browser gets a mark of its own on the way in, and the ration is per
// mark. It is a random id in a cookie and it is exactly as trustworthy as that
// sounds — anybody can drop it and get a new one — which is why the address
// limit stays. Two ceilings doing two jobs: the mark is the fair share, and it
// is generous because it is one person; the address is the backstop, and it is
// wide because it may be a building.
//
// Nothing is stored against the mark. It is not a profile, it is not written
// down, it does not follow anybody between instances, and it is dropped when a
// browser closes. See ClientID.

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// clientCookie is what the mark is called.
//
// Prefixed the way the session cookie is not, because this one is worth being
// able to recognise in a browser's storage inspector as ours and as not a
// login.
const clientCookie = "mu_client"

// trustedProxies is the parsed TRUSTED_PROXY list, and whether the default is
// in force.
//
// Parsed once. It is read on every request and the list does not change while
// the process runs.
var (
	proxyOnce sync.Once
	proxyNets []*net.IPNet
)

func loadTrustedProxies() {
	for _, part := range strings.Split(os.Getenv("TRUSTED_PROXY"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// A bare address is a host route, so an operator can name one load
		// balancer without knowing what a /32 is.
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				part += "/" + itoa(bits)
			}
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			proxyNets = append(proxyNets, n)
		} else {
			Log("http", "TRUSTED_PROXY: ignoring %q, which is not an address or a CIDR", part)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// fromTrustedProxy reports whether the peer is a hop we put in front of this.
//
// TRUSTED_PROXY when an operator has named one. Otherwise loopback and the
// private ranges, which is where a reverse proxy on the same host or the same
// network sits — and which nothing on the public internet can be, so a direct
// caller can never talk its way into being believed.
func fromTrustedProxy(r *http.Request) bool {
	proxyOnce.Do(loadTrustedProxies)

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	if len(proxyNets) > 0 {
		for _, n := range proxyNets {
			if n.Contains(ip) {
				return true
			}
		}
		// A named list is the whole list. An operator who says which hop to
		// believe has not also said "and any private one".
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// PeerIP is the address the connection actually came from, headers ignored.
//
// What a log should say, and what a limit falls back to. Never spoofable,
// because it is the socket.
func PeerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// forwardedIP is the client address a trusted proxy reported, or "".
//
// The first entry in X-Forwarded-For is the original client; the rest are the
// hops. X-Real-If is checked second because some proxies set only that one.
func forwardedIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			xff = xff[:i]
		}
		if ip := strings.TrimSpace(xff); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" && net.ParseIP(xr) != nil {
		return xr
	}
	return ""
}

// MarkClient gives this browser a mark if it does not have one.
//
// Called once, in the middleware, before anything asks who a request is from —
// so every gate downstream can read the mark off the request and none of them
// needs a ResponseWriter to do it.
//
// A session cookie: no Max-Age, so it lasts as long as the browser is open.
// That is the right lifetime for what it is for. A ration exists to stop one
// visit becoming a loop, not to remember somebody for a fortnight, and a
// persistent id on a product whose whole claim is no tracking would be a
// tracking cookie however honestly it was used.
//
// HttpOnly, so a script cannot read it; SameSite=Lax, so it is not sent along
// with somebody else's form post.
func MarkClient(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(clientCookie); err == nil && len(c.Value) >= 16 {
		return
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return // no mark is better than a predictable one
	}
	id := base64.RawURLEncoding.EncodeToString(b[:])

	http.SetCookie(w, &http.Cookie{
		Name:     clientCookie,
		Value:    id,
		Path:     "/",
		Secure:   r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Readable by the rest of this request, which is the one that was marked.
	// Without this the first call a new visitor makes has no mark and falls
	// through to the address limit alone — on a page that asks a question on
	// arrival, that is most first questions.
	r.AddCookie(&http.Cookie{Name: clientCookie, Value: id})
}

// ClientID is this browser's mark, or "" if it has none.
//
// Empty is a real answer and callers must handle it: a client that refuses
// cookies, a tool posting to the API, the first request of all. Those fall back
// to the address limit, which is why that one still exists.
func ClientID(r *http.Request) string {
	if c, err := r.Cookie(clientCookie); err == nil && len(c.Value) >= 16 {
		return c.Value
	}
	return ""
}

// ── The two ceilings ────────────────────────────────────────────

type rateBucket struct {
	count   int
	resetAt time.Time
}

var (
	rateMu sync.Mutex
	rates  = map[string]*rateBucket{}
)

// allow counts one call against a key and says whether it was allowed.
func allow(key string, max int, window time.Duration) bool {
	if max <= 0 {
		return false
	}
	rateMu.Lock()
	defer rateMu.Unlock()

	now := time.Now()
	b, ok := rates[key]
	if !ok || now.After(b.resetAt) {
		b = &rateBucket{resetAt: now.Add(window)}
		rates[key] = b
	}
	if b.count >= max {
		return false
	}
	b.count++

	if len(rates) > 20000 {
		for k, v := range rates {
			if now.After(v.resetAt) {
				delete(rates, k)
			}
		}
	}
	return true
}

// GuestAllowed reports whether an unauthenticated caller may make another free
// call, and counts it when they may.
//
// Both ceilings, and the call has to clear both.
//
//   - The mark, GUEST_MAX_PER_CLIENT (default 40) an hour, is the fair share.
//     One browser, so it can be sized for a person rather than for a building.
//   - The address, GUEST_MAX_PER_IP (default 300) an hour, is the backstop. It
//     is wide because it may be a cafe or a phone network, and it is there for
//     the caller who drops the mark to get a new one — which resets the first
//     ceiling and not this one.
//
// The address limit used to be the only one and was 120, sized as if an address
// were a person. It is both too tight for a shared one and too loose for a
// script that keeps its cookies, which is what having two of these fixes.
//
// Localhost is never limited, the same as before: that is a self-hosted
// instance or a developer, and both are the operator.
func GuestAllowed(r *http.Request) bool {
	ip := ClientIP(r)
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	window := time.Duration(EnvInt("GUEST_WINDOW_MINUTES", 60)) * time.Minute

	// The mark first, so a browser that is over its own share does not spend
	// the address's allowance finding that out.
	if id := ClientID(r); id != "" {
		if !allow("c:"+id, EnvInt("GUEST_MAX_PER_CLIENT", 40), window) {
			return false
		}
	}
	return allow("ip:"+ip, EnvInt("GUEST_MAX_PER_IP", 300), window)
}

// resetRates is for the tests, which have to start from nothing.
func resetRates() {
	rateMu.Lock()
	defer rateMu.Unlock()
	rates = map[string]*rateBucket{}
}

// resetProxies is for the tests, which have to be able to change TRUSTED_PROXY
// and have it read again. Kept beside the once it resets, so the coupling is
// visible from here rather than only from the test file.
func resetProxies() {
	proxyOnce = sync.Once{}
	proxyNets = nil
}
