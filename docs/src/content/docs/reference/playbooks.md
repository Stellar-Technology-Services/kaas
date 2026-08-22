---
title: Playbooks (Ansible)
description: The Ansible playbooks that provision the guest and the CLI commands that invoke them.
---

`kaas` generates an inventory per run; the playbooks do the provisioning.
After `kaas init`, the live user copy is `~/.kaas/ansible/`, cloned from
`https://github.com/kaas-dev/ansible` by default (or another URL via the init
prompt / `--ansible-repo`). For **why Ansible is required and how to extend
it**, see the [Ansible guide](../guides/ansible/). Day-to-day edit recipes are
also in [Ansible directory and git](../faq.md#ansible-directory-and-git) in the
FAQ.

Playbook filenames are wired through `ansible.playbooks.*` in `kaas.yaml` (defaults in `internal/config`).

| Playbook | CLI command | Role |
|---|---|---|
| `ansible/setup.yml` | `kaas provision` / `kaas up` | Guest hostname (= sandbox name), toolchains + system packages |
| `ansible/coding-agents.yml` | `kaas agents` | Install coding agents |
| `ansible/deploy-secrets.yml` | `kaas secrets deploy` | Write API keys as shell exports |
| `ansible/pubkey.yml` | `kaas pubkey` | Sandbox SSH public key |
| `ansible/egress.yml` | `kaas egress configure` / delegated `kaas up` | Install public CA/proxy placeholders; optionally enforce nftables |

On create/start, Colima is also given `--hostname <sandbox>` so the guest
name matches the instance from first boot. `setup.yml` re-applies that
hostname idempotently during provision (important after `kaas fork`, which
clones a Lima disk that may still carry the source hostname).

## Secret safety

The playbooks correspond to two different trust decisions:

- `deploy-secrets.yml` receives resolved values and writes guest-readable shell
  exports. Use it only for explicitly trusted guest-injected scopes.
- `egress.yml` never receives a resolved credential. It receives the host proxy
  URL, the proxy's **public** CA file, placeholder key names, and the selected
  mode. For `proxy_only`, it resolves the guest-visible proxy address, pins that
  IP/port in nftables output/forward rules, verifies the drop policy, and enables
  nftables persistence.

The proxy's CA private key and delegated values stay in the host runtime. See
[Delegated Egress](../guides/egress.md) for the full flow.

## Inventory & hosts

The static inventory is `ansible/hosts.ini`; `ansible/ansible.cfg` selects it when commands run from that directory.

`kaas` writes an ephemeral inventory per run and targets the sandbox with `--limit`. The static `ansible/hosts.ini` documents the shape:

```ini
[local]
localhost ansible_connection=local

[colima_vms]
kaas ansible_host=127.0.0.1 ansible_port=6022

[colima_vms:vars]
ansible_ssh_private_key_file=~/.colima/_lima/_config/user
ansible_python_interpreter=/usr/bin/python3
ansible_ssh_common_args='-o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new'
```

## `ansible/ansible.cfg`

```ini
[defaults]
inventory = hosts.ini
host_key_checking = False
retry_files_enabled = False
stdout_callback = default
callback_result_format = yaml
interpreter_python = auto_silent
gathering = smart
fact_caching = memory

[ssh_connection]
pipelining = True
ssh_args = -o ControlMaster=auto -o ControlPersist=60s -o StrictHostKeyChecking=no
```

## Group variables

- **`ansible/group_vars/all.yml`** — VM settings, toolchain versions, apt packages, paths, and agent toggles.
- **`ansible/group_vars/colima_vms.yml`** — `ansible_user: "{{ lookup('env', 'USER') }}"` (Colima SSH user matches the host `$USER`).

Key paths from `ansible/group_vars/all.yml`:

```yaml
home_dir: "{{ ansible_user_dir | default(ansible_env.HOME | default('/home/' + (ansible_user | default(vm_user)), true)) }}"
bashrc_path: "{{ home_dir }}/.bashrc"
profile_snippet: "{{ home_dir }}/.profile.d/dev-toolchains.sh"
secrets_env_path: "{{ home_dir }}/.profile.d/agent-secrets.sh"
sandbox_ssh_key: "{{ home_dir }}/.ssh/id_ed25519"
```

## GitHub CLI repository

The setup playbook installs GitHub CLI from GitHub's apt repository. It writes a
standard `sources.list.d` entry with Ansible's built-in `copy` module instead of
using `deb822_repository`, so provisioning does not depend on Ubuntu providing
the optional `python3-debian` package.

## Collections

`ansible/requirements.yml` pulls the Ansible Galaxy collections needed by the playbooks:

```yaml
---
collections:
  - name: community.general
    version: ">=8.0.0"
```

`kaas up` installs them automatically; run `kaas deps galaxy` only to refresh them manually.

## Direct usage

Playbooks can be invoked directly with `ansible-playbook`, e.g. for the secrets path:

```bash
cd ansible
kaas secrets deploy my-app
```
