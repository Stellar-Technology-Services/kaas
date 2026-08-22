package ui

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// DaemonStatus describes a background UI process.
type DaemonStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Listen  string `json:"listen,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StartDaemon launches `kaas ui serve` in the background.
func StartDaemon(opts Options) error {
	if err := validateListen(opts.Listen); err != nil {
		return err
	}
	if st := StatusDaemon(opts); st.Running {
		return fmt.Errorf("ui already running (pid %d)", st.PID)
	}
	if err := os.MkdirAll(opts.Root, 0o700); err != nil {
		return err
	}
	exe := opts.Executable
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	logFile, err := os.OpenFile(LogFile(opts.Root), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "ui", "serve", "--listen", opts.Listen)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	_ = logFile.Close()
	if err := WritePid(opts.Root, pid); err != nil {
		return err
	}
	// Wait briefly for healthz.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if StatusDaemon(opts).Running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// StopDaemon stops a background UI process.
func StopDaemon(opts Options) error {
	pid, err := ReadPid(opts.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("ui is not running")
		}
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		RemovePid(opts.Root)
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		RemovePid(opts.Root)
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			RemovePid(opts.Root)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
	RemovePid(opts.Root)
	return nil
}

// StatusDaemon reports whether the UI health endpoint responds.
func StatusDaemon(opts Options) DaemonStatus {
	st := DaemonStatus{Listen: opts.Listen}
	pid, err := ReadPid(opts.Root)
	if err == nil {
		st.PID = pid
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				st.Running = true
			}
		}
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://" + opts.Listen + "/healthz")
	if err != nil {
		if st.Running {
			st.Error = err.Error()
		}
		st.Running = false
		return st
	}
	defer resp.Body.Close()
	st.Running = resp.StatusCode == http.StatusOK
	return st
}
