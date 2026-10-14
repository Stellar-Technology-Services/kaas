---
title: CLI Reference
description: Every kaas command, its flags, and usage.
---

`kaas` is the single CLI for the sandbox lifecycle.

## Command summary

`kaas --help` groups commands by operator stage (host → machine → guest →
credentials → access → observe). Within each group, commands are listed in
likely usage order (for example `init` before `project` and `config`). Paths
stay flat (`kaas create`, not `kaas vm create`).

### Host setup

| Command | Description |
|---|---|
| `kaas init` | Initialize `~/.kaas`: clone Ansible from git (default `https://github.com/kaas-dev/ansible`), install config and SecretSpec; `--dir`, `--ansible-repo` |
| `kaas project init` | Scaffold a Git project `kaas.yaml` and `scripts/kaas-setup` contract |
| `kaas config show` | Print effective configuration and its user/project file sources |
| `kaas deps` | Show host dependencies for this OS; `--install` installs missing ones |
| `kaas deps galaxy` | Manually refresh Ansible Galaxy collections (`kaas up` installs them automatically) |
| `kaas completion …` | Shell completion |

### Machine lifecycle

| Command | Description |
|---|---|
| `kaas up <name>` | Install dependencies, create VM, provision toolchains, agents, optional secrets, and workspace setup; `--size`, `--cpus/--memory/--disk`, `--repo/--branch`, `--trust`, `--from <template>`, `--auto-stop` |
| `kaas create <name>` | Create / start Colima VM; `--size`, `--port`, `--cpus/--memory/--disk/--arch`, `--from <template>`, `--auto-stop` |
| `kaas start [name]` | Start a stopped sandbox with its existing disk and registry entry; re-arms auto-stop; `--auto-stop` changes the TTL |
| `kaas stop [name]` | Run `scripts/kaas-prestop` if present, then stop the Colima VM while preserving its disk and registry entry |
| `kaas extend [name] --for <dur>` / `--never` | Set the auto-stop deadline to now + duration (remaining time, not additive), or disable it |
| `kaas fork <sandbox-id> <new-name>` | Clone an existing sandbox VM under a new name |
| `kaas template save <sandbox> <template>` | Clone a sandbox's disk as a stopped **template** for `--from` (saving the same name again replaces it) |
| `kaas template list` / `rm <template>` | List templates; delete one (`-y` skips confirmation) |
| `kaas destroy [name]` | Confirm, then delete the Colima VM and registry row (retains registry if provider delete fails; `--force` drops registry anyway) |

### Guest readiness

| Command | Description |
|---|---|
| `kaas provision [name]` | Install packages and language toolchains |
| `kaas agents [name]` | Install coding agents |
| `kaas setup [name]` | Run the repository standard `scripts/kaas-setup` contract |
| `kaas check [name]` | Dry-run provision (`ansible --check`) |

### Secrets and egress

| Command | Description |
|---|---|
| `kaas secrets init` | Verify `secretspec.toml` and print setup hints |
| `kaas secrets set KEY` | Store a secret (`--sandbox` = profile) |
| `kaas secrets deploy [name]` | Resolve SecretSpec and sync onto the VM |
| `kaas secrets check` | `secretspec check` for a profile |
| `kaas egress plan` | Print the effective redacted project egress policy |
| `kaas egress configure [name]` | Start proxy and configure guest CA/proxy/topology |
| `kaas egress status [name]` | Show redacted proxy status |
| `kaas egress reload [name]` | Rotate delegated values without guest deployment |
| `kaas egress verify [name]` | Run proxy-only bypass probes |
| `kaas pubkey [name]` | Generate / print sandbox SSH public key |

### Sandbox access

| Command | Description |
|---|---|
| `kaas ssh [name]` | SSH into the VM |
| `kaas exec <name> -- <cmd>` | Run a command in the guest; `--detach`, `--status <id>`, `--attach <id>`, `--kill <id>`, `--history`, `--cwd`, `--timeout` |
| `kaas ps [name]` | List the busiest guest processes |
| `kaas cp <source> <destination>` | Copy a file between host and one sandbox |
| `kaas ping [name]` | Ansible ping |
| `kaas services list [name]` | List published sandbox services |
| `kaas services sync [name]` | Allocate host ports, SSH forwards, reload ingress |
| `kaas services open [name] <service>` | Print (and try to open) a service URL |
| `kaas services status` | Show ingress runtime status |
| `kaas dns enable` | Install split-horizon resolver for `*.kaas.test` (sudo) |
| `kaas dns disable` | Remove resolver hooks |
| `kaas dns status` | Show whether DNS hooks are installed |
| `kaas ingress serve` | Run host ingress/DNS (normally lifecycle-managed) |
| `kaas ui` / `serve` | Localhost management dashboard (foreground; default `127.0.0.1:18181`) |
| `kaas ui start` / `stop` / `status` / `open` | Background dashboard daemon; the UI's lifecycle buttons run `kaas … --jsonl` as background jobs |

### Inspect and diagnose

| Command | Description |
|---|---|
| `kaas list` | List all sandboxes (`ls`) |
| `kaas inspect [name]` | Show stored and current facts for a sandbox |
| `kaas contract show [name]` | Print the persisted redacted contract and any drift from the project `kaas.yaml` |
| `kaas contract adopt [name]` | Explicitly replace the persisted contract with the current project stack (reconcile) |
| `kaas doctor [name]` | Check host prerequisites and optional sandbox connectivity |
| `kaas events [name]` | Show recorded sandbox lifecycle events |

### Additional commands

| Command | Description |
|---|---|
| `kaas version` | Print version |
| `kaas help` | Help about any command |

If `[name]` is omitted and exactly one sandbox is registered, that name is used.

## Add a project contract

From a Git repository root:

```bash
kaas project init
```

This creates a constrained project `kaas.yaml` and executable
`./scripts/kaas-setup`. Project configuration can override only CPU, memory,
disk, and workspace settings; application configuration remains global in
`~/.kaas/kaas.yaml`. KaaS discovers a project `kaas.yaml` from any child
directory and continues to use the global `~/.kaas/ansible/` provisioning
assets.

## Inspect effective configuration

```bash
kaas config show
kaas config show --jsonl
```

`config show` reports the resolved values after built-in defaults, the user
workspace config, a discovered or explicit project config, environment
variables, and flags have been applied. Its YAML output includes the user and
project configuration files used as sources. JSONL emits a `config` record with
the same effective configuration and sources.

## Command options and aliases

`kaas up` accepts the same sizing flags as `create` plus a few ergonomics
presets so a sandbox needs no project `kaas.yaml`:

```bash
kaas up scratch --size small                          # 2 vCPU / 4 GiB / 32 GiB
kaas up web --repo git@github.com:acme/web.git --branch main
kaas up vendor-eval --trust untrusted                 # proxy_only, no guest secrets
```

- `--size <name>` picks a preset from `defaults.sizes` (`small`, `large`, …);
  `default` means `defaults.cpus/memory_gib/disk_gib`. Explicit `--cpus`,
  `--memory`, `--disk` still override the preset.
- `--repo <url> [--branch <b>]` makes that repository the primary
  `workspace.sources[0]`, cloned under `~/workspace/<repo>`. It is recorded in
  the persisted contract like any project source.
- `--trust untrusted` forces delegated egress in `proxy_only` mode with a
  default-deny policy. When the project declares no egress rules, GitHub hosts
  are allowed **without** credential injection so checkout works. `--trust
  trusted` (the default) keeps the configured behaviour (`kaas secrets deploy`
  into the guest in `open` mode).

`kaas create` accepts VM sizing overrides. Values not supplied on the command
line come from `--size`, then `kaas.yaml` (or the built-in defaults):

```bash
kaas create my-app --port 6025 --cpus 6 --memory 12 --disk 80 --arch aarch64
kaas create scratch --size small
```

- `--port` sets the host SSH port; otherwise the next available port starting
  at `defaults.ssh_port_base` is allocated.
- `kaas create` only requires Colima/Lima and the state database; Ansible assets are required when provisioning with `kaas up` or `kaas provision`.
- New sandbox names must match a strict grammar: lowercase letters, digits, and
  internal hyphens only (for example `my-app`), at most 63 characters. Names
  such as `default`, paths, whitespace, underscores, dots, and uppercase are
  rejected. Existing registry rows with looser names keep working.
- `kaas fork` applies the same grammar to the new name only.
- `--cpus`, `--memory`, `--disk`, and `--arch` override the VM's vCPUs,
  memory GiB, disk GiB, and architecture.
- `kaas up [name] --check` performs a dry-run of provisioning instead of
  changing the guest.
- `kaas list --refresh` reconciles the registry with the provider and **reports
  discrepancies** (it does not silently rewrite rows). Columns include
  `DESIRED` and `OBSERVED` lifecycle state.
  `--repo` filters by managed workspace repository URL or directory; `--jsonl`
  emits one record per sandbox (and discrepancy records when refreshing).
  See [Machine lifecycle](#machine-lifecycle) for the full stage model.
- `kaas stop [name]` stops the VM but preserves its disk, registry entry, and
  contract; run `kaas start <name>` to resume it.
- `kaas destroy <name>` prompts before permanently deleting the VM and its data. Pass `-y` (or `--yes`) to skip confirmation. If provider delete fails, the registry row is retained with observed `failed`/`unknown` so you can retry; pass `--force` to remove the registry (and host egress runtime) anyway. `--force` does not imply `-y`.

`kaas list` is also available as `kaas ls`; `kaas ssh` is also available as
`kaas shell`.

## File transfer

Use `kaas cp` for one host-to-sandbox or sandbox-to-host file transfer. Prefix
the remote side with the sandbox name:

```bash
kaas cp my-app:~/workspace/payments/test-report.xml ./
kaas cp ./patch.diff my-app:~/workspace/payments/
```

KaaS uses the stored SSH host, port, user, and Colima identity for that
sandbox, then invokes `scp` with strict host-key checking (`accept-new`).
Exactly one path must be remote; copying between two sandboxes is not supported
by this command. Quote paths with spaces in the shell as usual.

## Commands in the guest (`kaas exec`, `kaas ps`)

`kaas exec` runs a shell command over SSH without opening an interactive
session. Detached commands (boat.dev's "background commands") keep running
after your shell closes; their output is written on the **guest** disk under
`~/.kaas-runs/<id>.log`, so it survives a stop/start, while the host registry
keeps an index row (`sandbox_commands`).

```bash
kaas exec my-app -- make test                              # synchronous; exit code propagates
kaas exec my-app --cwd ~/workspace/app --timeout 10m -- bash ./setup.sh
id=$(kaas exec my-app --detach --jsonl -- npm run build | jq -r 'select(.type=="command") | .id')
kaas exec my-app --status "$id"                            # running | exited N | killed | lost, plus log tail
kaas exec my-app --attach "$id"                            # stream the log until it ends; exits with its code
kaas exec my-app --kill "$id"                              # SIGTERM the process group, SIGKILL after 5 s
kaas exec my-app --history                                 # recorded commands with exit codes
kaas ps my-app                                             # busiest processes (pid, cpu, rss, user, elapsed)
```

- `--cwd` defaults to the primary workspace directory, else `$HOME`.
- Detached commands start in their own session (`setsid`) so `--kill` reaches
  every child. A VM stop ends them like a reboot; `kaas stop` marks running
  rows `lost`. Make long-lived servers systemd units instead (see
  [Local services](../guides/services.md#keep-a-service-running)).
- `--jsonl` emits one `command` record per row (`id`, `status`, `exit_code`,
  `pid`, `tail`), and `kaas ps --jsonl` a `processes` record; the UI polls these.

### Prompting an agent without a terminal

The installed harnesses accept a prompt in non-interactive mode, so a turn can
run detached and be read back later:

```bash
kaas exec my-app --cwd ~/workspace/app --detach -- pi -p 'Add tests for the billing module'
kaas exec my-app --attach <id>
```

The UI's **Prompt an agent** box does exactly this for `pi`, `omp`, and
`prime-agent`. Conversation memory is the harness's own session directory on
the guest disk — nothing is stored on the host — so it survives stop, start,
fork, and template deploys. Connect credentials first (`kaas secrets deploy` or
delegated egress); the sandbox page shows a **creds** chip with the fix command.

## Host dependencies

```bash
kaas deps                      # list required/optional tools for this OS
kaas deps --install            # install missing auto-installable tools
kaas deps --install --optional # also install optional tools (herdr)
kaas deps galaxy               # Ansible Galaxy collections from requirements.yml
```

On macOS, `--install` uses Homebrew. On Linux, it uses `apt-get` for the KVM/Ansible
baseline and downloads Lima/Colima/SecretSpec when those binaries are missing.
Interactive terminals prompt before installing unless `--install` or `--jsonl` is set.
`kaas up` still installs Galaxy collections automatically.

## Inspectability and diagnosis

```bash
kaas inspect my-app       # stored resources, SSH facts, repository, and status
kaas doctor               # host prerequisites: Colima, Lima, Ansible, SecretSpec
kaas doctor my-app        # also verify sandbox SSH and Ansible connectivity
kaas events my-app        # recent lifecycle actions, newest first
kaas events my-app --limit 20 --jsonl
```

`inspect` reconciles the observed lifecycle state with the provider when Colima
is available without changing the registry. It also reports persisted workspace
repository, branch, directory,
resolved commit, setup result, and configuration source (primary =
`workspace.sources[0]`). When the contract lists multiple sources, inspect
prints the full checkout list. `doctor` reports each check independently and
includes repair advice for missing prerequisites or failed sandbox
connectivity; inspect its table or JSONL `check` records. `events` reads
KaaS's persisted lifecycle audit log; it does not expose secret values.

`kaas setup` / the setup phase of `kaas up` clones or updates every
`workspace.sources[]` entry, then runs Varlock and `scripts/kaas-setup` in the
primary directory only.

## Delegated egress

```bash
kaas egress plan
kaas egress plan --jsonl
```

`plan` shows the versioned project policy—hosts, declared SecretSpec key names,
and injection type—without resolving a value. `configure` starts the
per-sandbox host proxy and installs only its public CA, proxy variables, and
non-secret placeholders. `reload` sends newly resolved values over its private
Unix socket. `status` is redacted. `verify` checks the nftables policy and
adversarial direct-IP/DNS/raw-socket paths.

`proxy_preferred` remains bypassable. `proxy_only` is enforced by in-guest
nftables for unprivileged workloads, but guest root/`CAP_NET_ADMIN` can remove
that policy; KaaS does not claim a guest-root boundary.

## Local services (`*.kaas.test`)

```bash
kaas services sync my-app
# Published URLs:
#   api       http://api.my-app.kaas.test
#   catalog   http://kaas.test/
kaas services list
kaas dns enable
# http://api.my-app.kaas.test  (catalog: http://kaas.test/)
```

Declare `workspace.services` in the project contract. KaaS allocates unique host
ports, maintains SSH local forwards, and routes by `Host` on the loopback
ingress proxy. Prefer `kaas.test` — never `.local`. `kaas up` and
`kaas services sync` print a **Published URLs** footer for the sandbox. See
[Local services](../guides/services.md).

## Local web UI

```bash
kaas ui                 # foreground dashboard on 127.0.0.1:18181
kaas ui start           # background
kaas ui status
kaas ui open
kaas ui stop
```

Loopback-only management console (HTMX + embedded templates). Lifecycle buttons
(start/stop/fork/destroy/setup/sync) and the **New sandbox** form each start one
`kaas <verb> … --jsonl` background job, recorded in the `jobs` table with a log
under `~/.kaas/ui/jobs/`. Host-owned `ui_runtime` in `~/.kaas/kaas.yaml`. See
[Local web UI](../guides/ui.md).

## Machine lifecycle

Machine lifecycle commands own the Colima profile and disk. Guest readiness
(`provision`, `agents`, `setup`, `check`) and credential commands are separate
stages; `kaas up` is the one-shot path that runs machine + guest + credentials.

| Command | Effect |
|---|---|
| `kaas create <name>` | Register identity/contract, allocate SSH port, create/start Colima VM |
| `kaas start [name]` | Resume a stopped VM; restore egress when configured; sync services; contract is sole authority |
| `kaas stop [name]` | Stop VM; preserve disk, registry, and contract; terminate host proxy; drop active service forwards |
| `kaas fork <id> <new>` | Clone guest disk under a new name/port; restore source to prior observed state |
| `kaas destroy [name]` | Delete VM; retain registry on provider failure unless `--force` |
| `kaas up <name>` | Galaxy deps → create/start → provision → agents → secrets/egress → checkout → setup |

Desired vs observed state (see [Architecture](./architecture.md#sandbox-lifecycle)):

- **Desired** — operator intent (`running`, `stopped`, …)
- **Observed** — last reconciled provider fact
- `kaas list --refresh` reports discrepancies; it does not silently rewrite rows

```bash
kaas list                     # NAME PORT DESIRED OBSERVED REPO REPO PATH
kaas list --refresh           # reconcile; report discrepancies
kaas list --repo kaas
kaas list --jsonl
kaas create my-app --port 6025 --cpus 6 --memory 12 --disk 80
kaas stop my-app              # desired=stopped; disk and registry preserved
kaas start my-app             # resume from existing disk
kaas destroy my-app           # confirm; --force drops registry if provider delete fails
```

Without `--port`, the next free port from `defaults.ssh_port_base` (default
`6022`) is chosen using the state DB and existing Colima profiles.

Fork an existing sandbox:

```bash
kaas fork my-app my-app-copy
kaas ssh my-app-copy
```

`fork` briefly stops the source because Lima cannot clone a running instance,
then clones the guest disk and configuration, allocates a new SSH port, and
registers the new sandbox. The source is restored to its exact prior observed
state. SecretSpec values, host egress runtime, and the auto-stop TTL are not
copied; run `kaas secrets deploy my-app-copy` (or `kaas egress configure`)
after setting any desired overrides.

## Templates (`--from`)

Lima has no live snapshots on the VZ driver, so kaas's equivalent of a cloud
"named snapshot" is a **stopped golden sandbox** marked as a template. Install
everything once, save it, and deploy copies without re-running provision or
agents:

```bash
kaas up golden --size large          # provision toolchains + agents once
kaas template save golden web-stack  # clone the disk; the copy stays stopped
kaas up feature-a --from web-stack   # clone + secrets/egress + checkout + setup
kaas up feature-b --from web-stack --repo git@github.com:acme/b.git
kaas template list
kaas template rm web-stack -y
```

- `template save` stops the source briefly (like `fork`), clones it, stops the
  clone, and tags it `role=template`. Saving the same name again replaces the
  template; sandboxes already deployed from it are unaffected.
- `--from` works on `up` and `create`. The new sandbox records
  `provision`/`agents` as **skipped** in its events; `up` still runs
  secrets/egress, checkout, setup, and service sync so each copy gets its own
  credentials and workspace.
- A template holds the **disk**; the contract and SecretSpec profile hold the
  **settings**. Nothing credential-bearing is copied from the template or from
  its source — put secrets in SecretSpec, never on a template disk.
- Templates are never auto-stopped and cannot be started directly (`kaas start`
  refuses; deploy from them instead). They appear in `kaas list`; the UI shows
  them on a separate **Templates** page.
- Clone time depends on the Lima driver and filesystem; measure
  `kaas template save` on your host before planning many parallel deploys.

## Auto-stop

Every sandbox can carry a TTL. The deadline is **now + TTL at each start**, not
from last activity; it is cleared on stop and re-armed on the next start.
Templates are exempt. The built-in default is `never` (today's behaviour); set
`defaults.auto_stop: 1h` in `~/.kaas/kaas.yaml` to make new sandboxes expire.

```bash
kaas up scratch --auto-stop 4h       # stops 4h after it comes up
kaas extend scratch --for 12h        # new deadline = now + 12h; TTL kept for later starts
kaas extend scratch --never          # disable
kaas start scratch --auto-stop 2d    # change the TTL on resume
```

`--for` and `--auto-stop` accept Go durations (`90m`, `4h`), whole days (`2d`),
or `never`; the maximum is 30 days. Enforcement runs in the UI daemon
(`kaas ui start`): every 30 s it starts a `kaas stop` job for overdue
sandboxes that have no job in flight. Without the daemon the deadline is
recorded but nothing stops the VM.

## Pre-stop hook

If the primary workspace directory contains an executable
`scripts/kaas-prestop`, `kaas stop` (including auto-stop) runs it over SSH
first — a chance to flush state that lives only in memory. Failure is recorded
as a `prestop` event and never blocks the stop.

## Contract drift

```bash
kaas contract show my-app     # persisted contract + field-level drift from the project kaas.yaml
kaas contract adopt my-app    # accept the project stack as the new contract
```

Resume commands keep using the persisted contract even when the project
`kaas.yaml` changes; `contract show` lists the differences and `contract adopt`
is the explicit reconcile. The UI shows the same drift on the sandbox page with
a **Re-adopt** button.

## `up` readiness phases

`kaas up <name>` keeps the guest and registry row when any phase fails, so the
failure can be diagnosed and repaired rather than recreated. `kaas events
<name>` records the completed, failed, or skipped phases:

```text
create → dependencies → provision → agents → secrets/egress → checkout → setup
```

Galaxy dependency installation is automatic. Empty optional SecretSpec values
produce a successful `secrets: skipped` event. Each Ansible/SSH phase also tees
its output to `<paths.logs_dir>/<sandbox>/<phase>.log` (default
`~/.kaas/logs/…`) so a failure can be read later from the UI or the shell.
After fixing a prerequisite, use
`kaas doctor <name>`, inspect the event trail, and rerun the relevant stage
command (`provision`, `agents`, `setup`, `secrets deploy`, …) or rerun
`kaas up <name>`.

## JSONL automation output

Pass the persistent `--jsonl` flag to any command when another program or agent
will consume its output. Set `output.jsonl: true` in `kaas.yaml` to make this
the default for every configured command; leave it `false` for text by default.
An explicitly supplied `--jsonl` flag takes precedence:

```bash
kaas list --jsonl
kaas --jsonl create my-app
```

In this mode stdout contains only newline-delimited JSON objects. Every object
has `schema_version: 1` and a `type`; a successful command ends with one
`result` record (`status: "ok"`). Messages produced by KaaS use `type:
"message"`, and `list` emits one `sandbox` record per sandbox before the
terminal result. Diagnostics and output from Ansible, Colima, SecretSpec, SSH, and other invoked tools are redirected to stderr so they
cannot corrupt the JSONL stream. Error exits retain status code `1` and emit an
`error` result when KaaS can parse the flag.

For interactive commands such as `ssh` and `secrets set`, JSONL is intended for
capturing machine-readable completion, not an interactive terminal session.

## Global flags

| Flag | Description |
|---|---|
| `--config` | Explicit `kaas.yaml` path |
| `--state` | Override the state DB path |
| `--verbose` | Verbose logging |
| `--quiet` | Suppress non-error output |
| `--jsonl` | Emit newline-delimited JSON records on stdout for automation |

Human-readable tables and status panels use color (lipgloss) when the terminal
supports it. Set `NO_COLOR=1` or `TERM=dumb` for plain text; `--jsonl` emits
structured events instead of styled output.

## Secrets subcommands

```bash
kaas secrets init
kaas secrets set ANTHROPIC_API_KEY                # profile default
kaas secrets set GITHUB_TOKEN --sandbox work      # per-sandbox override
kaas secrets check [--sandbox work]
kaas secrets deploy work [--scope github]
```

See [Secrets & SecretSpec](../guides/secrets.md) for details.
