package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestJobsCRUD(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	job, err := st.CreateJob("demo", "up", []string{"demo", "--size", "small"})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Status != JobQueued {
		t.Fatalf("unexpected job: %+v", job)
	}

	active, err := st.ActiveJob("demo")
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != job.ID {
		t.Fatalf("active job %s, want %s", active.ID, job.ID)
	}
	if _, err := st.ActiveJob("other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for other sandbox, got %v", err)
	}

	now := time.Now().UTC()
	code := 0
	job.Status, job.Phase, job.Summary = JobOK, "setup", "Completed"
	job.ExitCode, job.StartedAt, job.FinishedAt, job.LogPath = &code, &now, &now, "/tmp/log"
	if err := st.UpdateJob(*job); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobOK || got.Phase != "setup" || got.ExitCode == nil || *got.ExitCode != 0 || len(got.Args) != 3 || got.StartedAt == nil {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.Active() {
		t.Fatal("finished job must not be active")
	}
	if _, err := st.ActiveJob("demo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected no active job after finish, got %v", err)
	}

	list, err := st.ListJobs("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len %d", len(list))
	}
	if err := st.UpdateJob(Job{ID: "missing", Status: JobOK}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
