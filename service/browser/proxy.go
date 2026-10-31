package browser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"mu/internal/hosts"
)

// publicDial validates resolved addresses and dials the checked IP, avoiding a
// second DNS lookup. The proxy is the only network path configured in Chromium.
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	return dialPublic(ctx, network, address, net.DefaultResolver.LookupIPAddr, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
}

func dialPublic(ctx context.Context, network, address string, resolve func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port != "80" && port != "443" {
		return nil, fmt.Errorf("browser port is not allowed")
	}
	ips, err := resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no public address")
	}
	for _, ip := range ips {
		if hosts.PrivateIP(ip.IP) || ip.IP.IsMulticast() {
			return nil, fmt.Errorf("private browser destination")
		}
	}
	var last error
	for _, ip := range ips {
		conn, e := dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if e == nil {
			return conn, nil
		}
		last = e
	}
	return nil, last
}

// startProxy owns all tunnels, including hijacked CONNECT connections, so the
// caller's deadline and cleanup also terminate outstanding browser traffic.
func startProxy(ctx context.Context) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	transport := &http.Transport{DialContext: publicDial, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	closed := false
	track := func(c net.Conn) bool {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			c.Close()
			return false
		}
		connections[c] = true
		return true
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			upstream, e := publicDial(ctx, "tcp", r.Host)
			if e != nil {
				http.Error(w, "Destination refused", http.StatusForbidden)
				return
			}
			defer upstream.Close()
			if !track(upstream) {
				return
			}
			client, buffered, e := w.(http.Hijacker).Hijack()
			if e != nil {
				return
			}
			defer client.Close()
			if !track(client) {
				return
			}
			defer func() { mu.Lock(); delete(connections, client); delete(connections, upstream); mu.Unlock() }()
			client.SetDeadline(time.Now().Add(wait))
			upstream.SetDeadline(time.Now().Add(wait))
			if _, e = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); e != nil {
				return
			}
			done := make(chan struct{})
			go func() { io.Copy(upstream, buffered); upstream.Close(); close(done) }()
			io.Copy(client, upstream)
			client.Close()
			<-done
			return
		}
		if r.URL.Scheme != "http" || hosts.Fetchable(r.URL) != nil || r.Header.Get("Upgrade") != "" {
			http.Error(w, "Destination refused", http.StatusForbidden)
			return
		}
		req := r.Clone(ctx)
		req.RequestURI = ""
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Proxy-Connection")
		response, e := transport.RoundTrip(req)
		if e != nil {
			http.Error(w, "Destination refused", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		for k, values := range response.Header {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: wait}
	go server.Serve(listener)
	stop := func() {
		mu.Lock()
		closed = true
		for c := range connections {
			c.Close()
		}
		mu.Unlock()
		server.Close()
		transport.CloseIdleConnections()
	}
	return "http://" + listener.Addr().String(), stop, nil
}
