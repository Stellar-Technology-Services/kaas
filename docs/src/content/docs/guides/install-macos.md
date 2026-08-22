---
title: Install on macOS
description: Install kaas and its host dependencies on macOS with Homebrew or a release binary.
---

KaaS on macOS uses [Colima](https://colima.run) + Lima to run Ubuntu guest VMs.
Install the CLI and host tools on the Mac; sandboxes always run Linux guests.

## 1. Install `kaas`

### Homebrew (recommended)

```bash
brew tap Stellar-Technology-Services/tools
brew install kaas
```

### GitHub release

Download the Darwin archive for your CPU from the
[releases](https://github.com/Stellar-Technology-Services/kaas/releases) page:

```bash
tar -xzf kaas_0.1.0_darwin_arm64.tar.gz   # or darwin_amd64
install -m 755 kaas "$HOME/.local/bin/kaas"
```

Ensure `"$HOME/.local/bin"` is on your `PATH`.

### From source

```bash
brew install go
git clone git@github.com:Stellar-Technology-Services/kaas.git
cd kaas
make build
install -m 755 dist/kaas "$HOME/.local/bin/kaas"
```

## 2. Install host dependencies

```bash
kaas deps                 # show what this Mac needs
kaas deps --install       # brew install ansible colima lima secretspec
# optional:
kaas deps --install --optional   # also herdr
```

Or install manually:

```bash
brew install ansible colima lima secretspec herdr
```

| Tool | Role |
|---|---|
| **Ansible 2.14+** | Guest provisioning |
| **Colima + Lima** | Ubuntu VM runtime |
| **SecretSpec** | Host secret provider client |
| **Herdr** | Terminal-native coding-agent multiplexer (optional but recommended) |

## 3. Create the Colima SSH identity

Colima creates a shared Lima SSH key on first start:

```bash
colima start
colima stop
```

Confirm the identity exists:

```bash
ls -l ~/.colima/_lima/_config/user
```

## 4. Initialize kaas

```bash
kaas init
kaas doctor
```

`kaas init` writes `~/.kaas/` (config, Ansible assets, SecretSpec manifest, SQLite registry).
`kaas doctor` should report `colima`, `limactl`, Ansible, and SecretSpec as `ok`.

### Optional: keep the local web UI running

After the Homebrew formula includes the in-repo `service` block
(`packaging/homebrew/kaas.rb`):

```bash
brew services start kaas   # kaas ui serve on 127.0.0.1:18181
```

Or without launchd: `kaas ui start`. See the [Local web UI](./ui/) guide.

## Next steps

Continue with [Getting Started](../getting-started/) to scaffold a project contract,
create a sandbox with `kaas up`, and deploy secrets.
