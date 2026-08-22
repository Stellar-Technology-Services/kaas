package ui

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestValidateListen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:18181", false},
		{"[::1]:18181", false},
		{"0.0.0.0:18181", true},
		{"192.168.1.1:18181", true},
		{"localhost:18181", true},
		{"::1:18181", true},
		{"127.0.0.1", true},
		{"", true},
	}
	for _, tc := range cases {
		err := validateListen(tc.addr)
		if tc.wantErr && err == nil {
			t.Fatalf("%q: expected error", tc.addr)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.addr, err)
		}
	}
}

func TestUIRuntimeValidate(t *testing.T) {
	t.Parallel()
	ok := config.UIRuntime{Root: "/tmp/ui", Listen: "127.0.0.1:18181"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := config.UIRuntime{Root: "/tmp/ui", Listen: "0.0.0.0:18181"}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected non-loopback rejection")
	}
}

func TestServerHealthAndPages(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	st, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSandbox(state.Sandbox{
		Name:          "demo",
		RepoPath:      "/tmp/demo",
		RepoName:      "demo",
		SSHHost:       "127.0.0.1",
		SSHPort:       6022,
		DesiredState:  state.DesiredRunning,
		ObservedState: state.ObservedRunning,
		CPUs:          2,
		MemoryGiB:     4,
		DiskGiB:       20,
	}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	listen := "127.0.0.1:" + freeTCPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServe(ctx, Options{
			Listen:    listen,
			StatePath: dbPath,
			Root:      filepath.Join(dir, "ui"),
			IngressRuntime: config.IngressRuntime{
				Root:      filepath.Join(dir, "ingress"),
				Listen:    "127.0.0.1:18000",
				DNSListen: "127.0.0.1:5353",
				Domain:    "kaas.test",
			},
		})
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	var healthOK bool
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + listen + "/healthz")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 && string(body) == "ok\n" {
				healthOK = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !healthOK {
		cancel()
		t.Fatal("healthz did not become ready")
	}

	for _, path := range []string{
		"/", "/sandboxes", "/services", "/events", "/sandboxes/demo",
		"/partials/overview", "/partials/sandboxes", "/partials/services",
		"/static/htmx.min.js",
	} {
		resp, err := client.Get("http://" + listen + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server exit: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}
