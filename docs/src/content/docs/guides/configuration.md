---
title: Configuration
description: kaas.yaml project config, precedence, environment variables, and defaults.
---

`kaas` is configured from a project `kaas.yaml` (repo root), a user config, environment variables, and command flags.

## Precedence

```
flags  >  KAAS_* env  >  project kaas.yaml  >  ~/.kaas/kaas.yaml (workspace)  >  built-in defaults
```

- **Workspace (user defaults)**: `~/.kaas/kaas.yaml`, created by `kaas init`; its Ansible files live in `~/.kaas/ansible/`.
- **Project override**: a `kaas.yaml` found in or above the current directory may contain only VM resource overrides, `workspace`, and a versioned `egress` policy; other application settings are rejected with a clear error. Run `kaas project init` at a Git repository root to scaffold it.
- **Env**: any key via `KAAS_` prefix, e.g. `KAAS_DEFAULTS_CPUS=6`.
- **Flags**: `--config <path>`, `--state <path>`, `--verbose`, `--quiet`.

## Persisted sandbox contracts

`kaas create` / `kaas up` resolve the precedence stack into a redacted
**sandbox contract** (resources, workspace, secret mode/key refs, egress policy)
and store it in SQLite (`sandbox_contracts`) with a stable hash.

After a contract exists, resume commands (`start`, `setup`, `secrets deploy`,
`egress *`, and `up` on an existing sandbox) treat that contract as sole
authority. Running them from another directory does not silently rewrite
workspace or egress behavior.

Legacy sandboxes with no contract row are **adopted once** on first resume:
KaaS resolves from the current stack, persists the snapshot, prints an
adoption summary, then applies strict authority afterward.

## Global application configuration (`~/.kaas/kaas.yaml`)

```yaml
# Make JSONL the default for commands; explicit --jsonl still takes precedence.
output:
  jsonl: false

defaults:
  cpus: 4
  memory_gib: 8
  disk_gib: 64
  arch: aarch64
  ssh_port_base: 6022

paths:
  ansible_dir: ansible
  ssh_key: ~/.colima/_lima/_config/user

state:
  path: ~/.kaas/state.db

ansible:
  playbooks:
    setup: setup.yml
    agents: coding-agents.yml
    secrets: deploy-secrets.yml
    pubkey: pubkey.yml
```

## VM resource defaults and project overrides

Set user-wide VM defaults in `~/.kaas/kaas.yaml` (created by `kaas init`):

```yaml
# ~/.kaas/kaas.yaml
# Used for every new sandbox unless a project or command overrides it.
defaults:
  cpus: 4
  memory_gib: 8
  disk_gib: 64
```

Create the contract at a Git repository root with:

```bash
kaas project init
```

It writes a project `kaas.yaml` and an executable `scripts/kaas-setup` starter.
A repository `kaas.yaml` can override only CPU, memory, disk, and workspace
settings. It is loaded on top of the user configuration when KaaS runs from that
project. KaaS rejects `state`, `paths`, `ansible`, `output`, and other global
application settings in a project file:

```yaml
# <project>/kaas.yaml
defaults:
  cpus: 8
  memory_gib: 16
  disk_gib: 120
```

Finally, `kaas create` flags override both for that one new sandbox:

```bash
kaas create benchmark --cpus 12 --memory 32 --disk 200
```

These values apply when a sandbox is first created. Existing sandboxes retain
their recorded CPU, memory, and disk settings when stopped and started; create
a new sandbox to use changed defaults.

## Project workspace setup

KaaS provisions its reusable Ubuntu/toolchain baseline first, installs agents,
and deploys SecretSpec values. A project can then declare its own checkout and
bootstrap contract without changing KaaS-owned Ansible playbooks:

```yaml
workspace:
  git_auth: agent                              # default; forward host SSH agent for Git
  sources:
    - repository: git@github.com:acme/payments.git
      branch: main                             # optional
      directory: ~/workspace/payments          # primary = sources[0]
  # KaaS clones/updates every sources[] entry, then runs ./scripts/kaas-setup
  # from sources[0] after optional Varlock validation.
```

### Multiple sources in one guest

Declare every managed checkout under `workspace.sources`. The first entry is
**primary**: Varlock and `scripts/kaas-setup` run only there. Additional
entries are cloned or updated on `kaas up` / `kaas setup` with the same
`workspace.git_auth`.

```yaml
workspace:
  git_auth: agent
  sources:
    - repository: git@github.com:acme/payments.git
      branch: main
      directory: ~/workspace/payments
    - repository: git@github.com:acme/payments-api.git
      branch: main
      directory: ~/workspace/payments-api
    - repository: git@github.com:acme/payments-ui.git
      branch: main
      directory: ~/workspace/payments-ui
```

Directories must be unique. `kaas list` / registry columns and the primary
inspect fields refer to `sources[0]`; `kaas inspect` prints the full list when
more than one source is configured. Every managed checkout is declared under
`workspace.sources`; each entry requires a `repository` and a `directory`.

### Local HTTP services (`*.kaas.test`)

Declare guest listeners under `workspace.services`. KaaS allocates unique host
ports, maintains SSH local forwards, and publishes Host-based URLs such as
`http://api.my-app.kaas.test`. Prefer `kaas.test` — never `.local` (mDNS).

```yaml
workspace:
  services:
    frontend:
      port: 3000
      scheme: http
    api:
      port: 8080
      scheme: http
```

Host-owned listen addresses live under `ingress_runtime` in `~/.kaas/kaas.yaml`.
See [Local services](./services.md). The optional management dashboard uses
`ui_runtime` (also host-owned only) — see [Local web UI](./ui.md).

### Git authentication for private checkouts

`workspace.git_auth` controls the repository clone/update performed by KaaS for
every `sources[]` entry:

| Value | Behavior |
|---|---|
| `agent` (default) | Forwards the host SSH agent to the guest for checkout. Start `ssh-agent` and add a key on the host before `kaas up`. |
| `guest_key` | Uses the guest SSH key generated by `kaas pubkey`; authorize its public key with the Git provider before setup. |
| `https` | Requires an `https://` repository URL; Git uses credentials already available in the guest. |
| `none` | Does not request SSH-agent forwarding; suitable for public repositories or project-managed authentication. |

The forwarded agent is used for the KaaS-managed Git clone or update only;
KaaS does not copy the host private key into the guest. Repository host-key
verification remains SSH's normal `accept-new` behavior.

#### `agent` — recommended for private SSH repositories

```yaml
workspace:
  git_auth: agent
  sources:
    - repository: git@github.com:acme/payments.git
      directory: ~/workspace/payments
```

Load an authorized key in the host SSH agent before running `kaas up` or `kaas
setup`:

```bash
ssh-add -l                         # verify a key is loaded
ssh-add ~/.ssh/id_ed25519          # load one when necessary
```

This is the default mode. It gives the checkout temporary access to the host
agent without placing the private key on the guest disk. It does **not** make
the host agent available to arbitrary later SSH sessions; configure a guest key
or your project tooling if those sessions need Git access too.

#### `guest_key` — persistent guest Git identity

```yaml
workspace:
  git_auth: guest_key
  sources:
    - repository: git@github.com:acme/payments.git
      directory: ~/workspace/payments
```

Generate and authorize a guest-owned key before the first private checkout:

```bash
kaas pubkey my-app
# Add the printed public key as a GitHub account key or repository deploy key.
kaas setup my-app
```

The key stays on the sandbox disk at `~/.ssh/id_ed25519`. Use this mode only
when that persistent per-sandbox identity is desired.

#### `https` — HTTPS checkout

```yaml
workspace:
  git_auth: https
  sources:
    - repository: https://github.com/acme/payments.git
      directory: ~/workspace/payments
```

KaaS validates that every `sources[].repository` URL starts with `https://`.
Git uses credentials already configured inside the guest; KaaS does not supply
a token. Use SecretSpec or project-owned setup to make those credentials
available without committing them to `kaas.yaml`.

#### `none` — public or project-managed authentication

```yaml
workspace:
  git_auth: none
  sources:
    - repository: https://github.com/acme/public-project.git
      directory: ~/workspace/payments
```

No host-agent forwarding is requested. This is appropriate for public
repositories or when the project setup contract manages its own Git
credentials.

### Optional Varlock environment contract

A project that uses [Varlock](https://varlock.dev/) can declare its checked-in
`.env.schema` as an agent-safe environment contract:

```yaml
workspace:
  varlock:
    schema: .env.schema
    validate_command: varlock load --agent --summary-stderr
  sources:
    - repository: git@github.com:acme/payments.git
      directory: ~/workspace/payments
```

After checkout and before project setup, KaaS runs the validation command from
the **primary** workspace directory (`sources[0]`). The recommended Varlock
command validates required configuration while redacting `@sensitive` values
for Ansible output and agent transcripts. It records an `environment`
lifecycle event as `ok`, `failed`, or `skipped`; a failed validation preserves
the sandbox and stops setup.

KaaS does not install Varlock or resolve its values. Add Varlock to the project
or install its CLI through the project setup, and keep SecretSpec responsible
for KaaS-managed credential deployment. Use Varlock's `varlock run -- <command>`
inside the repository setup script when that script needs its validated,
injected environment.

`kaas up <name>` and `kaas setup [name]` run the repository's standard
`./scripts/kaas-setup` executable after checkout and optional environment
validation. KaaS skips setup if the file is absent; the canonical KaaS
repository includes an executable, commented starter script. Keep it
idempotent and do not put secret values in it.

```bash
#!/usr/bin/env bash
set -euo pipefail
# Install dependencies, start local services, apply migrations, or create fixtures.
# varlock run -- pnpm db:migrate
```


## Project egress policy

A project can declare its intended HTTP(S) destinations separately from
SecretSpec declarations. Policy rules grant reachability independently from
credential injection: a rule without `inject_secret` never receives a secret.
The policy contains SecretSpec **key names**, never values.

```yaml
# <project>/kaas.yaml
egress:
  schema_version: 1
  mode: proxy_preferred # open, proxy_preferred, or proxy_only
  default: deny         # proxy_only requires deny
  rules:
    - host: github.com
    - host: api.github.com
      inject_secret:
        key: GITHUB_TOKEN
        type: bearer    # bearer, basic, or header (with header: X-Name)
```

Hosts are lowercase DNS names; `*.example.com` matches subdomains only. Use
`kaas egress plan` to review the redacted effective policy. **Runtime behavior:** `kaas up` automatically starts the per-sandbox host
proxy for delegated modes and configures the guest through `ansible/egress.yml`.
`proxy_preferred` installs proxy variables and the public CA but remains
bypassable. `proxy_only` additionally pins the resolved proxy IP and port in an
nftables output/forward policy for unprivileged guest workloads. Guest root or
`CAP_NET_ADMIN` can remove in-guest rules, so an external guest-root boundary
is not claimed. See [Delegated Egress](./egress.md).

## Keys

| Key | Default | Purpose |
|---|---|---|
| `output.jsonl` | `false` | Make JSONL the default stdout protocol for commands; an explicit `--jsonl` flag takes precedence |
| `defaults.cpus` | `4` | VM CPU count |
| `defaults.memory_gib` | `8` | VM memory (GiB) |
| `defaults.disk_gib` | `64` | VM disk (GiB) |
| `defaults.arch` | `aarch64` | VM architecture |
| `defaults.ssh_port_base` | `6022` | First SSH port allocated for sandboxes |
| `paths.ansible_dir` | `ansible` | Directory containing `ansible.cfg` and playbooks (default resolves to `~/.kaas/ansible`, cloned by `kaas init` from git — see the [FAQ](../faq.md#ansible-directory-and-git)) |
| `paths.ssh_key` | `~/.colima/_lima/_config/user` | Colima SSH identity |
| `state.path` | `~/.kaas/state.db` | SQLite registry. Relative paths resolve under `~/.kaas`, not the caller's CWD. |
| `workspace.sources` | empty | Managed guest checkouts (`repository`, optional `branch`, `directory`); primary is `sources[0]` |
| `workspace.git_auth` | `agent` | Checkout authentication for every source: `agent`, `guest_key`, `https`, or `none` |
| `workspace.varlock.schema` | empty | Optional checked-in Varlock schema, relative to primary (`sources[0]`) directory |
| `workspace.varlock.validate_command` | `varlock load --agent --summary-stderr` | Optional safe Varlock validation command; requires `schema` |
| `workspace.services` | empty | Named guest listeners (`port`, optional `scheme`) published as `service.sandbox.kaas.test` |
| `egress` | empty | Project-owned versioned HTTP(S) destination/credential policy |
| `egress_runtime.root` | `~/.kaas/egress` | Host-private per-sandbox proxy/CA/control state |
| `egress_runtime.listen_host` | `0.0.0.0` | Host proxy bind address |
| `egress_runtime.guest_host` | `host.lima.internal` | Guest-visible host proxy name, resolved and pinned during configuration |
| `egress_runtime.port_base` | `18080` | First deterministic per-sandbox proxy port |
| `ingress_runtime.root` | `~/.kaas/ingress` | Host ingress/DNS/control state |
| `ingress_runtime.listen` | `127.0.0.1:18000` | Loopback HTTP reverse proxy (Host routing + catalog) |
| `ingress_runtime.dns_listen` | `127.0.0.1:5353` | Loopback UDP DNS stub for `*.domain` |
| `ingress_runtime.domain` | `kaas.test` | Local zone (must not use `.local`) |
| `ingress_runtime.host_port_base` | `19000` | First allocated host port for SSH `-L` forwards |
| `ui_runtime.root` | `~/.kaas/ui` | Pid/log directory for the optional local web UI daemon |
| `ui_runtime.listen` | `127.0.0.1:18181` | Loopback HTTP bind for `kaas ui` (must be loopback) |
| `ansible.playbooks.*` | (see example) | Filenames for each playbook action |

## Global flags

| Flag | Description |
|---|---|
| `--config` | Explicit `kaas.yaml` path |
| `--state` | Override the state DB path |
| `--verbose` | Verbose logging |
| `--quiet` | Suppress non-error output |

## Environment variables

| Variable | Maps to |
|---|---|
| `KAAS_DEFAULTS_CPUS` | `defaults.cpus` |


Environment-variable caveat: the loader converts every underscore after
`KAAS_` into a nesting separator. Therefore names containing underscores in a
configuration key (for example `memory_gib` and `ssh_port_base`) currently do not map cleanly from environment variables;
prefer project/user YAML or command flags for those settings. `KAAS_STATE_PATH` is unambiguous.
