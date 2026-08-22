package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/ingress"
)

// Bootstrap is the private stdin payload for ingress serve.
type Bootstrap struct {
	Listen    string          `json:"listen"`
	DNSListen string          `json:"dns_listen"`
	Domain    string          `json:"domain"`
	Routes    []ingress.Route `json:"routes"`
}

// Request is a control-socket command.
type Request struct {
	Action string          `json:"action"`
	Routes []ingress.Route `json:"routes,omitempty"`
}

// Status is redacted runtime status.
type Status struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Listen    string `json:"listen,omitempty"`
	DNSListen string `json:"dns_listen,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Routes    int    `json:"routes,omitempty"`
	Error     string `json:"error,omitempty"`
}

func Dir(root string) string       { return root }
func Socket(root string) string    { return filepath.Join(root, "control.sock") }
func PidFile(root string) string   { return filepath.Join(root, "pid") }
func StateFile(root string) string { return filepath.Join(root, "status.json") }

// Serve runs the ingress HTTP proxy, DNS, and SSH forwards until ctx ends.
func Serve(ctx context.Context, root string, in io.Reader) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	var b Bootstrap
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&b); err != nil {
		return fmt.Errorf("read ingress bootstrap: %w", err)
	}
	if b.Domain == "" {
		b.Domain = ingress.DefaultDomain
	}
	if b.Listen == "" {
		b.Listen = "127.0.0.1:18000"
	}
	if b.DNSListen == "" {
		b.DNSListen = "127.0.0.1:5353"
	}

	proxy := ingress.NewProxy(b.Domain)
	proxy.SetRoutes(b.Routes)
	fwd := ingress.NewForwarder()
	if err := fwd.Sync(b.Routes); err != nil {
		// Continue; catalog still useful; forwards may recover on reload.
		_ = err
	}
	defer fwd.StopAll()

	httpLn, err := ingress.ListenAndServeHTTP(b.Listen, proxy)
	if err != nil {
		return err
	}
	defer httpLn.Close()

	dns := &ingress.DNSServer{
		Domain:   b.Domain,
		Address:  net.IPv4(127, 0, 0, 1),
		Address6: net.ParseIP("::1"),
		Allowed:  proxy.HasHostname,
	}
	go func() { _ = dns.ListenAndServe(ctx, b.DNSListen) }()

	sock := Socket(root)
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
		_ = httpLn.Close()
	}()
	_ = os.Chmod(sock, 0o600)
	_ = os.WriteFile(PidFile(root), []byte(strconv.Itoa(os.Getpid())), 0o600)
	defer os.Remove(PidFile(root))

	st := Status{
		Running:   true,
		PID:       os.Getpid(),
		Listen:    b.Listen,
		DNSListen: b.DNSListen,
		Domain:    b.Domain,
		Routes:    len(b.Routes),
	}
	_ = writeStatus(root, st)

	for {
		c, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go handle(c, proxy, fwd, &st, root)
	}
}

func handle(c net.Conn, proxy *ingress.Proxy, fwd *ingress.Forwarder, st *Status, root string) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	var r Request
	if json.NewDecoder(io.LimitReader(c, 1<<20)).Decode(&r) != nil {
		return
	}
	switch r.Action {
	case "reload":
		proxy.SetRoutes(r.Routes)
		_ = fwd.Sync(r.Routes)
		st.Routes = len(r.Routes)
		_ = writeStatus(root, *st)
		_ = json.NewEncoder(c).Encode(*st)
	case "status":
		st.Routes = len(proxy.Routes())
		_ = json.NewEncoder(c).Encode(*st)
	case "stop":
		_ = json.NewEncoder(c).Encode(*st)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	default:
		_ = json.NewEncoder(c).Encode(Status{Error: "unknown action"})
	}
}

func writeStatus(root string, st Status) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(StateFile(root), b, 0o600)
}

// Call sends a control request to the running ingress daemon.
func Call(root string, r Request) (Status, error) {
	c, err := net.DialTimeout("unix", Socket(root), 2*time.Second)
	if err != nil {
		return Status{}, err
	}
	defer c.Close()
	if err := json.NewEncoder(c).Encode(r); err != nil {
		return Status{}, err
	}
	var st Status
	err = json.NewDecoder(bufio.NewReader(c)).Decode(&st)
	return st, err
}

// Running reports whether a live control socket answers status.
func Running(root string) bool {
	st, err := Call(root, Request{Action: "status"})
	return err == nil && st.Running
}
