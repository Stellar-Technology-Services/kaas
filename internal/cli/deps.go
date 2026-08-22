package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// hostDep describes a host prerequisite for running kaas.
type hostDep struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Required bool   `json:"required"`
	// Check is a PATH binary to LookPath. Empty means a custom check (see checkHostDep).
	Check string `json:"check,omitempty"`
	// BrewPackages are installed with `brew install` on darwin when auto-installable.
	BrewPackages []string `json:"brew_packages,omitempty"`
	// AptPackages are installed with `sudo apt install -y` on linux when auto-installable.
	AptPackages []string `json:"apt_packages,omitempty"`
	// Manual is printed when the dep cannot be installed via the package manager.
	Manual string `json:"manual,omitempty"`
	// Installer is a special-case auto installer id (secretspec, herdr, linux-lima-colima).
	Installer string `json:"installer,omitempty"`
}

type hostDepStatus struct {
	hostDep
	Status string `json:"status"` // ok | missing
	Detail string `json:"detail,omitempty"`
}

func newDepsCmd() *cobra.Command {
	var install bool
	var optional bool
	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Show (and optionally install) host dependencies for this OS",
		Long: `Print the host tools kaas needs on this operating system, with install
status and commands. Pass --install to install package-manager packages (and
other auto-installable tools). On an interactive TTY, missing auto-installable
deps prompt for confirmation unless --install or --jsonl is set.

Ansible Galaxy collections: kaas deps galaxy (also installed automatically by kaas up).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHostDeps(cmd, install, optional)
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "install missing auto-installable dependencies")
	cmd.Flags().BoolVar(&optional, "optional", false, "include optional dependencies when installing")
	cmd.AddCommand(newDepsGalaxyCmd())
	return cmd
}

func newDepsGalaxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "galaxy",
		Short: "Install Ansible Galaxy collections from requirements.yml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			return installGalaxyCollections(cmd.Context(), cmd, app)
		},
	}
}

func runHostDeps(cmd *cobra.Command, installFlag, includeOptional bool) error {
	deps := hostDependencies(runtime.GOOS)
	if len(deps) == 0 {
		return fmt.Errorf("unsupported operating system %q (kaas host deps support darwin and linux)", runtime.GOOS)
	}

	statuses := make([]hostDepStatus, 0, len(deps))
	for _, d := range deps {
		statuses = append(statuses, checkHostDep(d))
	}

	if jsonlEnabled(cmd) {
		for _, st := range statuses {
			emitJSONL("dep", map[string]any{
				"command": cmd.CommandPath(), "os": runtime.GOOS, "dep": st,
			})
		}
	} else {
		out := commandOutput(cmd)
		writeTitle(out, fmt.Sprintf("Host dependencies for %s", runtime.GOOS))
		rows := make([][]string, 0, len(statuses))
		hints := make([]string, 0, len(statuses))
		for _, st := range statuses {
			req := "optional"
			if st.Required {
				req = "required"
			}
			rows = append(rows, []string{st.Name, strings.ToUpper(st.Status), req, st.Role})
			if st.Status != "ok" {
				if how := installHint(st.hostDep, runtime.GOOS); how != "" {
					hints = append(hints, st.Name+": "+how)
				}
			}
		}
		writeTable(out, []string{"NAME", "STATUS", "NEED", "ROLE"}, rows, tableOpts{StatusCols: []int{1, 2}})
		for _, h := range hints {
			writeHint(out, h)
		}
		fmt.Fprintln(out)
	}

	missingAuto := missingAutoInstallable(statuses, includeOptional)
	missingManual := missingManualOnly(statuses, includeOptional)

	shouldInstall := installFlag
	if !shouldInstall && len(missingAuto) > 0 && !jsonlEnabled(cmd) && stdinIsInteractive() {
		printMessage(cmd, "Install missing auto-installable dependencies? [y/N] ")
		answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err == nil {
			answer = strings.ToLower(strings.TrimSpace(answer))
			shouldInstall = answer == "y" || answer == "yes"
		}
		if !shouldInstall {
			printStyled(cmd, styleWarn, "Skipped install. Re-run with: kaas deps --install\n")
		}
	}

	if shouldInstall && len(missingAuto) > 0 {
		if err := installHostDeps(cmd.Context(), cmd, missingAuto, runtime.GOOS); err != nil {
			return err
		}
		// Re-check after install.
		statuses = statuses[:0]
		for _, d := range deps {
			statuses = append(statuses, checkHostDep(d))
		}
		missingManual = missingManualOnly(statuses, includeOptional)
		stillMissing := 0
		for _, st := range statuses {
			if st.Status != "ok" && (st.Required || includeOptional) {
				stillMissing++
			}
		}
		if !jsonlEnabled(cmd) {
			if stillMissing == 0 {
				printStyled(cmd, styleSuccess, "All selected host dependencies are present.\n")
			} else {
				printStyled(cmd, styleWarn, "Some dependencies are still missing after install.\n")
			}
		}
		if jsonlEnabled(cmd) {
			emitJSONL("deps_install", map[string]any{
				"command": cmd.CommandPath(), "os": runtime.GOOS, "ok": stillMissing == 0,
			})
		}
	}

	if len(missingManual) > 0 && !jsonlEnabled(cmd) {
		printStyled(cmd, styleWarn, "Manual steps still required:\n")
		for _, d := range missingManual {
			writeHint(commandOutput(cmd), d.Name+": "+installHint(d, runtime.GOOS))
		}
	}

	if !jsonlEnabled(cmd) {
		writeMuted(commandOutput(cmd), "Galaxy collections: kaas deps galaxy  (also run by kaas up)\n")
		writeMuted(commandOutput(cmd), "Verify: kaas doctor\n")
	}
	return nil
}

func hostDependencies(goos string) []hostDep {
	switch goos {
	case "darwin":
		return []hostDep{
			{Name: "ansible", Role: "Guest provisioning", Required: true, Check: "ansible", BrewPackages: []string{"ansible"}},
			{Name: "colima", Role: "Ubuntu VM runtime", Required: true, Check: "colima", BrewPackages: []string{"colima"}},
			{Name: "lima", Role: "VM backend for Colima (limactl)", Required: true, Check: "limactl", BrewPackages: []string{"lima"}},
			{Name: "secretspec", Role: "Host secret provider client", Required: true, Check: "secretspec", BrewPackages: []string{"secretspec"}},
			{Name: "herdr", Role: "Coding-agent multiplexer", Required: false, Check: "herdr", BrewPackages: []string{"herdr"}},
		}
	case "linux":
		return []hostDep{
			{
				Name: "kvm", Role: "Hardware virtualization for Colima/QEMU", Required: true, Check: "",
				AptPackages: []string{"qemu-kvm", "qemu-system", "cpu-checker", "libvirt-clients", "libvirt-daemon-system"},
			},
			{
				Name: "ansible", Role: "Guest provisioning", Required: true, Check: "ansible",
				AptPackages: []string{"ansible"},
			},
			{
				Name: "curl", Role: "Download release binaries and installers", Required: true, Check: "curl",
				AptPackages: []string{"curl", "ca-certificates", "jq"},
			},
			{
				Name: "lima", Role: "VM backend for Colima (limactl)", Required: true, Check: "limactl",
				Installer: "linux-lima",
				Manual:    "install Lima from GitHub releases (see Install on Linux)",
			},
			{
				Name: "colima", Role: "Ubuntu VM runtime", Required: true, Check: "colima",
				Installer: "linux-colima",
				Manual:    "install Colima from GitHub releases (see Install on Linux)",
			},
			{
				Name: "secretspec", Role: "Host secret provider client", Required: true, Check: "secretspec",
				Installer: "secretspec",
				Manual:    "curl -sSL https://install.secretspec.dev | sh",
			},
			{
				Name: "herdr", Role: "Coding-agent multiplexer", Required: false, Check: "herdr",
				Installer: "herdr",
				Manual:    "curl -fsSL https://herdr.dev/install.sh | sh",
			},
		}
	default:
		return nil
	}
}

func checkHostDep(d hostDep) hostDepStatus {
	st := hostDepStatus{hostDep: d, Status: "ok"}
	if d.Name == "kvm" {
		c := doctorKVMCheck()
		st.Status = c.Status
		if c.Status == "ok" {
			st.Status = "ok"
		} else {
			st.Status = "missing"
		}
		st.Detail = c.Detail
		return st
	}
	if d.Check == "" {
		st.Status = "missing"
		st.Detail = "no check configured"
		return st
	}
	if _, err := exec.LookPath(d.Check); err != nil {
		st.Status = "missing"
		st.Detail = err.Error()
	}
	return st
}

func installHint(d hostDep, goos string) string {
	if d.Manual != "" && !autoInstallable(d, goos) {
		return d.Manual
	}
	switch goos {
	case "darwin":
		if len(d.BrewPackages) > 0 {
			return "brew install " + strings.Join(d.BrewPackages, " ")
		}
	case "linux":
		if len(d.AptPackages) > 0 {
			return "sudo apt install -y " + strings.Join(d.AptPackages, " ")
		}
		switch d.Installer {
		case "secretspec":
			return "curl -sSL https://install.secretspec.dev | sh"
		case "herdr":
			return "curl -fsSL https://herdr.dev/install.sh | sh"
		case "linux-lima":
			return "install Lima from https://github.com/lima-vm/lima/releases (see Install on Linux)"
		case "linux-colima":
			return "install Colima from https://github.com/abiosoft/colima/releases (see Install on Linux)"
		}
	}
	if d.Manual != "" {
		return d.Manual
	}
	return ""
}

func autoInstallable(d hostDep, goos string) bool {
	switch goos {
	case "darwin":
		return len(d.BrewPackages) > 0
	case "linux":
		if len(d.AptPackages) > 0 {
			return true
		}
		switch d.Installer {
		case "secretspec", "herdr", "linux-lima", "linux-colima":
			return true
		}
	}
	return false
}

func missingAutoInstallable(statuses []hostDepStatus, includeOptional bool) []hostDep {
	var out []hostDep
	for _, st := range statuses {
		if st.Status == "ok" {
			continue
		}
		if !st.Required && !includeOptional {
			continue
		}
		if autoInstallable(st.hostDep, runtime.GOOS) {
			out = append(out, st.hostDep)
		}
	}
	return out
}

func missingManualOnly(statuses []hostDepStatus, includeOptional bool) []hostDep {
	var out []hostDep
	for _, st := range statuses {
		if st.Status == "ok" {
			continue
		}
		if !st.Required && !includeOptional {
			continue
		}
		if !autoInstallable(st.hostDep, runtime.GOOS) {
			out = append(out, st.hostDep)
		}
	}
	return out
}

func installHostDeps(ctx context.Context, cmd *cobra.Command, deps []hostDep, goos string) error {
	switch goos {
	case "darwin":
		return installDarwinBrew(ctx, cmd, deps)
	case "linux":
		return installLinuxDeps(ctx, cmd, deps)
	default:
		return fmt.Errorf("unsupported OS %s", goos)
	}
}

func installDarwinBrew(ctx context.Context, cmd *cobra.Command, deps []hostDep) error {
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("Homebrew not found on PATH; install from https://brew.sh then re-run kaas deps --install")
	}
	pkgs := uniqueStrings(nil)
	for _, d := range deps {
		pkgs = uniqueStrings(pkgs, d.BrewPackages...)
	}
	if len(pkgs) == 0 {
		return nil
	}
	printMessage(cmd, "Running: brew install %s\n", strings.Join(pkgs, " "))
	c := exec.CommandContext(ctx, "brew", append([]string{"install"}, pkgs...)...)
	c.Stdout, c.Stderr = commandOutput(cmd), os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("brew install failed: %w", err)
	}
	return nil
}

func installLinuxDeps(ctx context.Context, cmd *cobra.Command, deps []hostDep) error {
	apt := uniqueStrings(nil)
	var runSecretspec, runHerdr, runLima, runColima bool
	for _, d := range deps {
		apt = uniqueStrings(apt, d.AptPackages...)
		switch d.Installer {
		case "secretspec":
			runSecretspec = true
		case "herdr":
			runHerdr = true
		case "linux-lima":
			runLima = true
		case "linux-colima":
			runColima = true
		}
	}
	if len(apt) > 0 {
		if _, err := exec.LookPath("apt-get"); err != nil {
			return fmt.Errorf("apt-get not found; install packages manually: sudo apt install -y %s", strings.Join(apt, " "))
		}
		printMessage(cmd, "Running: sudo apt-get update && sudo apt-get install -y %s\n", strings.Join(apt, " "))
		update := exec.CommandContext(ctx, "sudo", "apt-get", "update")
		update.Stdout, update.Stderr = commandOutput(cmd), os.Stderr
		if err := update.Run(); err != nil {
			return fmt.Errorf("apt-get update failed: %w", err)
		}
		args := append([]string{"apt-get", "install", "-y"}, apt...)
		install := exec.CommandContext(ctx, "sudo", args...)
		install.Stdout, install.Stderr = commandOutput(cmd), os.Stderr
		if err := install.Run(); err != nil {
			return fmt.Errorf("apt-get install failed: %w", err)
		}
	}
	if runLima {
		if err := installLinuxLima(ctx, cmd); err != nil {
			return err
		}
	}
	if runColima {
		if err := installLinuxColima(ctx, cmd); err != nil {
			return err
		}
	}
	if runSecretspec {
		printMessage(cmd, "Running SecretSpec installer…\n")
		c := exec.CommandContext(ctx, "sh", "-c", "curl -sSL https://install.secretspec.dev | sh")
		c.Stdout, c.Stderr = commandOutput(cmd), os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("secretspec installer failed: %w", err)
		}
	}
	if runHerdr {
		printMessage(cmd, "Running Herdr installer…\n")
		c := exec.CommandContext(ctx, "sh", "-c", "curl -fsSL https://herdr.dev/install.sh | sh")
		c.Stdout, c.Stderr = commandOutput(cmd), os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("herdr installer failed: %w", err)
		}
	}
	return nil
}

func installLinuxLima(ctx context.Context, cmd *cobra.Command) error {
	arch, err := linuxReleaseArch()
	if err != nil {
		return err
	}
	limaArch := arch.lima
	script := fmt.Sprintf(`set -euo pipefail
VERSION=$(curl -fsSL https://api.github.com/repos/lima-vm/lima/releases/latest | jq -r .tag_name)
curl -fsSL "https://github.com/lima-vm/lima/releases/download/${VERSION}/lima-${VERSION#v}-Linux-%s.tar.gz" \
  | sudo tar Cxz - /usr/local
`, limaArch)
	printMessage(cmd, "Installing Lima (%s) into /usr/local…\n", limaArch)
	c := exec.CommandContext(ctx, "sh", "-c", script)
	c.Stdout, c.Stderr = commandOutput(cmd), os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("lima install failed: %w", err)
	}
	return nil
}

func installLinuxColima(ctx context.Context, cmd *cobra.Command) error {
	arch, err := linuxReleaseArch()
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`set -euo pipefail
curl -fsSL -o /tmp/colima "https://github.com/abiosoft/colima/releases/latest/download/colima-Linux-%s"
sudo install /tmp/colima /usr/local/bin/colima
`, arch.colima)
	printMessage(cmd, "Installing Colima (%s) into /usr/local/bin…\n", arch.colima)
	c := exec.CommandContext(ctx, "sh", "-c", script)
	c.Stdout, c.Stderr = commandOutput(cmd), os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("colima install failed: %w", err)
	}
	return nil
}

type linuxArches struct {
	lima   string
	colima string
}

func linuxReleaseArch() (linuxArches, error) {
	m := runtime.GOARCH
	switch m {
	case "amd64":
		return linuxArches{lima: "x86_64", colima: "x86_64"}, nil
	case "arm64":
		return linuxArches{lima: "aarch64", colima: "arm64"}, nil
	default:
		return linuxArches{}, fmt.Errorf("unsupported linux arch %q", m)
	}
}

func uniqueStrings(dst []string, add ...string) []string {
	seen := map[string]struct{}{}
	for _, s := range dst {
		seen[s] = struct{}{}
	}
	for _, s := range add {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		dst = append(dst, s)
	}
	return dst
}

// installGalaxyCollections converges the collections required by KaaS's
// canonical playbooks. It is called by both `kaas deps galaxy` and the complete
// `up` workflow so a first sandbox does not require a separate setup ritual.
func installGalaxyCollections(ctx context.Context, cmd *cobra.Command, app *App) error {
	dir := app.Config.AnsibleDir()
	if dir == "" || dir == "." {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	req := filepath.Join(dir, "requirements.yml")
	if _, err := os.Stat(req); err != nil {
		return fmt.Errorf("requirements.yml not found at %s (run from the project or set paths.ansible_dir)", req)
	}
	if _, err := exec.LookPath("ansible-galaxy"); err != nil {
		return fmt.Errorf("ansible-galaxy not found on PATH (install Ansible first; try: kaas deps --install)")
	}
	app.Log.Info("installing galaxy collections", "file", req)
	c := exec.CommandContext(ctx, "ansible-galaxy", "collection", "install", "-r", req)
	c.Dir, c.Stdout, c.Stderr = dir, commandOutput(cmd), os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("ansible-galaxy failed: %w", err)
	}
	return nil
}
