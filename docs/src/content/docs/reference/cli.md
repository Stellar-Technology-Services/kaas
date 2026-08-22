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
| `kaas up <name>` | Install dependencies, create VM, provision toolchains, agents, optional secrets, and workspace setup |
| `kaas create <name>` | Create / start Colima VM |
| `kaas start [name]` | Start a stopped sandbox with its existing disk and registry entry |
| `kaas stop [name]` | Stop the Colima VM while preserving its disk and registry entry |
| `kaas fork <sandbox-id> <new-name>` | Clone an existing sandbox VM under a new name |
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
| `kaas ui start` / `stop` / `status` / `open` | Background dashboard daemon |

### Inspect and diagnose

| Command | Description |
|---|---|
| `kaas list` | List all sandboxes (`ls`) |
| `kaas inspect [name]` | Show stored and current facts for a sandbox |
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

`kaas create` accepts VM sizing overrides. Values not supplied on the command
line come from `kaas.yaml` (or the built-in defaults):

```bash
kaas create my-app --port 6025 --cpus 6 --memory 12 --disk 80 --arch aarch64
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

Loopback-only management console (HTMX + embedded templates). Host-owned
`ui_runtime` in `~/.kaas/kaas.yaml`. See [Local web UI](../guides/ui.md).

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
state. SecretSpec values and host egress runtime are not copied; run
`kaas secrets deploy my-app-copy` (or `kaas egress configure`) after setting
any desired overrides.

## `up` readiness phases

`kaas up <name>` keeps the guest and registry row when any phase fails, so the
failure can be diagnosed and repaired rather than recreated. `kaas events
<name>` records the completed, failed, or skipped phases:

```text
create → dependencies → provision → agents → secrets/egress → checkout → setup
```

Galaxy dependency installation is automatic. Empty optional SecretSpec values
produce a successful `secrets: skipped` event. After fixing a prerequisite, use
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
