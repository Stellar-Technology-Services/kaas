package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const projectConfigTemplate = `# KaaS project workspace contract.
# Application defaults (state, SSH identity, Ansible assets) live in ~/.kaas/kaas.yaml.
# This project file may override only CPU, memory, disk, and workspace settings.

defaults:
  cpus: 4
  memory_gib: 8
  disk_gib: 64

workspace:
  git_auth: agent # agent (default), guest_key, https, or none
  # Optional: validate a checked-in Varlock environment contract before setup
  # (runs in sources[0] only).
  # varlock:
  #   schema: .env.schema
  #   validate_command: varlock load --agent --summary-stderr

  # Managed guest checkouts. sources[0] is primary (Varlock + scripts/kaas-setup).
  sources:
    - # repository: git@github.com:your-org/your-project.git
      # branch: main
      directory: %s
    # - repository: git@github.com:your-org/your-other-repo.git
    #   branch: main
    #   directory: ~/workspace/your-other-repo

  # Optional: publish guest listeners as http://<service>.<sandbox>.kaas.test
  # services:
  #   frontend:
  #     port: 3000
  #     scheme: http
  #   api:
  #     port: 8080
  #     scheme: http

# Optional, versioned egress policy. This declares destinations and SecretSpec
# key identifiers only—never secret values. proxy_preferred is not an enforced
# boundary; proxy_only is reserved until a platform enforcement backend exists.
# egress:
#   schema_version: 1
#   mode: proxy_preferred
#   default: deny
#   rules:
#     - host: github.com                  # reachable without a credential
#     - host: api.github.com
#       inject_secret:
#         key: GITHUB_TOKEN
#         type: bearer                    # bearer, basic, or header
`

const projectSetupTemplate = `#!/usr/bin/env bash
# KaaS project setup contract.
#
# KaaS runs this script inside the guest after it has cloned/updated the
# workspace and, when configured, validated the project's Varlock schema.
# Keep it safe to run repeatedly: install dependencies, start local services,
# apply idempotent migrations, or create development fixtures here.
#
# Examples:
#   pnpm install --frozen-lockfile
#   docker compose up -d postgres redis
#   varlock run -- pnpm db:migrate
#
# Do not put secret values in this file. Use SecretSpec and/or Varlock.
set -euo pipefail

# Add project bootstrap steps below.
`

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Create and manage a repository KaaS workspace contract"}
	cmd.AddCommand(&cobra.Command{
		Use: "init", Short: "Add kaas.yaml and scripts/kaas-setup to the current Git project", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			git := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
			output, err := git.Output()
			if err != nil {
				return fmt.Errorf("project init must run inside a Git repository")
			}
			cwd = string(output)
			cwd = strings.TrimSpace(cwd)
			name := filepath.Base(cwd)
			configPath := filepath.Join(cwd, "kaas.yaml")
			scriptPath := filepath.Join(cwd, "scripts", "kaas-setup")
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf("%s already exists; edit it instead", configPath)
			}
			if _, err := os.Stat(scriptPath); err == nil {
				return fmt.Errorf("%s already exists; preserve it and create kaas.yaml manually if needed", scriptPath)
			}
			if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
				return fmt.Errorf("create scripts directory: %w", err)
			}
			if err := os.WriteFile(configPath, []byte(fmt.Sprintf(projectConfigTemplate, "~/workspace/"+name)), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", configPath, err)
			}
			if err := os.WriteFile(scriptPath, []byte(projectSetupTemplate), 0o755); err != nil {
				_ = os.Remove(configPath)
				return fmt.Errorf("write %s: %w", scriptPath, err)
			}
			printStyled(cmd, styleSuccess, "Created %s\nCreated %s\nNext: edit kaas.yaml, then commit both files and run kaas up %s\n", configPath, scriptPath, name)
			return nil
		},
	})
	return cmd
}
