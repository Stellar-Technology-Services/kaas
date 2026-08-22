package ansible

import (
	"os"
	"strings"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestWriteInventory(t *testing.T) {
	path, err := WriteInventory(state.Sandbox{
		Name:    "demo",
		SSHHost: "127.0.0.1",
		SSHPort: 6023,
		SSHUser: "alice",
		SSHKey:  "/tmp/key",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"[local]",
		"[colima_vms]",
		"demo ansible_host=127.0.0.1 ansible_port=6023",
		"ansible_user=alice",
		"ansible_ssh_private_key_file=/tmp/key",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestWriteInventoryWithAgentForwarding(t *testing.T) {
	path, err := WriteInventoryWithOptions(state.Sandbox{Name: "demo", SSHPort: 6023}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ForwardAgent=yes") {
		t.Fatalf("agent forwarding missing in:\n%s", data)
	}
}
