package colima

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Profile is a Colima VM profile from `colima list --json`.
type Profile struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Arch    string `json:"arch"`
	CPUs    int    `json:"cpus"`
	Memory  int    `json:"memory"` // MiB in some versions; treated as opaque cache
	Disk    int    `json:"disk"`
	Address string `json:"address"`
}

// Client wraps the colima CLI.
type Client struct {
	Bin     string
	Timeout time.Duration
	// LimaBin is used for operations that Colima does not expose, such as cloning.
	LimaBin string
	Stdout  io.Writer
	Stderr  io.Writer
}

func New() *Client {
	return &Client{
		Bin:     "colima",
		LimaBin: "limactl",
		Timeout: 30 * time.Second,
	}
}

// Output returns the configured process stdout, defaulting to the terminal.
func (c *Client) Output() io.Writer {
	if c.Stdout != nil {
		return c.Stdout
	}
	return os.Stdout
}

// ErrorOutput returns the configured process stderr, defaulting to the terminal.
func (c *Client) ErrorOutput() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

// Available reports whether the colima binary is on PATH.
func (c *Client) Available() error {
	bin := c.bin()
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("colima not found on PATH (see docs: Install on macOS or Install on Linux)")
	}
	return nil
}

// List returns Colima profiles (NDJSON from `colima list --json`).
func (c *Client) List(ctx context.Context) ([]Profile, error) {
	out, err := c.run(ctx, "list", "--json")
	if err != nil {
		// Empty install often exits non-zero with no profiles.
		if strings.TrimSpace(string(out)) == "" {
			return nil, nil
		}
		return nil, err
	}
	var profiles []Profile
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var p Profile
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			return nil, fmt.Errorf("parse colima list json: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// StatusMap returns name → status for quick lookup.
func (c *Client) StatusMap(ctx context.Context) (map[string]string, error) {
	profiles, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(profiles))
	for _, p := range profiles {
		m[p.Name] = p.Status
	}
	return m, nil
}

func (c *Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "colima"
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.bin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("colima %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
