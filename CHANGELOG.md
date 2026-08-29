# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.5] - 2026-08-29

### Fixed

- KaaS now loads and runs when `workspace.sources` is empty or still a
  scaffolded placeholder (`kaas project init` no longer requires a
  repository before any command works). Checkout, Varlock, and setup are
  skipped until a source is configured.

[0.2.5]: https://github.com/Stellar-Technology-Services/kaas/releases/tag/v0.2.5

## [0.2.4] - 2026-08-22

### Changed

- Default Ansible git repository cloned by `kaas init` is now
  [`https://github.com/kaas-dev/ansible`](https://github.com/kaas-dev/ansible)
  (override with `--ansible-repo` or the init prompt). Existing
  `~/.kaas/ansible` clones keep their current remote until updated.

[0.2.4]: https://github.com/Stellar-Technology-Services/kaas/releases/tag/v0.2.4
