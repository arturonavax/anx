package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultHTTPTransportConfig(t *testing.T) {
	tr := DefaultHTTPTransport()
	if tr == nil {
		t.Fatal("DefaultHTTPTransport() returned nil")
	}
	if tr.MaxIdleConns != 64 {
		t.Errorf("MaxIdleConns = %d, want 64", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 16 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 16", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 15*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 15s", tr.ResponseHeaderTimeout)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 = false, want true")
	}
	if tr.Proxy == nil {
		t.Error("Proxy is nil, want http.ProxyFromEnvironment")
	}
}

func TestNewHTTPClientDefaultsAndTimeout(t *testing.T) {
	cDefault := NewHTTPClient(0)
	if cDefault == nil {
		t.Fatal("NewHTTPClient(0) returned nil")
	}
	if cDefault.Timeout != 30*time.Second {
		t.Errorf("NewHTTPClient(0).Timeout = %v, want 30s", cDefault.Timeout)
	}

	cNeg := NewHTTPClient(-5 * time.Second)
	if cNeg.Timeout != 30*time.Second {
		t.Errorf("NewHTTPClient(-5s).Timeout = %v, want 30s", cNeg.Timeout)
	}

	cCustom := NewHTTPClient(12 * time.Second)
	if cCustom.Timeout != 12*time.Second {
		t.Errorf("NewHTTPClient(12s).Timeout = %v, want 12s", cCustom.Timeout)
	}

	tr, ok := cCustom.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("Transport = %T, want non-nil *http.Transport", cCustom.Transport)
	}
	if tr.MaxIdleConnsPerHost != 16 {
		t.Errorf("Transport.MaxIdleConnsPerHost = %d, want 16", tr.MaxIdleConnsPerHost)
	}

	// Verify the client performs requests cleanly against a local server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := cCustom.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
