package ui

import (
	"context"
	"log"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// autoStopInterval is how often the daemon checks deadlines. Boat stops
// "within about 5 seconds of the deadline"; 30 s is plenty for a laptop.
const autoStopInterval = 30 * time.Second

// autoStopDue returns the sandboxes whose deadline has passed and that can be
// stopped right now: running, not a template, and no job in flight (a running
// job pushes the deadline, like boat's "a running prompt extends the TTL").
func autoStopDue(sandboxes []state.Sandbox, active map[string]bool, now time.Time) []state.Sandbox {
	var due []state.Sandbox
	for _, sb := range sandboxes {
		if sb.IsTemplate() || sb.AutoStopAt == nil || sb.ObservedState != state.ObservedRunning {
			continue
		}
		if active[sb.Name] || sb.AutoStopAt.After(now) {
			continue
		}
		due = append(due, sb)
	}
	return due
}

// autoStopLoop runs in the UI daemon and starts `kaas stop` jobs for overdue
// sandboxes. It is the only place the UI acts without a click.
func (s *Server) autoStopLoop(ctx context.Context) {
	ticker := time.NewTicker(autoStopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.autoStopTick(now.UTC())
		}
	}
}

func (s *Server) autoStopTick(now time.Time) {
	sandboxes, err := s.store.ListSandboxes("")
	if err != nil {
		return
	}
	jobs, _ := s.store.ActiveJobs()
	active := make(map[string]bool, len(jobs))
	for _, j := range jobs {
		active[j.Sandbox] = true
	}
	for _, sb := range autoStopDue(sandboxes, active, now) {
		job, err := s.jobs.Start(startRequest{Sandbox: sb.Name, Verb: "stop", Args: []string{sb.Name}, Dir: sb.RepoPath})
		if err != nil {
			log.Printf("auto-stop %s: %v", sb.Name, err)
			continue
		}
		_ = s.store.AddEvent(sb.Name, "auto_stop", "ok", "deadline reached; job "+job.ID)
		log.Printf("auto-stop %s: deadline %s passed, job %s", sb.Name, sb.AutoStopAt.Format(time.RFC3339), job.ID)
	}
}
