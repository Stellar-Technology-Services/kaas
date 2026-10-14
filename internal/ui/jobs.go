package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// JobRunner starts `kaas <verb> ... --jsonl` as a background process and
// persists its progress in the jobs table so HTMX pages can poll it. The UI
// never re-implements lifecycle logic: it drives the CLI and reads JSONL.
type JobRunner struct {
	store     *storeAdapter
	exe       string
	statePath string
	root      string // directory for per-job logs
	mu        sync.Mutex
	done      map[string]chan struct{}
}

// jobDescription is the UI-facing summary of a started job.
type startRequest struct {
	Sandbox string   // registry name the job is about (may not exist yet for `up`)
	Verb    string   // kaas subcommand, e.g. "up", "stop", "services"
	Args    []string // remaining args, e.g. ["sync", "my-app"]
	Dir     string   // working directory; empty means $HOME (no accidental project discovery)
}

func newJobRunner(store *storeAdapter, exe, statePath, root string) *JobRunner {
	return &JobRunner{store: store, exe: exe, statePath: statePath, root: root, done: map[string]chan struct{}{}}
}

// Start spawns the job. It refuses when another job for the sandbox is active
// so a double-click cannot create two VMs (the CLI's operation claims remain
// the authoritative guard; this is the UI's fast path).
func (r *JobRunner) Start(req startRequest) (*state.Job, error) {
	if r == nil || r.store == nil {
		return nil, fmt.Errorf("job runner is not configured")
	}
	if req.Sandbox == "" || req.Verb == "" {
		return nil, fmt.Errorf("sandbox and verb are required")
	}
	exe := r.exe
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	if active, err := r.store.ActiveJob(req.Sandbox); err == nil {
		return nil, fmt.Errorf("a %s job is already running for %s", active.Verb, req.Sandbox)
	} else if !errors.Is(err, state.ErrNotFound) {
		return nil, err
	}
	job, err := r.store.CreateJob(req.Sandbox, req.Verb, req.Args)
	if err != nil {
		return nil, err
	}

	logDir := filepath.Join(r.root, "jobs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}
	job.LogPath = filepath.Join(logDir, job.ID+".log")
	logFile, err := os.OpenFile(job.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}

	args := append([]string{req.Verb}, req.Args...)
	args = append(args, "--jsonl")
	if r.statePath != "" {
		args = append(args, "--state", r.statePath)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	// Run from the project directory when it still exists so a project
	// kaas.yaml applies; otherwise from $HOME so no stray config is discovered.
	if info, statErr := os.Stat(req.Dir); req.Dir == "" || statErr != nil || !info.IsDir() {
		cmd.Dir, _ = os.UserHomeDir()
	} else {
		cmd.Dir = req.Dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	fmt.Fprintf(logFile, "$ %s %s\n", filepath.Base(exe), strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		job.Status = state.JobFailed
		job.Summary = err.Error()
		now := time.Now().UTC()
		job.FinishedAt = &now
		_ = r.store.UpdateJob(*job)
		return job, err
	}
	now := time.Now().UTC()
	job.Status = state.JobRunning
	job.StartedAt = &now
	if err := r.store.UpdateJob(*job); err != nil {
		_ = cmd.Process.Kill()
		_ = logFile.Close()
		return nil, err
	}

	done := make(chan struct{})
	r.mu.Lock()
	r.done[job.ID] = done
	r.mu.Unlock()
	go r.wait(*job, cmd, stdout, stderr, logFile, done)
	return job, nil
}

// Wait blocks until the job finishes (tests and the auto-stop ticker use it).
func (r *JobRunner) Wait(id string, timeout time.Duration) bool {
	r.mu.Lock()
	ch, ok := r.done[id]
	r.mu.Unlock()
	if !ok {
		return true
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// jsonlRecord is the subset of kaas --jsonl output the runner understands.
type jsonlRecord struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Status  string `json:"status"`
	Error   string `json:"error"`
}

func (r *JobRunner) wait(job state.Job, cmd *exec.Cmd, stdout, stderr io.Reader, logFile *os.File, done chan struct{}) {
	defer close(done)
	defer func() {
		r.mu.Lock()
		delete(r.done, job.ID)
		r.mu.Unlock()
	}()

	var logMu sync.Mutex
	writeLog := func(prefix, line string) {
		logMu.Lock()
		defer logMu.Unlock()
		if prefix != "" {
			fmt.Fprintf(logFile, "%s%s\n", prefix, line)
			return
		}
		fmt.Fprintln(logFile, line)
	}

	var (
		lastMsg  string
		lastErr  string
		resultOK = false
		wg       sync.WaitGroup
		stateMu  sync.Mutex
	)
	// Only the last few stderr lines are kept as the failure summary; the full
	// stream stays in the log file.
	var stderrTail []string

	wg.Add(2)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			writeLog("", line)
			var rec jsonlRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			stateMu.Lock()
			switch rec.Type {
			case "message":
				if msg := strings.TrimSpace(rec.Message); msg != "" {
					lastMsg = truncate(msg, 160)
					job.Phase = lastMsg
					_ = r.store.UpdateJob(job)
				}
			case "result":
				if rec.Status == "ok" {
					resultOK = true
				}
				if rec.Error != "" {
					lastErr = rec.Error
				}
			}
			stateMu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			writeLog("! ", line)
			stateMu.Lock()
			stderrTail = append(stderrTail, line)
			if len(stderrTail) > 5 {
				stderrTail = stderrTail[1:]
			}
			stateMu.Unlock()
		}
	}()
	wg.Wait()
	err := cmd.Wait()
	_ = logFile.Close()

	now := time.Now().UTC()
	job.FinishedAt = &now
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = -1
	}
	job.ExitCode = &code
	stateMu.Lock()
	defer stateMu.Unlock()
	if err == nil && (resultOK || lastErr == "") {
		job.Status = state.JobOK
		job.Summary = lastMsg
	} else {
		job.Status = state.JobFailed
		switch {
		case lastErr != "":
			job.Summary = truncate(lastErr, 300)
		case len(stderrTail) > 0:
			job.Summary = truncate(strings.Join(stderrTail, " | "), 300)
		case err != nil:
			job.Summary = err.Error()
		}
	}
	_ = r.store.UpdateJob(job)
}

// tailFile returns the last n lines of a file (best effort).
func tailFile(path string, n int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
