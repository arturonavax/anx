package platform

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const defaultHTTPClientTimeout = 30 * time.Second

// sharedHTTPTransport lazily initializes a single pooled transport reused across
// clients created by NewHTTPClient so keep-alive connections and HTTP/2 streams
// are shared instead of leaking idle connection pools per call.
var sharedHTTPTransport = sync.OnceValue(DefaultHTTPTransport)

// DefaultHTTPTransport returns a freshly configured *http.Transport tuned for
// concurrent downloads and API queries with bounded idle pools and explicit timeouts.
func DefaultHTTPTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// NewHTTPClient constructs an *http.Client backed by the shared pooled transport.
// When timeout <= 0, it defaults to 30 seconds.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedHTTPTransport(),
	}
}
