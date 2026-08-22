package proxy

import (
	"context"
	"crypto/x509"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPPolicyAndInjection(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	upstreamAddr := strings.TrimPrefix(upstream.URL, "http://")
	ca, e := NewCA()
	if e != nil {
		t.Fatal(e)
	}
	s := &Server{Policy: config.Egress{SchemaVersion: 1, Mode: "proxy_preferred", Default: "deny", Rules: []config.EgressRule{{Host: "secret.test", InjectSecret: &config.EgressCredential{Key: "TOKEN", Type: "bearer"}}}}, Credentials: map[string]string{"TOKEN": "actual-secret"}, CA: ca, Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, upstreamAddr)
	}}}
	req := httptest.NewRequest("GET", "http://secret.test/", nil)
	req.Header.Set("Authorization", "Bearer attacker-value")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Fatalf("status %d", rr.Code)
	}
	if gotAuth != "Bearer actual-secret" {
		t.Fatalf("auth = %q", gotAuth)
	}
}

func TestDisallowedHostDenied(t *testing.T) {
	ca, _ := NewCA()
	s := &Server{Policy: config.Egress{SchemaVersion: 1, Mode: "proxy_preferred", Default: "deny"}, CA: ca}
	req := httptest.NewRequest("GET", "http://evil.example/path", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("code %d", rr.Code)
	}
}
func TestCAFilesPrivate(t *testing.T) {
	dir := t.TempDir()
	ca, e := LoadOrCreateCA(dir)
	if e != nil || len(ca.CertPEM) == 0 {
		t.Fatalf("CA: %v", e)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.CertPEM) {
		t.Fatal("bad public cert")
	}
	info, e := os.Stat(filepath.Join(dir, "ca-key.pem"))
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private key permissions %o", info.Mode().Perm())
	}
	if strings.Contains(string(ca.CertPEM), "PRIVATE") {
		t.Fatal("public cert leaked private material")
	}
}

func TestInjectionTypesOverwriteClientValues(t *testing.T) {
	cases := []struct{ typ, header, want string }{{"basic", "", "Basic dXNlcjpzM2NyM3Q="}, {"header", "X-Api-Key", "user:s3cr3t"}}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			rule := config.EgressRule{Host: "api.test", InjectSecret: &config.EgressCredential{Key: "GITHUB_TOKEN", Type: tc.typ, Header: tc.header}}
			s := &Server{Policy: config.Egress{Default: "deny", Rules: []config.EgressRule{rule}}, Credentials: map[string]string{"GITHUB_TOKEN": "user:s3cr3t"}}
			r := httptest.NewRequest("GET", "http://api.test/", nil)
			r.Header.Set("Authorization", "attacker")
			r.Header.Set("X-Api-Key", "attacker")
			s.inject(r, "api.test")
			if tc.typ == "basic" {
				if got := r.Header.Get("Authorization"); got != tc.want {
					t.Fatalf("auth %q", got)
				}
			} else if got := r.Header.Get(tc.header); got != tc.want {
				t.Fatalf("header %q", got)
			}
		})
	}
}
func TestReloadCopiesCredentials(t *testing.T) {
	s := &Server{}
	input := map[string]string{"TOKEN": "first"}
	s.Reload(input)
	input["TOKEN"] = "mutated"
	s.mu.RLock()
	got := s.Credentials["TOKEN"]
	s.mu.RUnlock()
	if got != "first" {
		t.Fatalf("credentials aliased: %q", got)
	}
}
