package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestCommandsCRUD(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateSandbox(Sandbox{Name: "demo", RepoPath: "/tmp/demo", RepoName: "demo", SSHPort: 6022}); err != nil {
		t.Fatal(err)
	}
	c := Command{ID: "c1a2b3", Sandbox: "demo", Command: "npm run build", Cwd: "~/workspace/demo", PID: 4242}
	if err := st.CreateCommand(c); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCommand("c1a")
	if err != nil || got.ID != "c1a2b3" || !got.Running() || got.PID != 4242 {
		t.Fatalf("prefix lookup: %+v %v", got, err)
	}
	code := 0
	now := time.Now().UTC()
	got.Status, got.ExitCode, got.EndedAt = CmdExited, &code, &now
	if err := st.UpdateCommand(*got); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCommand(Command{ID: "d4", Sandbox: "demo", Command: "sleep 100"}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkCommandsLost("demo"); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListCommands("demo", 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}
	for _, c := range list {
		switch c.ID {
		case "c1a2b3":
			if c.Status != CmdExited || c.ExitCode == nil || *c.ExitCode != 0 {
				t.Fatalf("exited row: %+v", c)
			}
		case "d4":
			if c.Status != CmdLost || c.EndedAt == nil {
				t.Fatalf("lost row: %+v", c)
			}
		}
	}
	if err := st.DeleteSandbox("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCommand("c1a2b3"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cascade delete expected, got %v", err)
	}
}
