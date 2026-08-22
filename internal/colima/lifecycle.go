package colima

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StartOptions configures `colima start`.
type StartOptions struct {
	Name     string
	CPUs     int
	Memory   int // GiB
	Disk     int // GiB
	Arch     string
	SSHPort  int
	Hostname string // guest OS hostname; defaults to profile name when set by callers
}

// SSHConfig is parsed from `colima ssh-config`.
type SSHConfig struct {
	Host         string
	User         string
	Port         int
	IdentityFile string
}

var (
	reHostName = regexp.MustCompile(`(?m)^\s*HostName\s+(\S+)`)
	reUser     = regexp.MustCompile(`(?m)^\s*User\s+(\S+)`)
	rePort     = regexp.MustCompile(`(?m)^\s*Port\s+(\S+)`)
	reIdentity = regexp.MustCompile(`(?m)^\s*IdentityFile\s+(\S+)`)
)

// Start creates or starts a Colima profile.
func (c *Client) Start(ctx context.Context, opts StartOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	args := []string{"start", opts.Name}
	if opts.CPUs > 0 {
		args = append(args, "--cpus", strconv.Itoa(opts.CPUs))
	}
	if opts.Memory > 0 {
		args = append(args, "--memory", strconv.Itoa(opts.Memory))
	}
	if opts.Disk > 0 {
		args = append(args, "--disk", strconv.Itoa(opts.Disk))
	}
	if opts.Arch != "" {
		args = append(args, "--arch", opts.Arch)
	}
	if opts.SSHPort > 0 {
		args = append(args, "--ssh-port", strconv.Itoa(opts.SSHPort))
	}
	if opts.Hostname != "" {
		args = append(args, "--hostname", opts.Hostname)
	}
	args = append(args, "--ssh-config")

	return c.runStreaming(ctx, 15*time.Minute, args...)
}

// Stop stops a Colima profile without deleting its data.
func (c *Client) Stop(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	return c.runStreaming(ctx, 5*time.Minute, "stop", name)
}

// Clone duplicates the Lima instance backing a Colima profile and starts the clone.
// Lima requires the source instance to be stopped while cloning.
func (c *Client) Clone(ctx context.Context, source, target string, sshPort int) error {
	if source == "" || target == "" {
		return fmt.Errorf("source and target profile names are required")
	}
	if source == target {
		return fmt.Errorf("source and target profile names must differ")
	}
	if sshPort < 0 || sshPort > 65535 {
		return fmt.Errorf("invalid ssh port: %d", sshPort)
	}
	// Colima stores profiles as Lima instances with a "colima-" prefix.
	// LIMA_HOME must point at Colima's Lima state directory.
	args := []string{"clone", "colima-" + source, "colima-" + target, "--start"}
	if sshPort > 0 {
		args = append(args, "--ssh-port", strconv.Itoa(sshPort))
	}
	return c.runLimaStreaming(ctx, 15*time.Minute, args...)
}

// Delete removes a Colima profile and its data.
func (c *Client) Delete(ctx context.Context, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	args := []string{"delete", name}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--data")
	return c.runStreaming(ctx, 5*time.Minute, args...)
}

// SSHConfig returns parsed SSH settings for a profile.
func (c *Client) SSHConfig(ctx context.Context, name string) (*SSHConfig, error) {
	out, err := c.run(ctx, "ssh-config", name)
	if err != nil {
		return nil, err
	}
	return ParseSSHConfig(string(out))
}

// ParseSSHConfig extracts HostName/User/Port/IdentityFile from ssh config text.
func ParseSSHConfig(text string) (*SSHConfig, error) {
	cfg := &SSHConfig{}
	if m := reHostName.FindStringSubmatch(text); len(m) == 2 {
		cfg.Host = m[1]
	}
	if m := reUser.FindStringSubmatch(text); len(m) == 2 {
		cfg.User = m[1]
	}
	if m := rePort.FindStringSubmatch(text); len(m) == 2 {
		p, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse Port: %w", err)
		}
		cfg.Port = p
	}
	if m := reIdentity.FindStringSubmatch(text); len(m) == 2 {
		cfg.IdentityFile = m[1]
	}
	if cfg.Host == "" || cfg.Port == 0 {
		return nil, fmt.Errorf("incomplete ssh-config (host=%q port=%d)", cfg.Host, cfg.Port)
	}
	return cfg, nil
}

// ProfileStatus returns the status string for a named profile, or "".
func (c *Client) ProfileStatus(ctx context.Context, name string) (string, error) {
	profiles, err := c.List(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range profiles {
		if p.Name == name {
			return p.Status, nil
		}
	}
	return "", nil
}

func (c *Client) runLimaStreaming(ctx context.Context, timeout time.Duration, args ...string) error {
	bin := c.LimaBin
	if bin == "" {
		bin = "limactl"
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if limaHome := os.Getenv("COLIMA_LIMA_HOME"); limaHome != "" {
		cmd.Env = append(os.Environ(), "LIMA_HOME="+limaHome)
	} else if home, err := os.UserHomeDir(); err == nil {
		cmd.Env = append(os.Environ(), "LIMA_HOME="+filepath.Join(home, ".colima", "_lima"))
	}
	cmd.Stdout = c.Output()
	cmd.Stderr = c.ErrorOutput()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
	return nil
}

func (c *Client) runStreaming(ctx context.Context, timeout time.Duration, args ...string) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Stdout = c.Output()
	cmd.Stderr = c.ErrorOutput()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("colima %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// UsedSSHPorts best-effort extracts ports from running profiles' ssh-config.
func (c *Client) UsedSSHPorts(ctx context.Context) ([]int, error) {
	profiles, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	var ports []int
	for _, p := range profiles {
		cfg, err := c.SSHConfig(ctx, p.Name)
		if err != nil {
			continue
		}
		ports = append(ports, cfg.Port)
	}
	return ports, nil
}

// RunSSHConfigRaw is used by tests.
func RunSSHConfigRaw(out []byte, err error) (*SSHConfig, error) {
	if err != nil {
		return nil, err
	}
	return ParseSSHConfig(string(bytes.TrimSpace(out)))
}
