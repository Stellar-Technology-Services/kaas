---
title: Toolchains & Coding Agents
description: What gets installed on a sandbox — system packages, language toolchains, and coding agents.
---

Everything is driven by [`ansible/group_vars/all.yml`](https://github.com/Stellar-Technology-Services/kaas/blob/main/ansible/group_vars/all.yml). The `ansible/setup.yml` playbook installs system packages and toolchains; `ansible/coding-agents.yml` installs the coding agents.

## System packages

`ansible/setup.yml` installs (via `apt_packages` in `ansible/group_vars/all.yml`):

- **Build tools**: `build-essential`, `cmake`, `clang`, `make`, `pkg-config`, `protobuf-compiler`
- **Essentials**: `git`, `curl`, `wget`, `unzip`, `zip`, `gnupg`, `ca-certificates`, `software-properties-common`
- **Shell/UX**: `zsh`, `tmux`, `direnv`, `htop`, `ripgrep`, `fd-find`, `fzf`, `jq`
- **Language/deps**: `libssl-dev`, `lldb`, `python3`, `python3-dev`, `python3-pip`, `python3-venv`

## GitHub CLI

- **`gh`** from the official apt repository (installed by `ansible/setup.yml`).

## Language toolchains

| Toolchain | Detail |
|---|---|
| **Python** | `python3`, `pip`, `venv`, plus `pip`, `virtualenv`, `setuptools`, `wheel`, `uv` |
| **Node.js** | Node 22 LTS with `pnpm`, `yarn`, `typescript`, `tsx` |
| **Go** | `/usr/local/go` (`go_version: "1.26.5"`) |
| **Rust** | `rustup` stable with `rustfmt`, `clippy` |
| **Herdr** | terminal-native coding-agent multiplexer ([herdr.dev](https://herdr.dev)), installed by `ansible/setup.yml` |

`herdr` is installed during `kaas up` / `kaas provision` via its official installer (`curl -fsSL https://herdr.dev/install.sh | sh`) into `~/.local/bin`. Toggle with `install_herdr` in `ansible/group_vars/all.yml` (default `true`).

## Coding agents

`kaas agents <name>` installs Pi and Oh My Pi. Each is togglable via `install_*` vars in `ansible/group_vars/all.yml`:

| Agent | Toggle | Package |
|---|---|---|
| Pi | `install_pi` | `@earendil-works/pi-coding-agent` (npm) |
| Oh My Pi | `install_oh_my_pi` | `omp` |

```yaml
install_pi: true
install_oh_my_pi: true
pi_npm_package: "@earendil-works/pi-coding-agent"
```

Set any toggle to `false` to skip that agent. A separate `install_herdr` toggle (see above) controls the Herdr multiplexer installed by `ansible/setup.yml`.

## Paths

Because Colima/Lima homes are often `/home/<user>.guest` rather than `/home/<user>`, `home_dir` is resolved from Ansible facts (`ansible_user_dir`), with a fallback chain. Toolchain exports are written to `~/.profile.d/dev-toolchains.sh`; secrets to `~/.profile.d/agent-secrets.sh`.

## Secrets for the agents

Do not deploy every agent credential automatically. For HTTP(S) bearer/basic or
header credentials, prefer [Delegated Egress](./egress.md) when the agent or
project code should use a capability without reading the real value. Use
`kaas secrets deploy <name> --scope <trusted-scope>` only when a trusted agent
must possess the value or the protocol cannot be proxied. See the
[credential-model table](./secrets.md#choose-a-credential-model).


## Prime Agent

`kaas agents` installs [Prime Agent](https://github.com/PrimeIntellect-ai/prime-agent) using its official installer. KaaS sets npm's prefix to `~/.local`, so the installer writes the resulting `prime-agent` command to the guest user's `~/.local/bin` without requiring access to system npm directories. Set `install_prime_agent: false` in `ansible/group_vars/all.yml` to skip it.
