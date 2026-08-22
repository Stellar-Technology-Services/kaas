package secretspec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client shells out to the secretspec CLI.
type Client struct {
	Bin    string
	File   string // path to secretspec.toml
	Dir    string // working directory
	Stdout io.Writer
	Stderr io.Writer
}

func New(repoRoot string) *Client {
	return &Client{
		Bin:  "secretspec",
		File: filepath.Join(repoRoot, "secretspec.toml"),
		Dir:  repoRoot,
	}
}

func (c *Client) Available() error {
	if _, err := exec.LookPath(c.bin()); err != nil {
		return fmt.Errorf("secretspec not found on PATH (macOS: brew install secretspec; Linux: curl -sSL https://install.secretspec.dev | sh)")
	}
	return nil
}

func (c *Client) EnsureManifest() error {
	if c.File == "" {
		return fmt.Errorf("secretspec.toml path not set")
	}
	if _, err := os.Stat(c.File); err != nil {
		return fmt.Errorf("missing %s — run kaas init (or restore secretspec.toml)", c.File)
	}
	return nil
}

// SetPrompt runs interactive `secretspec set` (user types the value).
func (c *Client) SetPrompt(key, profile string) error {
	cmd := c.cmd("set", key, "--profile", profile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = c.streamOutput()
	cmd.Stderr = c.errorOutput()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("secretspec set %s: %w", key, err)
	}
	return nil
}

// SetValue sets a secret non-interactively via stdin.
func (c *Client) SetValue(key, profile, value string) error {
	cmd := c.cmd("set", key, "--profile", profile)
	cmd.Stdin = strings.NewReader(value + "\n")
	cmd.Stdout = c.streamOutput()
	cmd.Stderr = c.errorOutput()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("secretspec set %s: %w", key, err)
	}
	return nil
}

// ExportJSON resolves one profile to name→value.
func (c *Client) ExportJSON(profile, scope string) (map[string]string, error) {
	args := []string{"export", "--format", "json", "--profile", profile, "--reason", "kaas secrets"}
	if scope != "" {
		args = append(args, "--scope", scope)
	}
	out, err := c.output(args...)
	if err != nil {
		return nil, err
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 || string(out) == "null" {
		return map[string]string{}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("parse secretspec export json: %w\n%s", err, out)
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}

// ResolveSandbox merges profile "default" with profile sandboxName (sandbox wins).
func (c *Client) ResolveSandbox(sandboxName, scope string) (map[string]string, error) {
	base, err := c.ExportJSON("default", scope)
	if err != nil {
		return nil, fmt.Errorf("export profile default: %w", err)
	}
	merged := make(map[string]string, len(base))
	for k, v := range base {
		if strings.TrimSpace(v) != "" {
			merged[k] = v
		}
	}
	if sandboxName == "" || sandboxName == "default" {
		return merged, nil
	}
	over, err := c.ExportJSON(sandboxName, scope)
	if err != nil {
		if isMissingProfile(err) {
			return merged, nil
		}
		return nil, fmt.Errorf("export profile %s: %w", sandboxName, err)
	}
	for k, v := range over {
		if strings.TrimSpace(v) != "" {
			merged[k] = v
		}
	}
	return merged, nil
}

func (c *Client) Check(profile string) error {
	cmd := c.cmd("check", "--profile", profile)
	cmd.Stdout = c.streamOutput()
	cmd.Stderr = c.errorOutput()
	return cmd.Run()
}

func (c *Client) streamOutput() io.Writer {
	if c.Stdout != nil {
		return c.Stdout
	}
	return os.Stdout
}
func (c *Client) errorOutput() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

func (c *Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "secretspec"
}

func (c *Client) cmd(args ...string) *exec.Cmd {
	if c.File != "" {
		args = append([]string{"--file", c.File}, args...)
	}
	cmd := exec.Command(c.bin(), args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	return cmd
}

func (c *Client) output(args ...string) ([]byte, error) {
	cmd := c.cmd(args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

func isMissingProfile(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "profile") &&
		(strings.Contains(s, "not found") || strings.Contains(s, "unknown") || strings.Contains(s, "does not exist"))
}

// ToAnsibleVars maps ENV_NAME keys to lowercase ansible vars for deploy-secrets.yml.
func ToAnsibleVars(secrets map[string]string) map[string]string {
	out := make(map[string]string, len(secrets))
	for k, v := range secrets {
		if strings.TrimSpace(v) == "" {
			continue
		}
		out[strings.ToLower(k)] = v
	}
	return out
}
