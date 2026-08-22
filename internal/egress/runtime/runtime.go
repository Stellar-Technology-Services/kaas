package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/egress/proxy"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type Bootstrap struct {
	Policy      config.Egress     `json:"policy"`
	Credentials map[string]string `json:"credentials"`
	Listen      string            `json:"listen"`
}
type Request struct {
	Action      string            `json:"action"`
	Credentials map[string]string `json:"credentials,omitempty"`
}
type Status struct {
	Running       bool   `json:"running"`
	PID           int    `json:"pid,omitempty"`
	Listen        string `json:"listen,omitempty"`
	CAFingerprint string `json:"ca_fingerprint,omitempty"`
	Error         string `json:"error,omitempty"`
}

func Dir(root, name string) string    { return filepath.Join(root, name) }
func Socket(root, name string) string { return filepath.Join(Dir(root, name), "control.sock") }
func Fingerprint(c *proxy.CA) string {
	block, _ := pem.Decode(c.CertPEM)
	if block == nil {
		return ""
	}
	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
func Serve(ctx context.Context, root, name string, in io.Reader) error {
	d := Dir(root, name)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	var b Bootstrap
	if e := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&b); e != nil {
		return fmt.Errorf("read private bootstrap: %w", e)
	}
	ca, e := proxy.LoadOrCreateCA(d)
	if e != nil {
		return e
	}
	s := &proxy.Server{Policy: b.Policy, Credentials: b.Credentials, CA: ca}
	url, closeHTTP, e := proxy.Listen(ctx, b.Listen, s)
	if e != nil {
		return e
	}
	defer closeHTTP()
	sock := Socket(root, name)
	_ = os.Remove(sock)
	ln, e := net.Listen("unix", sock)
	if e != nil {
		return e
	}
	defer ln.Close()
	go func() { <-ctx.Done(); _ = ln.Close() }()
	_ = os.Chmod(sock, 0600)
	_ = os.WriteFile(filepath.Join(d, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	defer os.Remove(filepath.Join(d, "pid"))
	st := Status{Running: true, PID: os.Getpid(), Listen: url, CAFingerprint: Fingerprint(ca)}
	for {
		c, e := ln.Accept()
		if e != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return e
			}
		}
		go handle(c, s, st)
	}
}
func handle(c net.Conn, s *proxy.Server, st Status) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	var r Request
	if json.NewDecoder(io.LimitReader(c, 1<<20)).Decode(&r) != nil {
		return
	}
	switch r.Action {
	case "reload":
		s.Reload(r.Credentials)
		_ = json.NewEncoder(c).Encode(st)
	case "status":
		_ = json.NewEncoder(c).Encode(st)
	case "stop":
		_ = json.NewEncoder(c).Encode(st)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	default:
		_ = json.NewEncoder(c).Encode(Status{Error: "unknown action"})
	}
}
func Call(root, name string, r Request) (Status, error) {
	c, e := net.DialTimeout("unix", Socket(root, name), 2*time.Second)
	if e != nil {
		return Status{}, e
	}
	defer c.Close()
	if e = json.NewEncoder(c).Encode(r); e != nil {
		return Status{}, e
	}
	var st Status
	e = json.NewDecoder(bufio.NewReader(c)).Decode(&st)
	return st, e
}
