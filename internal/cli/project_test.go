package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInitTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skipf("git init: %v", err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cmd := newProjectCmd().Commands()[0]
	cmd.SetContext(context.WithValue(context.Background(), ctxKey{}, &App{}))
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(dir, "kaas.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "directory: ~/workspace/"+filepath.Base(dir)) {
		t.Fatalf("unexpected config: %s", config)
	}
	script := filepath.Join(dir, "scripts", "kaas-setup")
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("setup script is not executable")
	}
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected existing files error")
	}
}
