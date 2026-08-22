package dnsconfig

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const marker = "# managed by kaas dns enable"

// Enable installs split-horizon resolver config so domain queries hit dnsListen.
// Requires elevated privileges (writes under /etc).
func Enable(domain, dnsListen string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if strings.HasSuffix(domain, ".local") || domain == "local" {
		return fmt.Errorf("domain %q uses .local which conflicts with mDNS; use kaas.test", domain)
	}
	host, port, err := net.SplitHostPort(dnsListen)
	if err != nil {
		return fmt.Errorf("dns_listen: %w", err)
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 {
		return fmt.Errorf("invalid DNS port in %s", dnsListen)
	}

	switch runtime.GOOS {
	case "darwin":
		return enableDarwin(domain, host, portNum)
	case "linux":
		return enableLinux(domain, host, portNum)
	default:
		return fmt.Errorf("kaas dns enable is not supported on %s", runtime.GOOS)
	}
}

// Disable removes resolver config written by Enable.
func Disable(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return disableDarwin(domain)
	case "linux":
		return disableLinux(domain)
	default:
		return fmt.Errorf("kaas dns disable is not supported on %s", runtime.GOOS)
	}
}

// Status describes whether resolver hooks appear installed.
func Status(domain string) (installed bool, detail string, err error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	switch runtime.GOOS {
	case "darwin":
		path := filepath.Join("/etc/resolver", domain)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return false, path + " not present", nil
			}
			return false, "", readErr
		}
		return strings.Contains(string(data), marker), path, nil
	case "linux":
		path := linuxDropInPath(domain)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return false, path + " not present", nil
			}
			return false, "", readErr
		}
		return strings.Contains(string(data), marker), path, nil
	default:
		return false, "unsupported OS", nil
	}
}

func enableDarwin(domain, nameserver string, port int) error {
	dir := "/etc/resolver"
	body := fmt.Sprintf("%s\nnameserver %s\nport %d\n", marker, nameserver, port)
	tmp, err := os.CreateTemp("", "kaas-resolver-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, _ = tmp.WriteString(body)
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	dest := filepath.Join(dir, domain)
	script := fmt.Sprintf("mkdir -p %s && cp %s %s && chmod 644 %s",
		shellQuote(dir), shellQuote(tmpPath), shellQuote(dest), shellQuote(dest))
	return runElevated(script)
}

func disableDarwin(domain string) error {
	dest := filepath.Join("/etc/resolver", domain)
	return runElevated("rm -f " + shellQuote(dest))
}

func linuxDropInPath(domain string) string {
	return filepath.Join("/etc/systemd/resolved.conf.d", "kaas-"+domain+".conf")
}

func enableLinux(domain, nameserver string, port int) error {
	// systemd-resolved: route only this domain to our stub. Port is not
	// configurable in Domains= lines; when non-53 we document dnsmasq fallback.
	body := fmt.Sprintf(`%s
[Resolve]
DNS=%s:%d
Domains=~%s
`, marker, nameserver, port, domain)
	tmp, err := os.CreateTemp("", "kaas-resolved-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, _ = tmp.WriteString(body)
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	dest := linuxDropInPath(domain)
	script := fmt.Sprintf(
		"mkdir -p /etc/systemd/resolved.conf.d && cp %s %s && chmod 644 %s && (systemctl reload systemd-resolved 2>/dev/null || systemctl restart systemd-resolved 2>/dev/null || true)",
		shellQuote(tmpPath), shellQuote(dest), shellQuote(dest),
	)
	return runElevated(script)
}

func disableLinux(domain string) error {
	dest := linuxDropInPath(domain)
	script := fmt.Sprintf(
		"rm -f %s && (systemctl reload systemd-resolved 2>/dev/null || systemctl restart systemd-resolved 2>/dev/null || true)",
		shellQuote(dest),
	)
	return runElevated(script)
}

func runElevated(script string) error {
	if os.Geteuid() == 0 {
		cmd := exec.Command("sh", "-c", script)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found; rerun as root or install sudo")
	}
	cmd := exec.Command(sudo, "sh", "-c", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
