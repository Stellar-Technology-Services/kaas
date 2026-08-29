---
title: Getting Started
description: Initialize kaas, create a sandbox, add secrets, and log in after installing on macOS or Linux.
---

Install the CLI and host tools first:

- [Install on macOS](./guides/install-macos/)
- [Install on Linux](./guides/install-linux/)

Then continue below.

## 0. Initialize kaas (once per user)

With `kaas` on your `PATH`, bootstrap the per-user working directory:

```bash
kaas init
```

`kaas init` prompts for an Ansible git repository (default
`https://github.com/kaas-dev/ansible`) and clones it into `~/.kaas/ansible/`.
Pass `--ansible-repo URL` to skip the prompt (non-interactive runs use the
default when the flag is omitted). It also installs `~/.kaas/secretspec.toml`
and `~/.kaas/kaas.yaml` (configured to use that Ansible directory). The SQLite
registry lives at `~/.kaas/state.db`. Existing files and an existing
`ansible/` tree are preserved, so rerunning `kaas init` is safe. After
initialization, `kaas` uses this workspace from any current directory.

Confirm host prerequisites:

```bash
kaas doctor
```

## 1. Add KaaS to a Git project

:::caution[Important]
A project contract is optional. After `kaas init`, you can run sandboxes without
`kaas project init` — use a project `kaas.yaml` only when you need workspace
checkout, setup scripts, published services, or project egress/resource
overrides.
:::

From the project root, scaffold its committed workspace contract:

```bash
cd ~/src/payments
kaas project init
```

This creates:

```text
kaas.yaml
scripts/kaas-setup
```

Edit `kaas.yaml` with the project repository, resource requirements, and guest
workspace directory. Optionally declare `workspace.services` so HTTP apps
publish as `http://<service>.<sandbox>.kaas.test` (see
[Local services](./guides/services.md)). Put project bootstrap work in the
executable `./scripts/kaas-setup`; commit both files. KaaS discovers this
project config from any subdirectory without requiring an `ansible/` directory
in the project. The global `~/.kaas/ansible/` assets remain the single shared
provisioning baseline.

## 2. Create a sandbox

```bash
kaas up my-app                 # Galaxy deps + VM + toolchains + agents + credentials + setup
kaas agents my-app             # optional repeat; kaas up installs pi, omp, and Prime Agent
```

`kaas up` is the one-shot path across **machine lifecycle** and **guest
readiness**. Lower-level commands stay available (`kaas create`, then
`kaas provision`, `kaas agents`, `kaas setup`). `kaas --help` groups them by
stage; see [Commands by stage](./overview.md#commands-by-stage).

`kaas up` installs required Ansible Galaxy collections automatically. It also
continues when no optional SecretSpec values resolve; add credentials later
with `kaas secrets set` and deploy them with `kaas secrets deploy`. Likewise, a
scaffolded `kaas.yaml` whose `workspace.sources` entry has no repository yet is
valid: KaaS skips the checkout and setup phases instead of failing.

KaaS provisions the Ubuntu release supplied by Colima. To adopt a newer guest
baseline, create a fresh sandbox rather than performing an in-place OS upgrade.

## 3. Add secrets

```bash
secretspec config global init     # once — pick keyring
kaas secrets init
kaas secrets set ANTHROPIC_API_KEY
kaas secrets set GITHUB_TOKEN --sandbox my-app   # optional per-sandbox override
kaas secrets deploy my-app
```

> **Trust warning:** `kaas secrets deploy` writes values into the guest shell
> environment. Use it only for trusted code. VM isolation does not prevent
> guest code from reading or exfiltrating deployed secrets.

## 4. Use it

```bash
kaas pubkey my-app             # SSH public key for GitHub
kaas ssh my-app
kaas cp my-app:~/workspace/project/report.xml ./
kaas list                      # DESIRED / OBSERVED columns show lifecycle state
kaas list --refresh            # reconcile registry vs provider; report drift
```

## 5. Stop, resume, or destroy

Machine lifecycle commands preserve or remove the Colima disk independently of
guest provisioning:

```bash
kaas stop my-app                # stop VM; preserve disk, registry, and contract
kaas start my-app               # resume; contract is sole authority on resume
kaas destroy my-app             # prompts before permanent deletion
kaas destroy my-app -y          # skip confirmation
kaas destroy my-app -y --force  # drop registry even if provider delete fails
```

## Multiple sandboxes

Multiple sandboxes get unique SSH ports automatically:

```bash
kaas up project-a
kaas up project-b
kaas list
kaas ssh project-a
```

To duplicate an already-provisioned sandbox, clone its guest disk and allocate
a new SSH port:

```bash
kaas fork my-app my-app-copy
kaas ssh my-app-copy
```

The fork does not copy SecretSpec values; deploy the desired credentials to the
new sandbox explicitly.

For private repository checkout in a workspace setup, the default
`workspace.git_auth: agent` forwards a key already loaded in the host SSH agent
for KaaS's clone/update only. See [Configuration](./guides/configuration.md#git-authentication-for-private-checkouts)
for `guest_key`, `https`, and `none` modes.

Read on to learn about [Secrets & SecretSpec](./guides/secrets.md) or the full [CLI Reference](./reference/cli.md).


## Optional delegated egress

Use delegated egress for HTTP(S) credentials that guest code should be able to
use at reviewed destinations without reading the real value.

1. Add the project policy:

   ```yaml
   egress:
     schema_version: 1
     mode: proxy_only
     default: deny
     rules:
       - host: api.github.com
         inject_secret:
           key: GITHUB_TOKEN
           type: bearer
   ```

2. Store the value and inspect the value-free plan:

   ```bash
   kaas secrets set GITHUB_TOKEN --sandbox my-app
   kaas egress plan
   ```

3. Bring up the sandbox and inspect the proxy:

   ```bash
   kaas up my-app
   kaas egress status my-app
   kaas egress verify my-app       # proxy_only
   ```

4. Rotate a value without deploying it into the guest:

   ```bash
   kaas egress reload my-app
   ```

`proxy_preferred` is a compatibility mode and remains bypassable.
`proxy_only` covers unprivileged guest code; guest root/`CAP_NET_ADMIN` can
remove in-guest firewall rules.

Before choosing a mode, read [Delegated Egress](./guides/egress.md), especially
[Choose per secret](./guides/egress.md#choose-per-secret). The detailed policy
schema is in [Configuration](./guides/configuration.md#project-egress-policy),
and the control flow is in [Architecture](./reference/architecture.md#how-delegated-egress-works).
