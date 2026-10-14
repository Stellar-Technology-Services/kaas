package ui

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
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
			Listen:     listen,
			StatePath:  dbPath,
			Root:       filepath.Join(dir, "ui"),
			Executable: fakeKaas(t, dir),
			IngressRuntime: config.IngressRuntime{
				Root:      filepath.Join(dir, "ingress"),
				Listen:    "127.0.0.1:18000",
				DNSListen: "127.0.0.1:5353",
				Domain:    "kaas.test",
			},
		})
	}()

	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
		"/", "/sandboxes", "/sandboxes/new", "/services", "/events", "/jobs", "/templates", "/sandboxes/demo",
		"/partials/overview", "/partials/sandboxes", "/partials/services", "/partials/sandboxes/demo/status",
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

	// Lifecycle action → background job → redirect back to the detail page.
	resp, err := client.PostForm("http://"+listen+"/sandboxes/demo/actions/stop", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	stopBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/sandboxes/demo" {
		t.Fatalf("stop action: status %d location %q body %s", resp.StatusCode, resp.Header.Get("Location"), stopBody)
	}
	// Start is not allowed while running (409), independent of the job runner.
	resp, err = client.PostForm("http://"+listen+"/sandboxes/demo/actions/start", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("start while running: status %d", resp.StatusCode)
	}
	// Destroy without the typed confirmation is rejected.
	resp, err = client.PostForm("http://"+listen+"/sandboxes/demo/actions/destroy", url.Values{"confirm": {"nope"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusConflict {
		t.Fatalf("destroy without confirm: status %d", resp.StatusCode)
	}
	// Create form: bad name → 400 with the form re-rendered; good name → job + redirect.
	resp, err = client.PostForm("http://"+listen+"/sandboxes", url.Values{"name": {"Bad Name!"}, "size": {"small"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid create: status %d", resp.StatusCode)
	}
	resp, err = client.PostForm("http://"+listen+"/sandboxes", url.Values{"name": {"fresh"}, "size": {"small"}, "trust": {"untrusted"}, "repo": {"git@github.com:acme/x.git"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/sandboxes/fresh" {
		t.Fatalf("create: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	// The pending page renders for a name that has a job but no registry row yet.
	resp, err = client.Get("http://" + listen + "/sandboxes/fresh")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "kaas up fresh --size small --trust untrusted --repo") {
		t.Fatalf("pending page: status %d body has command: %v", resp.StatusCode, strings.Contains(string(body), "kaas up fresh"))
	}

	// Let the fake jobs finish before shutdown so the temp dir can be removed.
	st2, _ := state.Open(dbPath)
	for _, job := range mustActiveJobs(t, st2) {
		_ = job
	}
	_ = st2.Close()
	time.Sleep(300 * time.Millisecond)

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server exit: %v", err)
		}
	case <-time.After(8 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("server did not shut down; goroutines:\n%s", buf[:n])
	}
}

func mustActiveJobs(t *testing.T, st *state.Store) []state.Job {
	t.Helper()
	jobs, err := st.ActiveJobs()
	if err != nil {
		t.Fatal(err)
	}
	return jobs
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
