---
title: Overview
description: What kaasis, and why it exists.
---

`kaas` (Koding Agent Appkication Sandbox) is a CLI plus a set of Ansible playbooks for running disposable [Colima](https://colima.run) Ubuntu VMs. Each sandbox gets its own isolated Linux guest pre-loaded with toolchains, API credentials, and coding agents (Pi, Oh My Pi) — without contaminating the host Mac.

## What it does

- **Creates and manages Colima VMs** — `kaas up <name>` allocates a free SSH port, registers the sandbox in a SQLite state database, starts the VM, and provisions toolchains.
- **Installs toolchains and coding agents** — Go, Node, Rust, Python/uv, and the GitHub CLI, then the coding agent CLIs.
- **Manages secrets via [SecretSpec](https://secretspec.dev/)** — use explicit guest injection for trusted workspaces or keep selected HTTP credentials in a host-owned delegated-egress proxy.
- **Separates machine lifecycle from guest readiness** — `create` / `start` / `stop` / `fork` / `destroy` own the VM; `provision` / `agents` / `setup` / `check` own the guest; `up` is the one-shot path that runs both.
- **Tracks desired vs observed state** — the registry stores operator intent and last reconciled provider facts; `kaas list --refresh` reports discrepancies without silently rewriting rows.
- **Persists a secret-free sandbox contract** — resources, workspace, and egress policy are hashed and reused on resume so another directory’s `kaas.yaml` cannot silently change behavior.
- **Publishes local HTTP services** — declare `workspace.services`, get unique host ports and optional `*.kaas.test` Host-based URLs (never `.local`).
- **Makes readiness inspectable** — inspect desired/observed state and contract hash, diagnose with `kaas doctor`, and review lifecycle events.

## Commands by stage

`kaas --help` groups flat top-level commands by operator stage. Paths stay
`kaas <command>` (not nested under `vm` / `guest`).

| Stage | Commands | Purpose |
|---|---|---|
| Host setup | `init`, `project`, `config`, `deps`, `completion` | Workspace, config, Galaxy collections |
| Machine lifecycle | `up`, `create`, `start`, `stop`, `fork`, `destroy` | Colima profile and disk |
| Guest readiness | `provision`, `agents`, `setup`, `check` | Toolchains, agents, project bootstrap |
| Secrets and egress | `secrets`, `egress`, `pubkey` | Credentials and identity plumbing |
| Sandbox access | `ssh`, `cp`, `ping`, `services`, `dns`, `ingress` | Daily use, published HTTP services, local DNS |
| Inspect and diagnose | `list`, `inspect`, `doctor`, `events` | Registry, contract, connectivity, audit trail |

See the [CLI Reference](./reference/cli.md) for flags and examples, and
[Architecture](./reference/architecture.md#sandbox-lifecycle) for desired vs
observed state.

## Secret trust boundary

KaaS has two credential models:

- `kaas secrets deploy` writes the real value into the guest for trusted
  workspaces and tools that must possess it.
- Delegated egress keeps selected HTTP(S) credentials in a host proxy and
  injects them only for matching destination rules. The guest receives a
  non-secret placeholder, proxy settings, and the proxy's public CA.

Use `proxy_preferred` for compatibility testing; it can be bypassed by direct
network access. Use `proxy_only` for untrusted **unprivileged** guest workloads:
its nftables policy blocks direct egress except to the proxy. Guest root or
`CAP_NET_ADMIN` can remove in-guest rules and is outside the claimed boundary.

See [Delegated Egress](./guides/egress.md) for the design, configuration, and a
credential-selection guide.

## Why a sandbox at all?

Coding agents assume a full Linux toolchain, network access, and long-lived API credentials. Running them on a shared host shell fails in practice:

- **Host pollution.** Agents install packages, mutate `PATH`, write caches, and leave credentials in the user environment. One experiment contaminates every other project on the machine.
- **Non-reproducible Linux.** The host OS is often not the deployment target. Native builds, apt packages, and agent installers behave differently (or not at all) on Darwin, and even on Linux hosts you want isolation. Guests need a real Ubuntu userspace.
- **Multi-project concurrency.** Working on `project-a` and `project-b` at once needs separate disks, SSH ports, and often different API keys — not one shared VM or one `.env` file.
- **Secrets vs metadata.** API keys must never live in git or in a casual SQLite dump of *which VMs exist*. Declaration (which keys exist) and storage (keyring/provider) have to stay separate, with per-sandbox overrides.
- **Disposable by default.** Sandboxes should be cheap to create and safe to destroy. Lifecycle needs a single CLI, not a tribal Makefile ritual.

## Repo layout

```
cmd/kaas/                   CLI entrypoint
internal/cli/               commands (help groups by lifecycle stage)
internal/config/            kaas.yaml loading
internal/state/             SQLite registry
internal/colima/            Colima wrappers
internal/ansible/           playbook runner + inventory
internal/secretspec/        SecretSpec CLI client
internal/sandbox/           port allocation
internal/sandbox/identity   name grammar and identity types
internal/sandbox/contract   desired-behavior contract (secret-free)
internal/sandbox/lifecycle  provider create/start/stop/destroy/fork/reconcile
kaas.example.yaml           example project config
secretspec.toml             secret contract (values not in git)
ansible/                    Ansible playbooks, inventory, config, and defaults
```

## Next steps

- [Install on macOS](./guides/install-macos.md) or [Install on Linux](./guides/install-linux.md) — host setup.
- [Getting Started](./getting-started.md) — create your first sandbox.
- [Ansible](./guides/ansible.md) — why playbooks are required and how to extend them.
- [CLI Reference](./reference/cli.md) — every command and flag.
- [Secrets & SecretSpec](./guides/secrets.md) — choose guest injection or delegation.
- [Delegated Egress](./guides/egress.md) — why it exists, how the proxy works, and how to configure it.
- [Local services](./guides/services.md) — publish guest HTTP apps as `*.kaas.test`.
