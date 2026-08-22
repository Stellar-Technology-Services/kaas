// Package proxy provides KaaS's host-owned, policy-enforcing HTTP(S) proxy.
// It deliberately supports only HTTP/1.x requests and MITM CONNECT; blind
// tunnels are rejected so they cannot bypass host/injection policy.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/egress"
)

type Server struct {
	Policy      config.Egress
	Credentials map[string]string
	CA          *CA
	Transport   http.RoundTripper
	mu          sync.RWMutex
}

// CA is proxy-private signing material. CertPEM is safe to install in a guest;
// KeyPEM must remain in a host-owned 0600 runtime directory.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

func NewCA() (*CA, error) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, e
	}
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "KaaS Egress Proxy CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(5, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if e != nil {
		return nil, e
	}
	return &CA{cert: tmpl, key: key, CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), KeyPEM: pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: mustMarshal(key)})}, nil
}
func mustMarshal(k *ecdsa.PrivateKey) []byte {
	b, e := x509.MarshalECPrivateKey(k)
	if e != nil {
		panic(e)
	}
	return b
}

// LoadOrCreateCA persists host-only CA material with restrictive permissions.
func LoadOrCreateCA(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	cp, kp := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem")
	cb, ce := os.ReadFile(cp)
	kb, ke := os.ReadFile(kp)
	if ce == nil && ke == nil {
		block, _ := pem.Decode(cb)
		if block == nil {
			return nil, fmt.Errorf("invalid proxy CA certificate")
		}
		cert, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			return nil, e
		}
		block, _ = pem.Decode(kb)
		if block == nil {
			return nil, fmt.Errorf("invalid proxy CA key")
		}
		key, e := x509.ParseECPrivateKey(block.Bytes)
		if e != nil {
			return nil, e
		}
		return &CA{cert: cert, key: key, CertPEM: cb, KeyPEM: kb}, nil
	}
	if !os.IsNotExist(ce) && ce != nil {
		return nil, ce
	}
	if !os.IsNotExist(ke) && ke != nil {
		return nil, ke
	}
	ca, e := NewCA()
	if e != nil {
		return nil, e
	}
	if e = os.WriteFile(cp, ca.CertPEM, 0644); e != nil {
		return nil, e
	}
	if e = os.WriteFile(kp, ca.KeyPEM, 0600); e != nil {
		return nil, e
	}
	return ca, nil
}
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.connect(w, r)
		return
	}
	host := requestHost(r)
	if !s.allow(host, w) {
		return
	}
	s.forward(w, r)
}
func requestHost(r *http.Request) string {
	h := r.URL.Hostname()
	if h == "" {
		h = r.Host
	}
	// URL.Hostname has already removed any port. Only parse r.Host when it
	// supplied the host; preserve a bare DNS host on SplitHostPort failure.
	if r.URL.Hostname() == "" {
		if parsed, _, err := net.SplitHostPort(h); err == nil {
			h = parsed
		}
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}
func (s *Server) allow(host string, w http.ResponseWriter) bool {
	if !egress.Decide(s.Policy, host).Allowed {
		http.Error(w, "egress denied", http.StatusForbidden)
		return false
	}
	return true
}
func (s *Server) inject(r *http.Request, host string) {
	d := egress.Decide(s.Policy, host)
	if d.Rule == nil || d.Rule.InjectSecret == nil {
		return
	}
	c := d.Rule.InjectSecret
	s.mu.RLock()
	v := s.Credentials[c.Key]
	s.mu.RUnlock()
	if v == "" {
		return
	}
	switch c.Type {
	case "bearer":
		r.Header.Set("Authorization", "Bearer "+v)
	case "basic":
		// A basic SecretSpec value is `username:password`; the key name is
		// never transmitted as an identity.
		user, password, ok := strings.Cut(v, ":")
		if !ok {
			return
		}
		r.SetBasicAuth(user, password)
	case "header":
		r.Header.Set(c.Header, v)
	}
}
func (s *Server) forward(w http.ResponseWriter, r *http.Request) {
	host := requestHost(r)
	out := r.Clone(r.Context())
	out.RequestURI = ""
	if !out.URL.IsAbs() {
		out.URL.Scheme = "http"
		out.URL.Host = r.Host
	}
	out.Header = out.Header.Clone()
	out.Header.Del("Proxy-Authorization")
	s.inject(out, host)
	tr := s.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	resp, e := tr.RoundTrip(out)
	if e != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
func copyHeader(dst, src http.Header) {
	for k, v := range src {
		for _, x := range v {
			dst.Add(k, x)
		}
	}
}
func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	host, _, e := net.SplitHostPort(r.Host)
	if e != nil {
		http.Error(w, "CONNECT requires host:port", 400)
		return
	}
	if !s.allow(strings.ToLower(host), w) {
		return
	}
	if s.CA == nil {
		http.Error(w, "TLS interception unavailable", 503)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unavailable", 500)
		return
	}
	conn, _, e := hj.Hijack()
	if e != nil {
		return
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	cert, e := s.leaf(host)
	if e != nil {
		return
	}
	tlsConn := tlsServer(conn, cert)
	if e = tlsConn.Handshake(); e != nil {
		return
	}
	defer tlsConn.Close()
	br := newBuffered(tlsConn)
	for {
		req, e := http.ReadRequest(br)
		if e != nil {
			return
		}
		req.URL.Scheme = "https"
		req.URL.Host = r.Host
		req.Host = r.Host
		if !s.allow(host, discardWriter{}) {
			writeResponse(tlsConn, 403, "egress denied")
			return
		}
		var buf bytes.Buffer
		rw := responseRecorder{header: http.Header{}, body: &buf, status: 200}
		s.forward(&rw, req)
		resp := &http.Response{StatusCode: rw.status, Status: fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status)), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: rw.header, Body: io.NopCloser(&buf), ContentLength: int64(buf.Len()), Request: req}
		if e := resp.Write(tlsConn); e != nil {
			return
		}
	}
}

func (s *Server) leaf(host string) (tls.Certificate, error) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return tls.Certificate{}, e
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(0, 3, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, s.CA.cert, &key.PublicKey, s.CA.key)
	if e != nil {
		return tls.Certificate{}, e
	}
	return tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: mustMarshal(key)}))
}
func tlsServer(c net.Conn, cert tls.Certificate) *tls.Conn {
	return tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
}
func newBuffered(r io.Reader) *bufio.Reader { return bufio.NewReader(r) }

type discardWriter struct{}

func (discardWriter) Header() http.Header       { return http.Header{} }
func (discardWriter) Write([]byte) (int, error) { return 0, nil }
func (discardWriter) WriteHeader(int)           {}

type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	status int
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(n int)           { r.status = n }
func writeResponse(w io.Writer, status int, message string) {
	_, _ = fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s", status, http.StatusText(status), len(message), message)
}

// Reload atomically replaces credentials held solely by this host process.
func (s *Server) Reload(credentials map[string]string) {
	copy := make(map[string]string, len(credentials))
	for k, v := range credentials {
		copy[k] = v
	}
	s.mu.Lock()
	s.Credentials = copy
	s.mu.Unlock()
}

// URL returns a loopback URL suitable for guest proxy configuration in
// proxy_preferred mode. Binding is intentionally host-controlled.
func Listen(ctx context.Context, addr string, s *Server) (string, func() error, error) {
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return "", nil, e
	}
	h := http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() { <-ctx.Done(); _ = h.Shutdown(context.Background()) }()
	go h.Serve(ln)
	return "http://" + ln.Addr().String(), h.Close, nil
}

var _ = url.URL{}
