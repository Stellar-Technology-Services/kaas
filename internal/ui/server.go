package ui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

// Options configures the local dashboard server.
type Options struct {
	Listen         string
	StatePath      string
	Root           string // ~/.kaas/ui for pid/log when managed as daemon
	IngressRuntime config.IngressRuntime
	Executable     string // path to kaas binary for POST sync
}

// Server is the localhost management UI.
type Server struct {
	opts  Options
	mux   *http.ServeMux
	store *storeAdapter
}

// ListenAndServe binds loopback-only and serves until ctx is cancelled.
func ListenAndServe(ctx context.Context, opts Options) error {
	if err := validateListen(opts.Listen); err != nil {
		return err
	}
	if opts.StatePath == "" {
		return fmt.Errorf("state path is required")
	}
	st, err := openStore(opts.StatePath)
	if err != nil {
		return err
	}
	defer st.Close()

	s := &Server{opts: opts, store: st, mux: http.NewServeMux()}
	s.routes()

	ln, err := net.Listen("tcp", opts.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", opts.Listen, err)
	}
	defer ln.Close()

	httpServer := &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func validateListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("ui listen address: %w", err)
	}
	if port == "" {
		return fmt.Errorf("ui listen address must include a port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("ui must bind to a loopback address (got %q); use 127.0.0.1 or ::1", host)
	}
	return nil
}

func (s *Server) routes() {
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS()))))
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /sandboxes", s.handleSandboxes)
	s.mux.HandleFunc("GET /sandboxes/{name}", s.handleSandboxDetail)
	s.mux.HandleFunc("POST /sandboxes/{name}/sync", s.handleSandboxSync)
	s.mux.HandleFunc("GET /services", s.handleServices)
	s.mux.HandleFunc("GET /events", s.handleEvents)
	s.mux.HandleFunc("GET /partials/overview", s.handlePartialOverview)
	s.mux.HandleFunc("GET /partials/sandboxes", s.handlePartialSandboxes)
	s.mux.HandleFunc("GET /partials/services", s.handlePartialServices)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// PidFile returns the daemon pid path under root.
func PidFile(root string) string { return filepath.Join(root, "ui.pid") }

// LogFile returns the daemon log path under root.
func LogFile(root string) string { return filepath.Join(root, "ui.log") }

// WritePid writes the process id for daemon management.
func WritePid(root string, pid int) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	return os.WriteFile(PidFile(root), []byte(fmt.Sprintf("%d\n", pid)), 0o600)
}

// ReadPid reads a previously written daemon pid.
func ReadPid(root string) (int, error) {
	b, err := os.ReadFile(PidFile(root))
	if err != nil {
		return 0, err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

// RemovePid deletes the pid file if present.
func RemovePid(root string) { _ = os.Remove(PidFile(root)) }
