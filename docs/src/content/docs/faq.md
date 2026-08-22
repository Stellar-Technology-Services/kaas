---
title: Frequently Asked Questions
description: Answers to common Kaas questions about configuration ownership, Ansible, projects, secrets, delegated egress, local services (*.kaas.test), and Colima escape hatches.
---

## How do I edit, add, or update Ansible at the user level?

For the conceptual overview (why Ansible is required, what it owns vs
`scripts/kaas-setup`, and extension patterns), read the
[Ansible guide](./guides/ansible/). This section is the short edit cookbook.

Kaas runs playbooks from a **user-owned** Ansible tree (not from each Git
project). After `kaas init`, that tree is normally:

```text
~/.kaas/
├── kaas.yaml
└── ansible/
    ├── ansible.cfg
    ├── setup.yml              # toolchains / packages (kaas provision)
    ├── coding-agents.yml      # agent CLIs (kaas agents)
    ├── deploy-secrets.yml
    ├── egress.yml
    ├── pubkey.yml
    ├── group_vars/
    │   └── all.yml            # versions, apt_packages, agent toggles
    └── requirements.yml
```

Confirm the active directory:

```bash
kaas config show
```

`kaas init` clones an Ansible git repository into `~/.kaas/ansible/` (default
`https://github.com/kaas-dev/ansible`) and **preserves an existing `ansible/`
tree**, so rerunning init does not wipe your edits.

### Edit variables (packages, versions, toggles)

```bash
$EDITOR ~/.kaas/ansible/group_vars/all.yml
```

Example changes:

```yaml
go_version: "1.26.5"
node_major: "22"

apt_packages:
  # … keep the baseline list …
  - redis-tools
  - postgresql-client

install_prime_agent: false   # skip one coding agent
install_herdr: true
```

Apply to a sandbox:

```bash
kaas provision my-app    # setup.yml + group_vars
kaas agents my-app       # coding-agents.yml toggles
```

### Add or change playbook tasks

Edit the playbook that owns the phase you care about:

```bash
$EDITOR ~/.kaas/ansible/setup.yml           # provision / up toolchain phase
$EDITOR ~/.kaas/ansible/coding-agents.yml   # agents phase
```

Example task appended to `setup.yml`:

```yaml
- name: Ensure bun is installed for the sandbox user
  become: false
  shell: |
    set -euo pipefail
    curl -fsSL https://bun.sh/install | bash
  args:
    creates: "{{ home_dir }}/.bun/bin/bun"
```

Then rerun:

```bash
kaas provision my-app
# or dry-run first:
kaas check my-app
```

Keep tasks idempotent. Do not weaken egress, CA, or secret-delivery behavior in
`egress.yml` / `deploy-secrets.yml` unless you intend to change the security
boundary.

### Update Galaxy collections

```bash
kaas deps galaxy          # refresh from ansible/requirements.yml
# or automatically on the next:
kaas up my-app
```

### Host tool dependencies

```bash
kaas deps                 # list required tools for this OS
kaas deps --install       # brew/apt (and Linux release installers) for missing tools
kaas deps --install --optional   # also install herdr
```

### Ansible directory and git

`kaas init` seeds `~/.kaas/ansible` by cloning a git repository (default
`https://github.com/kaas-dev/ansible`). You can accept the default at the
prompt, type another URL, or pass `--ansible-repo URL` for non-interactive
runs. KaaS already runs from that directory by default, so no config change is
required.

```bash
kaas init
# or:
kaas init --ansible-repo https://github.com/kaas-dev/ansible
```

Day to day: edit playbooks or `group_vars/all.yml`, commit in that directory,
then `kaas provision` / `kaas agents` / `kaas check` as usual.

Version only `~/.kaas/ansible` — not all of `~/.kaas`. Keep `state.db`,
SecretSpec values, and personal `kaas.yaml` local. `kaas init` preserves an
existing `ansible/` tree, so a git-managed checkout is safe across re-init.

Do not commit Ansible Galaxy collection installs unless you intend to vendor
them; refresh with `kaas deps galaxy` on each machine instead.

### Point at a different user Ansible tree

Optional alternate for unusual layouts — only in `~/.kaas/kaas.yaml` (rejected
in project config). Most people should use the cloned `~/.kaas/ansible` tree
instead (see above).

```yaml
paths:
  ansible_dir: /Users/me/src/kaas-ansible
```

Relative paths resolve from the global KaaS workspace (`~/.kaas`).

### Syncing with upstream canonical playbooks

Upstream Ansible lives at [kaas-dev/ansible](https://github.com/kaas-dev/ansible).
Your live copy is the clone at `~/.kaas/ansible/`. When you want new upstream
tasks:

1. In `~/.kaas/ansible`, fetch/merge from `origin` (or your fork’s upstream).
2. Resolve conflicts carefully — do not delete your local customizations
   blindly. Commit the merge.
3. Rerun `kaas provision` / `kaas agents` on each sandbox that should pick up
   the change.

Application-specific bootstrap (deps, Compose, migrations) still belongs in the
project’s `scripts/kaas-setup`, not in user Ansible.

## Can Ansible configuration be customized per project?

Not for KaaS-owned infrastructure playbooks. A project `kaas.yaml` may contain:

- VM resource overrides (`cpus`, `memory_gib`, and `disk_gib`);
- repository/workspace settings, including optional `workspace.services`;
- Varlock environment validation;
- the project’s value-free egress policy.

Application-owned settings such as `paths.ansible_dir` and
`ansible.playbooks.*` are intentionally rejected in a project configuration.

Project-specific setup belongs in the repository’s executable:

```text
scripts/kaas-setup
```

For example, that script can install application dependencies, start Compose
services, apply migrations, and create development fixtures. KaaS owns the
reusable Linux/toolchain/security baseline; the project owns its application
bootstrap.

## Why is the Ansible baseline not per project?

### Prevent provisioning drift

If every repository carries its own `setup.yml`, projects quickly develop
incompatible meanings of the standard toolchain, Ubuntu baseline, agent
versions, CA setup, and egress firewall behavior.

### Avoid duplicated infrastructure

A project should not copy and maintain KaaS’s `ansible.cfg`, inventory,
playbooks, group variables, and Galaxy requirements. Centralizing these assets
makes upgrades and security fixes consistent.

### Keep security behavior reviewable

The playbooks install sensitive infrastructure such as the public proxy CA,
nftables proxy-only policy, SSH settings, and guest secret delivery. A
project-local replacement could accidentally weaken those boundaries by opening
network access, exposing a CA key, or changing privilege behavior.

### Preserve stable CLI contracts

Commands such as `kaas provision`, `kaas agents`, `kaas secrets deploy`,
`kaas egress configure`, and `kaas check` depend on stable playbook names,
variables, and inventory behavior. A per-project playbook override would make
those commands project-specific and harder to diagnose.

## What belongs in a project instead?

Use the project contract and setup script:

```yaml
# kaas.yaml
workspace:
  git_auth: agent
  varlock:
    schema: .env.schema
  sources:
    - repository: git@github.com:acme/launchpad.git
      branch: main
      directory: ~/workspace/launchpad
  services:
    frontend:
      port: 3000
    api:
      port: 8080
```

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$HOME/workspace/launchpad"
pnpm install --frozen-lockfile
go mod download
docker compose up -d postgres redis
varlock run -- go run ./cmd/migrate
# Start app listeners on the guest ports declared under workspace.services
```

Keep the project setup idempotent and do not put secret values in `kaas.yaml` or
`scripts/kaas-setup`. See [Local services](./guides/services.md) for Host-based
`*.kaas.test` publishing.

## How do I use the `scripts/kaas-setup` shell script?

`scripts/kaas-setup` is the **project-owned** bootstrap contract. KaaS runs it
inside the guest after checkout (and optional Varlock validation). Put
application setup here — not in user Ansible.

### Create it

From a Git repository root:

```bash
kaas project init
```

That writes an executable starter at `./scripts/kaas-setup` next to `kaas.yaml`.
Commit both. Edit the script for your app:

```bash
#!/usr/bin/env bash
# Ran in the guest as the sandbox user, cwd = primary workspace directory
# (workspace.sources[0].directory) after clone/update.
set -euo pipefail

pnpm install --frozen-lockfile
docker compose up -d postgres redis
varlock run -- pnpm db:migrate
```

### When it runs

| Command | Behavior |
|---|---|
| `kaas up <name>` | After provision, agents, and credentials/egress — checkout all `workspace.sources`, then run this script in the **primary** directory |
| `kaas setup [name]` | Same checkout + script path only (no full reprovision) |

If the file is missing, KaaS skips the setup phase (recorded in `kaas events`).
The script must be executable (`chmod +x scripts/kaas-setup`).

### Working directory and multi-repo

The script always starts in `workspace.sources[0]` (primary). Additional sources
are already cloned beside it; `cd` to them if needed:

```bash
set -euo pipefail
ROOT="${HOME}/workspace"
cd "${ROOT}/payments"          # primary
pnpm install --frozen-lockfile

cd "${ROOT}/payments-api"      # another sources[] entry
go mod download
```

### Secrets and environment

- Do **not** put secret values in the script or in `kaas.yaml`.
- Use `kaas secrets deploy` (trusted guest injection) and/or delegated egress
  before setup when `kaas up` runs the full path.
- With Varlock, prefer `varlock run -- <command>` inside the script when a step
  needs the validated environment.

### Idempotency and failures

Make the script safe to rerun (`kaas setup my-app` after a fix). Prefer
`pnpm install`, Compose `up -d`, and migrations that no-op when already applied.

If setup fails, the VM and registry stay. Diagnose and retry:

```bash
kaas events my-app
kaas doctor my-app
kaas setup my-app
```

### What not to put here

- Host Ansible / toolchain baseline changes → `~/.kaas/ansible/`
- One-off interactive debugging → `kaas ssh <name>`
- Private keys or API tokens in plaintext

## Can one sandbox check out more than one repository?

Yes. List every managed checkout under `workspace.sources`. The first entry is
primary (`scripts/kaas-setup` and optional Varlock run there); the rest are
cloned or updated with the same `git_auth`:

```yaml
workspace:
  git_auth: agent
  sources:
    - repository: git@github.com:acme/launchpad.git
      directory: ~/workspace/launchpad
    - repository: git@github.com:acme/launchpad-api.git
      directory: ~/workspace/launchpad-api
```

`kaas list` and registry columns track the primary source; `kaas inspect`
prints the full list when more than one source is configured.

## What if a project needs a different base provisioning profile?

The supported customization point is a separate user-level KaaS workspace or a
custom global `paths.ansible_dir`. A project can also add application-level
setup in `scripts/kaas-setup`, but it must not weaken the KaaS security boundary.

A future project-specific infrastructure profile would need explicit contracts
for versioning, security review, lifecycle compatibility, and upgrade behavior;
it is not currently part of the project configuration model.


## Why do coding agents need a sandbox?

Coding agents modify more than source files. They install toolchains, change
shell configuration, start services, write caches, run package hooks, and may
handle credentials. Running that work directly on a Mac creates host pollution,
platform drift, credential sprawl, and collisions between projects.

KaaS gives each named workload its own Linux guest, disk, resources, SSH port,
process space, project checkout, and lifecycle — plus uniquely allocated host
ports when `workspace.services` is declared. The host remains the control plane
rather than becoming the agent's shared workspace.

## Why does KaaS use virtual machines instead of only containers?

Containers are excellent for packaging an application process. KaaS is solving
a workstation problem: an agent needs a complete, mutable Linux environment
with users, SSH, package managers, system services, filesystems, and sometimes
Docker Compose.

A Colima/Lima VM provides:

- a separate guest kernel and stronger host boundary than a shared-kernel
  container model;
- full Ubuntu semantics that resemble a Linux deployment environment;
- a durable disk that can be stopped, resumed, or forked;
- the ability to run containers inside the guest without using the host daemon
  as the agent's workspace;
- per-sandbox CPU, memory, disk, network, and SSH identity.

Containers can still be useful inside the KaaS guest. They are not a
replacement for the persistent VM boundary KaaS provides.

## Why does KaaS use Ansible?

See the [Ansible guide](./guides/ansible/) for the full rationale and extension
patterns. Short version:

A bootstrap shell script records what happened once. Ansible describes the
state a fresh or repaired guest should converge toward. This makes provisioning
idempotent, inspectable, rerunnable, and testable with `--check`.

KaaS keeps the reusable Linux and security baseline in Ansible so that toolchain
versions, agent installation, CA trust, SSH behavior, and egress enforcement do
not drift between projects. The project-specific `scripts/kaas-setup` script
then handles application dependencies, Compose services, migrations, and
fixtures.

## Why use SecretSpec instead of putting secrets in `kaas.yaml`?

`kaas.yaml` and `secretspec.toml` can be committed. Secret values should not be.
SecretSpec separates:

1. **Declaration** — which keys exist in the committed manifest;
2. **Storage** — provider-backed values, commonly an OS keyring;
3. **Selection** — shared `default` values combined with a sandbox override;
4. **Delivery** — intentional guest injection or delegated HTTP egress.

This prevents values from entering Git, project configuration, and ordinary
KaaS metadata. It does not automatically make a guest-injected value safe: a
value written to the guest can be read by guest code.

## What is the difference between guest-injected secrets and delegated egress?

Use **guest injection** with `kaas secrets deploy` when a trusted local process
must read the actual value or when the credential is not an HTTP credential—for
example, a database password, SSH key, signing key, mTLS key, or encryption key.
The value is readable and potentially exfiltratable by guest code.

Use **delegated egress** when an HTTP(S) client needs a bearer, basic, or named
header credential but the workload should not possess the value. The host proxy
holds the value and injects it only after matching an approved destination.

- `proxy_preferred` configures proxy-aware clients but permits direct bypass.
- `proxy_only` adds an nftables boundary for unprivileged guest workloads.
- A host rule without `inject_secret` grants reachability only; it receives no
  credential.
- Guest root or `CAP_NET_ADMIN` can remove in-guest `proxy_only` rules and is
  outside the claimed boundary.

Prefer a hybrid policy. Keep delegated HTTP keys and intentionally guest-readable
keys in separate SecretSpec scopes, and never deploy a delegated scope with
`kaas secrets deploy`.

## How do I publish guest HTTP apps without port fights?

Every vibecoding project wants `:8000` or `:8888`. KaaS solves that for
**sandbox guests** with a service registry, allocated host ports, SSH local
forwards, and optional Host-based URLs under `*.kaas.test`.

Declare listeners in the project contract:

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

Then:

```bash
kaas up my-app                 # syncs services after setup when declared
kaas services sync my-app      # allocate host ports, start ingress, SSH -L
kaas services list
kaas dns enable                # once: split-horizon resolver (sudo)
# http://frontend.my-app.kaas.test
# http://api.my-app.kaas.test
# catalog: http://kaas.test/
```

Important details:

- Prefer **`kaas.test`**. Do **not** use `*.kaas.local` — on macOS `.local` is
  reserved for Bonjour/mDNS and fights unicast DNS.
- Host ports allocate from `ingress_runtime.host_port_base` (default `19000`).
- The loopback ingress proxy (default `127.0.0.1:18000`) routes by `Host`
  header; DNS alone is not enough.
- Without DNS, use Host routing explicitly, for example
  `curl --resolve api.my-app.kaas.test:18000:127.0.0.1 http://api.my-app.kaas.test:18000/`.
- Publishing is **local-only** — no public HTTPS sharing.
- Services are part of the persisted sandbox contract; changing `kaas.yaml`
  after create may require adopt/reconcile before sync picks up edits.

See [Local services](./guides/services.md) and
[Configuration](./guides/configuration.md).

## Why use Herdr?

A sandbox gives agents a machine; Herdr gives the human operator a visible room
for coordinating them. Its terminal-native multiplexer supports parallel agent
roles—for example API, web, tests, and review—while keeping commands, output,
failures, and handoffs observable.

This is useful for long-running work because sessions remain inside the
persistent guest and can be revisited over SSH. Herdr does not replace KaaS
isolation; it operates inside the KaaS workspace.

## What is Varlock, and how do I use it?

[Varlock](https://varlock.dev/) is a **project-owned environment contract** tool.
It validates a checked-in schema (typically `.env.schema`) so required config —
URLs, ports, enums, feature flags, and sensitive values — is present and
well-typed **without** dumping secrets into logs or agent transcripts.

KaaS does **not** install Varlock or replace [SecretSpec](https://secretspec.dev/).
SecretSpec stores provider-backed keys for `kaas secrets` / egress. Varlock owns
the application’s full env shape inside the repo.

### Wire it into the project contract

In project `kaas.yaml` (primary workspace only):

```yaml
workspace:
  git_auth: agent
  varlock:
    schema: .env.schema
    # Recommended: validate with redacted output for agents / transcripts
    validate_command: varlock load --agent --summary-stderr
  sources:
    - repository: git@github.com:acme/launchpad.git
      directory: ~/workspace/launchpad
```

`schema` is required when Varlock is enabled. Paths are relative to
`workspace.sources[0].directory`.

### Make Varlock available in the guest

KaaS only runs your configured command; the CLI must exist on the sandbox PATH.
Install it in the primary repo (or once in `scripts/kaas-setup` before other
steps), for example via the project’s package manager or Varlock’s install
docs.

Commit `.env.schema` with the app. Keep real values out of git — use local
`.env` / SecretSpec / Varlock’s own loading rules as documented upstream.

### When KaaS runs validation

On `kaas up <name>` and `kaas setup [name]`, after checkout and **before**
`scripts/kaas-setup`:

1. `cd` to the primary workspace directory  
2. run `workspace.varlock.validate_command` (default above if you set `schema`)  
3. record an `environment` event: `ok`, `failed`, or `skipped`  
4. on failure: keep the VM, skip setup — fix env, then `kaas setup <name>`

```bash
kaas events my-app          # look for environment / setup
kaas setup my-app           # retry after fixing .env / schema
```

### Use Varlock inside `scripts/kaas-setup`

Validation only checks the contract. Commands that need the resolved
environment should use Varlock’s runner:

```bash
#!/usr/bin/env bash
set -euo pipefail

pnpm install --frozen-lockfile
docker compose up -d postgres redis
varlock run -- pnpm db:migrate
varlock run -- pnpm test
```

### Varlock vs SecretSpec (quick)

| | SecretSpec | Varlock |
|---|---|---|
| Owned by | KaaS / host keyring | Application repository |
| Declares | Named API keys for deploy/egress | Full env schema (`.env.schema`) |
| Values | Never in git; `kaas secrets set` | Project-local / Varlock loading |
| KaaS hook | `kaas secrets deploy` / egress | Optional pre-setup `validate_command` |

Use both when you want host-managed credentials **and** a validated app env
contract before bootstrap.

## What does a typical KaaS application workflow look like?

For a full-stack application, the usual flow is:

```bash
# One-time host setup
kaas init
kaas doctor

# Project contract
cd ~/src/launchpad
kaas project init
$EDITOR kaas.yaml scripts/kaas-setup

# Provider-backed credentials and policy
secretspec config global init
kaas secrets init
kaas secrets set GITHUB_TOKEN --sandbox launchpad
kaas egress plan

# One-shot path (machine + guest + credentials)
kaas up launchpad

# Inspect and diagnose
kaas list --refresh
kaas inspect launchpad
kaas events launchpad
kaas doctor launchpad

# Sandbox access
kaas ssh launchpad
kaas cp launchpad:~/workspace/launchpad/test-report.xml ./
kaas ping launchpad
kaas services list
kaas services open launchpad api
kaas dns status

# Guest readiness (rerun individual stages)
kaas provision launchpad
kaas setup launchpad
kaas check launchpad

# Machine lifecycle
kaas stop launchpad
kaas start launchpad
kaas fork launchpad launchpad-experiment
kaas destroy launchpad-experiment -y
```

`kaas --help` groups these by stage (host → machine → guest → credentials →
access → observe). For delegated egress, also use `kaas egress status`,
`kaas egress reload`, and `kaas egress verify`. For published HTTP apps, use
`kaas services sync`, `kaas services list`, and `kaas dns enable`. For agent
automation, add `--jsonl` to commands such as `kaas list`, `kaas inspect`, and
`kaas events`.

## What does `kaas up` actually do?

`kaas up <name>` is a recoverable sequence rather than an opaque installation.
It spans machine lifecycle and guest readiness:

1. install or refresh Ansible Galaxy dependencies (host setup);
2. create or start the named Colima profile and allocate an SSH port (machine);
3. provision the base Ubuntu toolchains (guest);
4. install coding agents (guest);
5. either deploy trusted guest secrets in `open` mode or configure delegated
   egress in delegated modes (credentials);
6. check out or update the configured repository;
7. validate the optional Varlock schema;
8. run `scripts/kaas-setup` (guest);
9. sync declared `workspace.services` (allocate host ports, SSH forwards,
   reload host ingress when services are present).

Each phase is recorded in `kaas events`. If a phase fails, KaaS keeps the VM and
registry entry so you can use `kaas doctor`, repair the relevant stage command
(`provision`, `agents`, `setup`, `secrets deploy`, `services sync`, …), and
rerun it instead of starting over. See
[Machine lifecycle](./reference/cli.md#machine-lifecycle).

## How do I inspect or automate KaaS safely?

Use human-readable commands during interactive work:

```bash
kaas list --refresh
kaas inspect launchpad
kaas events launchpad
```

Use JSONL when another program or agent consumes output:

```bash
kaas list --jsonl
kaas inspect launchpad --jsonl
kaas events launchpad --jsonl
```

JSONL keeps stdout structured and sends invoked-tool output to stderr. KaaS
redacts egress status and metadata and does not print delegated values.

## Escape hatch: managing stray Colima VMs

KaaS owns the registry at `~/.kaas/state.db` and drives Colima through that
lifecycle. Colima itself can still show profiles that KaaS does not know about
(manual `colima start`, a failed destroy, an old relative `state.path` that
wrote a DB in a project directory, or a VM created before the registry was
moved). Use Colima directly as the escape hatch for those strays.

### List provider VMs vs the KaaS registry

```bash
kaas list                 # registry rows KaaS manages
kaas list --refresh       # same, plus report drift vs Colima (no silent rewrite)
colima list               # every Colima profile on this machine
```

A profile in `colima list` that is missing from `kaas list` is a stray (or was
registered under a different state DB). A row in `kaas list` whose observed
state disagrees with Colima is drift — inspect it with `kaas inspect <name>`
and `kaas list --refresh`.

### Destroy through KaaS when the sandbox is registered

```bash
kaas destroy my-app -y              # delete Colima profile + registry row
kaas destroy my-app -y --force      # drop registry even if Colima delete fails
```

### Delete a stray Colima profile

When Colima has a profile KaaS does not track:

```bash
colima list
colima stop stray-name              # optional; delete can stop it
colima delete stray-name --force --data
```

`--data` removes the profile's disk and runtime data. This does **not** edit
`~/.kaas/state.db`. If a stale registry row remains afterward:

```bash
kaas destroy stray-name -y --force  # remove the registry row only path when provider is already gone
kaas list --refresh                 # confirm no remaining discrepancy
```

### Start / stop without going through KaaS

Useful for debugging a VM that still exists under Colima:

```bash
colima stop my-app
colima start my-app
colima ssh my-app
```

Prefer `kaas stop` / `kaas start` / `kaas ssh` for registered sandboxes so the
registry's desired/observed state stays coherent. After a raw Colima start or
stop, run `kaas list --refresh` (or `kaas inspect <name>`) before trusting the
registry columns.

### SSH and status without the CLI wrappers

```bash
colima ssh-config my-app            # HostName / Port / IdentityFile
colima status my-app
limactl list                        # underlying Lima instances (colima-<name>)
```

KaaS sandboxes are Colima profiles named exactly like the sandbox
(`kaas up kaas` → profile `kaas`, Lima instance `colima-kaas`).

## What is the practical security boundary?

KaaS isolates the host from ordinary guest changes and keeps delegated HTTP
credentials out of unprivileged guest processes. It does not provide general
data-loss prevention, guarantee that every protocol is mediated, or protect
against a guest root process that can change the VM's firewall and inspect its
own privileged environment.

Use narrow provider permissions, review egress destinations, separate trusted
and untrusted scopes, and treat `kaas secrets deploy` as a deliberate trust
choice.
