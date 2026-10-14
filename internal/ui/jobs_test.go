package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// fakeKaas writes a shell script that mimics `kaas <verb> ... --jsonl`.
// It emits two message records and a result; exit code comes from $FAKE_EXIT.
func fakeKaas(t *testing.T, dir string) string {
	t.Helper()
	script := filepath.Join(dir, "kaas")
	body := `#!/bin/sh
echo '{"schema_version":1,"type":"message","message":"phase one"}'
echo '{"schema_version":1,"type":"message","message":"phase two"}'
echo "args: $*" 1>&2
if [ "${FAKE_EXIT:-0}" != "0" ]; then
  echo "boom: provider exploded" 1>&2
  echo '{"schema_version":1,"type":"result","status":"error","error":"provider exploded"}'
  exit "$FAKE_EXIT"
fi
echo '{"schema_version":1,"type":"result","status":"ok"}'
`
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestJobRunnerSuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := openStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	exe := fakeKaas(t, dir)
	runner := newJobRunner(st, exe, filepath.Join(dir, "state.db"), filepath.Join(dir, "ui"))

	job, err := runner.Start(startRequest{Sandbox: "demo", Verb: "stop", Args: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Wait(job.ID, 5*time.Second) {
		t.Fatal("job did not finish")
	}
	got, err := st.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != state.JobOK || got.Phase != "phase two" || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("unexpected job: %+v", got)
	}
	log, err := os.ReadFile(got.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "stop demo --jsonl --state") || !strings.Contains(string(log), "! args: stop demo") {
		t.Fatalf("log missing command echo or stderr: %s", log)
	}

	t.Setenv("FAKE_EXIT", "3")
	job2, err := runner.Start(startRequest{Sandbox: "demo", Verb: "start", Args: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if !runner.Wait(job2.ID, 5*time.Second) {
		t.Fatal("job2 did not finish")
	}
	got2, _ := st.GetJob(job2.ID)
	if got2.Status != state.JobFailed || got2.ExitCode == nil || *got2.ExitCode != 3 || !strings.Contains(got2.Summary, "provider exploded") {
		t.Fatalf("unexpected failed job: %+v", got2)
	}
}

func TestJobRunnerRefusesConcurrentJobForSandbox(t *testing.T) {
	dir := t.TempDir()
	st, err := openStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// A sleeping fake keeps the first job active long enough to collide.
	script := filepath.Join(dir, "kaas")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\necho '{\"type\":\"result\",\"status\":\"ok\"}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := newJobRunner(st, script, "", filepath.Join(dir, "ui"))
	first, err := runner.Start(startRequest{Sandbox: "demo", Verb: "up", Args: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Start(startRequest{Sandbox: "demo", Verb: "stop", Args: []string{"demo"}}); err == nil {
		t.Fatal("expected second job to be refused while first is active")
	}
	if _, err := runner.Start(startRequest{Sandbox: "other", Verb: "stop", Args: []string{"other"}}); err != nil {
		t.Fatalf("unrelated sandbox must not be blocked: %v", err)
	}
	runner.Wait(first.ID, 5*time.Second)
}
