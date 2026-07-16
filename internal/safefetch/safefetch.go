// Package safefetch is an SSRF-guarded HTTP client for fetching untrusted URLs
// on behalf of user code (micro apps). It refuses non-public destinations —
// loopback, private ranges, link-local (incl. the cloud metadata address),
// multicast — validating both the initial URL and every redirect hop, and caps
// response size and time.
//
// It respects an outbound proxy via the environment (HTTP(S)_PROXY). When no
// proxy is configured — the normal self-hosted case — connections go directly to
// the resolved address, so the public-IP check is authoritative.
package safefetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrBlocked is returned when a destination is not a public host.
var ErrBlocked = errors.New("blocked: destination is not a public host")

const (
	// DefaultTimeout bounds a whole fetch (connect + read).
	DefaultTimeout = 10 * time.Second
	// DefaultMaxBytes caps the response body read into memory.
	DefaultMaxBytes = 2 << 20 // 2 MiB
	maxRedirects    = 5
)

// isBlockedIP reports whether an address must not be reached: loopback, private,
// link-local (169.254/fe80, which includes the 169.254.169.254 metadata
// endpoint), multicast, or unspecified.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// validateURL parses raw, requires an http(s) scheme, and rejects it unless the
// host is a public address (or resolves entirely to public addresses).
func validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https urls are allowed")
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, ErrBlocked
		}
		return u, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("cannot resolve host")
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, ErrBlocked
		}
	}
	return u, nil
}

// Options tunes a fetch. Zero values fall back to safe defaults.
type Options struct {
	Method   string
	Headers  map[string]string
	Body     string
	MaxBytes int64
	Timeout  time.Duration
}

// Response is the guarded fetch result handed back to the caller.
type Response struct {
	Status  int               `json:"status"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers,omitempty"`
}

var hopHeaders = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true,
	"proxy-authorization": true, "te": true, "trailer": true,
	"transfer-encoding": true, "upgrade": true, "host": true,
}

// Fetch performs a guarded request and returns the (size-capped) response.
func Fetch(ctx context.Context, raw string, opt Options) (*Response, error) {
	if _, err := validateURL(raw); err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(opt.Method))
	if method == "" {
		method = "GET"
	}
	switch method {
	case "GET", "POST", "HEAD":
	default:
		return nil, fmt.Errorf("method not allowed")
	}
	maxBytes := opt.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if opt.Body != "" {
		body = strings.NewReader(opt.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return nil, err
	}
	for k, v := range opt.Headers {
		if hopHeaders[strings.ToLower(k)] {
			continue
		}
		req.Header.Set(k, v)
	}

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	// When the request goes direct (no proxy), pin the connection to a resolved
	// public IP at dial time — this closes the DNS-rebinding gap the pre-flight
	// check alone can't (a host that resolves public now, private at connect).
	// When a proxy is configured we trust it for egress (and it may itself be a
	// loopback address), so the pre-flight + redirect checks are the guard.
	if p, _ := http.ProxyFromEnvironment(req); p == nil {
		transport.DialContext = guardedDial
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			if _, err := validateURL(r.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	headers := map[string]string{}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers["Content-Type"] = ct
	}
	return &Response{Status: resp.StatusCode, Body: string(data), Headers: headers}, nil
}

// guardedDial resolves the target and connects only to a public IP, rejecting
// private/loopback/link-local addresses at dial time. Installed on the transport
// only for direct (non-proxied) requests.
func guardedDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: DefaultTimeout}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, ErrBlocked
		}
		return dialer.DialContext(ctx, network, address)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, a := range addrs {
		if isBlockedIP(a.IP) {
			return nil, ErrBlocked
		}
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrBlocked
}
