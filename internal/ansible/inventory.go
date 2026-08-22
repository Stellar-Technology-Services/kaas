package ansible

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// WriteInventory writes a temporary Ansible inventory for one sandbox (+ local).
// Returns the inventory file path; caller should remove when done.
func WriteInventory(sb state.Sandbox) (string, error) {
	return WriteInventoryWithOptions(sb, false)
}

// WriteInventoryWithOptions writes an inventory and optionally forwards the
// host SSH agent to the guest for a command such as a private Git checkout.
func WriteInventoryWithOptions(sb state.Sandbox, forwardAgent bool) (string, error) {
	sshKey := sb.SSHKey
	if sshKey == "" {
		sshKey = "~/.colima/_lima/_config/user"
	}
	user := sb.SSHUser
	if user == "" {
		user = os.Getenv("USER")
	}
	host := sb.SSHHost
	if host == "" {
		host = "127.0.0.1"
	}

	var b strings.Builder
	b.WriteString("[local]\n")
	b.WriteString("localhost ansible_connection=local\n\n")
	b.WriteString("[colima_vms]\n")
	fmt.Fprintf(&b, "%s ansible_host=%s ansible_port=%d\n\n", sb.Name, host, sb.SSHPort)
	b.WriteString("[colima_vms:vars]\n")
	fmt.Fprintf(&b, "ansible_user=%s\n", user)
	fmt.Fprintf(&b, "ansible_ssh_private_key_file=%s\n", sshKey)
	b.WriteString("ansible_python_interpreter=/usr/bin/python3\n")
	sshArgs := "-o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"
	if forwardAgent {
		sshArgs += " -o ForwardAgent=yes"
	}
	fmt.Fprintf(&b, "ansible_ssh_common_args='%s'\n", sshArgs)

	f, err := os.CreateTemp("", "kaas-inventory-*.ini")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// WriteInventoryTo writes inventory to a specific path (e.g. .kaas/inventory.ini).
func WriteInventoryTo(dir string, sb state.Sandbox) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "inventory.ini")
	tmp, err := WriteInventory(sb)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	data, err := os.ReadFile(tmp)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
