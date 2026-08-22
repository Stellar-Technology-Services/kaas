package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func testInitCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), ctxKey{}, &App{}))
	return cmd
}

func TestRunInitClonesAnsibleRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	src := t.TempDir()
	if err := exec.Command("git", "init", src).Run(); err != nil {
		t.Fatalf("git init source: %v", err)
	}
	// Identity for commit in temp repo (local config only).
	_ = exec.Command("git", "-C", src, "config", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "-C", src, "config", "user.name", "test").Run()
	if err := os.WriteFile(filepath.Join(src, "ansible.cfg"), []byte("[defaults]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", src, "add", ".").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := exec.Command("git", "-C", src, "commit", "-m", "seed").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	dir := t.TempDir()
	cmd := testInitCmd(t)
	if err := runInit(cmd, dir, src); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg := filepath.Join(dir, "ansible", "ansible.cfg")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("expected cloned ansible.cfg: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kaas.yaml")); err != nil {
		t.Fatalf("expected kaas.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secretspec.toml")); err != nil {
		t.Fatalf("expected secretspec.toml: %v", err)
	}

	// Second init preserves existing ansible/ and does not error.
	marker := filepath.Join(dir, "ansible", "local-marker")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(cmd, dir, "https://example.invalid/should-not-clone"); err != nil {
		t.Fatalf("second runInit: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep" {
		t.Fatalf("ansible tree was replaced; marker=%q", data)
	}
}

func TestSeedAnsibleRepoRequiresGitURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := testInitCmd(t)
	dir := filepath.Join(t.TempDir(), "ansible")
	_, err := seedAnsibleRepo(cmd, dir, "https://example.invalid/no-such-repo.git")
	if err == nil {
		t.Fatal("expected clone failure")
	}
}
