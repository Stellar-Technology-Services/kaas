package ansible

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// Runner executes ansible-playbook / ansible against a sandbox.
type Runner struct {
	AnsibleDir   string
	ExtraArgs    []string
	Stdout       io.Writer
	Stderr       io.Writer
	ForwardAgent bool
}

// PlaybookOpts configures a playbook run.
type PlaybookOpts struct {
	Playbook  string
	Sandbox   state.Sandbox
	Limit     string // default: sandbox name
	Check     bool
	ExtraVars map[string]string
	ExtraArgs []string
}

// RunPlaybook invokes ansible-playbook with generated inventory and limit.
func (r *Runner) RunPlaybook(ctx context.Context, opts PlaybookOpts) error {
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return fmt.Errorf("ansible-playbook not found on PATH")
	}
	if opts.Playbook == "" {
		return fmt.Errorf("playbook is required")
	}
	playbook := opts.Playbook
	if !filepath.IsAbs(playbook) {
		playbook = filepath.Join(r.AnsibleDir, playbook)
	}
	if _, err := os.Stat(playbook); err != nil {
		return fmt.Errorf("playbook not found: %s", playbook)
	}

	inv, err := WriteInventory(opts.Sandbox)
	if err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	defer os.Remove(inv)

	limit := opts.Limit
	if limit == "" {
		limit = opts.Sandbox.Name
	}

	args := []string{
		"-i", inv,
		playbook,
		"-e", "vm_name=" + opts.Sandbox.Name,
		"-e", "vm_ssh_port=" + strconv.Itoa(opts.Sandbox.SSHPort),
	}
	base := filepath.Base(playbook)
	args = append(args, "--limit", limit)
	if opts.Check {
		args = append(args, "--check")
	}
	for k, v := range opts.ExtraVars {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, r.ExtraArgs...)
	args = append(args, opts.ExtraArgs...)

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...)
	cmd.Dir = r.AnsibleDir
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook %s: %w", base, err)
	}
	return nil
}

// Ping runs `ansible colima_vms -m ping` against the sandbox inventory.
func (r *Runner) Ping(ctx context.Context, sb state.Sandbox) error {
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not found on PATH")
	}
	inv, err := WriteInventory(sb)
	if err != nil {
		return err
	}
	defer os.Remove(inv)

	args := []string{"-i", inv, "colima_vms", "-m", "ping", "--limit", sb.Name}
	args = append(args, r.ExtraArgs...)
	cmd := exec.CommandContext(ctx, "ansible", args...)
	cmd.Dir = r.AnsibleDir
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible ping: %w", err)
	}
	return nil
}

// AdHoc runs an arbitrary ansible module against the sandbox.
func (r *Runner) AdHoc(ctx context.Context, sb state.Sandbox, module, args string, become bool) error {
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not found on PATH")
	}
	inv, err := WriteInventory(sb)
	if err != nil {
		return err
	}
	defer os.Remove(inv)

	cmdArgs := []string{"-i", inv, "colima_vms", "-m", module, "--limit", sb.Name}
	if args != "" {
		cmdArgs = append(cmdArgs, "-a", args)
	}
	if become {
		cmdArgs = append(cmdArgs, "-b")
	}
	cmdArgs = append(cmdArgs, r.ExtraArgs...)
	cmd := exec.CommandContext(ctx, "ansible", cmdArgs...)
	cmd.Dir = r.AnsibleDir
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible %s: %w", module, err)
	}
	return nil
}

func (r *Runner) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r *Runner) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

// RunShell executes a shell command through Ansible on one sandbox. It is
// intentionally for declared project setup commands, not arbitrary CLI input.
func (r *Runner) RunShell(ctx context.Context, sb state.Sandbox, command string) error {
	if _, err := exec.LookPath("ansible"); err != nil {
		return fmt.Errorf("ansible not found on PATH")
	}
	inv, err := WriteInventoryWithOptions(sb, r.ForwardAgent)
	if err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	defer os.Remove(inv)
	cmd := exec.CommandContext(ctx, "ansible", "-i", inv, "colima_vms", "-m", "shell", "--limit", sb.Name, "-a", command)
	cmd.Dir = r.AnsibleDir
	cmd.Stdout, cmd.Stderr = r.stdout(), r.stderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible shell: %w", err)
	}
	return nil
}

// Preflight checks required tools are available.
func Preflight(needColima bool) error {
	var missing []string
	if needColima {
		if _, err := exec.LookPath("colima"); err != nil {
			missing = append(missing, "colima")
		}
	}
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		missing = append(missing, "ansible-playbook")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools on PATH: %s", strings.Join(missing, ", "))
	}
	return nil
}
