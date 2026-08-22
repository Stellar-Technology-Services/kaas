package ingress

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Forwarder maintains SSH local port forwards for published services.
type Forwarder struct {
	mu    sync.Mutex
	procs map[string]*exec.Cmd // key: hostname
}

// NewForwarder constructs an empty forwarder.
func NewForwarder() *Forwarder {
	return &Forwarder{procs: map[string]*exec.Cmd{}}
}

// Sync starts missing forwards and stops removed ones.
func (f *Forwarder) Sync(routes []Route) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	wanted := make(map[string]Route, len(routes))
	for _, r := range routes {
		if r.Hostname == "" || r.HostPort < 1 || r.GuestPort < 1 || r.SSHPort < 1 {
			continue
		}
		wanted[r.Hostname] = r
	}

	for host, cmd := range f.procs {
		if _, ok := wanted[host]; ok {
			continue
		}
		_ = stopCmd(cmd)
		delete(f.procs, host)
	}

	var firstErr error
	for host, route := range wanted {
		if cmd, ok := f.procs[host]; ok && cmd.Process != nil {
			// Still running?
			if cmd.ProcessState == nil {
				continue
			}
			delete(f.procs, host)
		}
		cmd, err := startForward(route)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("forward %s: %w", host, err)
			}
			continue
		}
		f.procs[host] = cmd
	}
	return firstErr
}

// StopAll terminates every forward process.
func (f *Forwarder) StopAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for host, cmd := range f.procs {
		_ = stopCmd(cmd)
		delete(f.procs, host)
	}
}

func startForward(r Route) (*exec.Cmd, error) {
	bin, err := exec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("ssh not found on PATH")
	}
	user := r.SSHUser
	if user == "" {
		user = os.Getenv("USER")
	}
	host := r.SSHHost
	if host == "" {
		host = "127.0.0.1"
	}
	key := r.SSHKey
	if key == "" {
		key = os.ExpandEnv("$HOME/.colima/_lima/_config/user")
	}
	spec := fmt.Sprintf("%d:127.0.0.1:%d", r.HostPort, r.GuestPort)
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=30",
		"-i", key,
		"-p", strconv.Itoa(r.SSHPort),
		"-L", spec,
		fmt.Sprintf("%s@%s", user, host),
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd, nil
}

func stopCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	return nil
}
