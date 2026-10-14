# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Borrowed from a study of boat.dev's sandbox ergonomics
(`notes/boat-dev-ui-borrowings.md`): the local web UI now drives the CLI, and
the CLI gains presets, templates, auto-stop, and detached guest commands.

### Added

- **UI job runner.** Every dashboard action starts one
  `kaas <verb> … --jsonl` background process, recorded in a new `jobs` table
  with a log under `~/.kaas/ui/jobs/<id>.log`, and polled with HTMX. One active
  job per sandbox. New **Jobs** page and `/jobs/<id>/log`.
- **New sandbox form** (`/sandboxes/new`): suggested name, size preset, trust
  preset, start from a template, auto-stop, optional project directory or
  repository URL. Submits a `kaas up` job and shows a pending page until the
  registry row exists.
- **Lifecycle buttons** gated by observed state: Start, Stop, Extend, Re-run
  setup, Sync services, Fork…, Save as template…, Destroy… (type the name to
  confirm). The fork dialog states that the source is stopped briefly and that
  secrets/egress are not copied.
- **Readiness badge** (Ready / Starting phase N/6 / Working / Stopped /
  Setup failed / Failed) derived from registry facts only; **creds** chip with
  the exact fix command; **host capacity meter** on Overview.
- `kaas up --size | --repo/--branch | --trust untrusted | --from <template> |
  --auto-stop`; `kaas create --size | --from | --auto-stop`;
  `defaults.sizes` presets (`small`, `large`).
- **Templates**: `kaas template save <sandbox> <template>` / `list` / `rm`
  (a stopped golden disk, `role=template`); `--from` clones it and skips
  provision/agents. UI **Templates** page.
- **Auto-stop**: `defaults.auto_stop` (built-in `never`), `--auto-stop`,
  `kaas extend [name] --for <dur> | --never`; the UI daemon enforces deadlines
  every 30 s. Deadline is now + TTL at each start; templates are exempt.
- **Phase logs**: provision, agents, pubkey, and setup tee their output to
  `<paths.logs_dir>/<sandbox>/<phase>.log` (default `~/.kaas/logs`); the
  sandbox page lists and tails them.
- `kaas contract show | adopt`: persisted contract, field-level drift from the
  project `kaas.yaml`, explicit re-adopt. UI **Environment** card with drift and
  a Re-adopt button.
- Optional repository pre-stop hook `scripts/kaas-prestop`, run by `kaas stop`
  and auto-stop before the VM stops.
- `kaas exec <name> -- <cmd>` with `--detach`, `--status`, `--attach`,
  `--kill`, `--history`, `--cwd`, `--timeout`; detached output lives on the
  guest under `~/.kaas-runs/<id>.log`, indexed in a new `sandbox_commands`
  table. `kaas ps [name]` lists the busiest guest processes.
- UI **Run in the guest**: detached command box, agent prompt box
  (`pi`/`omp`/`prime-agent -p …`), live command list with Kill, Processes.
- Service rows on the sandbox page show **health** (host-port probe) and offer a
  systemd unit snippet for services that are declared but not listening.

### Changed

- `kaas stop` clears the auto-stop deadline and marks running detached
  commands `lost`; `kaas start` refuses templates and re-arms auto-stop.
- The UI's security note no longer omits destroy/fork; destroy requires typing
  the sandbox name.
- State schema migrations 003 (`jobs`), 004 (`sandboxes.role`,
  `auto_stop_ttl`, `auto_stop_at`), 005 (`sandbox_commands`).

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
