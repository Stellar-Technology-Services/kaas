// Package kaas exposes canonical KaaS workspace assets for the installed CLI.
package kaas

import "embed"

// Assets contains the canonical project configuration and SecretSpec manifest.
// kaas init installs these into the per-user workspace. Ansible playbooks are
// seeded separately by cloning a git repository (default:
// https://github.com/Stellar-Technology-Services/ansible).
//
//go:embed kaas.example.yaml secretspec.toml
var Assets embed.FS
