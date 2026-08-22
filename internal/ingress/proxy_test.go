package ingress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyCatalogAndRoute(t *testing.T) {
	p := NewProxy("kaas.test")
	p.SetRoutes([]Route{
		{Hostname: "api.demo.kaas.test", HostPort: 19001, Scheme: "http", Sandbox: "demo", Service: "api", GuestPort: 8080},
	})
	if !p.HasHostname("api.demo.kaas.test") {
		t.Fatal("expected hostname registered")
	}
	if !p.HasHostname("kaas.test") {
		t.Fatal("apex should be allowed")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://kaas.test/", nil)
	req.Host = "kaas.test"
	p.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("catalog status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "api.demo.kaas.test") {
		t.Fatalf("catalog missing service: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://missing.kaas.test/", nil)
	req.Host = "missing.kaas.test"
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown host status %d", rec.Code)
	}
}
