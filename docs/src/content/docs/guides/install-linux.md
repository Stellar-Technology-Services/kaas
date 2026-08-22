---
title: Install on Linux
description: Install kaas and its host dependencies on Ubuntu with apt and upstream release binaries.
---

KaaS on Linux uses the same [Colima](https://colima.run) + Lima stack as macOS.
The host runs Colima VMs (typically via QEMU/KVM); guests remain Ubuntu sandboxes.

These steps target **Ubuntu** (apt). Adjust package names for other Debian-based
distros as needed. Colima, Lima, and SecretSpec are not in the Ubuntu archive,
so those install from upstream release binaries after apt packages. Install
`kaas` from the GitHub release `.deb` (or tarball).

## 0. Enable KVM and install apt packages

```bash
# CPU flags (look for vmx or svm)
grep -E 'vmx|svm' /proc/cpuinfo | head -1

sudo apt update
sudo apt install -y \
  qemu-kvm \
  qemu-system \
  cpu-checker \
  libvirt-clients \
  libvirt-daemon-system \
  ansible \
  curl \
  ca-certificates \
  jq

# Confirm KVM is usable
kvm-ok
ls -l /dev/kvm

# Add your user to the kvm group, then log out and back in
sudo usermod -aG kvm "$USER"
```

`kaas doctor` checks that `/dev/kvm` is present and readable.

## 1. Install Lima and Colima

Install [Lima](https://lima-vm.io/docs/installation/) and
[Colima](https://colima.run/docs/installation/) from their GitHub releases
(pick `x86_64` or `aarch64` / `arm64` to match `uname -m`):

```bash
# Lima
VERSION=$(curl -fsSL https://api.github.com/repos/lima-vm/lima/releases/latest | jq -r .tag_name)
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) LIMA_ARCH=x86_64 ;;
  aarch64|arm64) LIMA_ARCH=aarch64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
curl -fsSL "https://github.com/lima-vm/lima/releases/download/${VERSION}/lima-${VERSION#v}-Linux-${LIMA_ARCH}.tar.gz" \
  | sudo tar Cxz - /usr/local

# Colima
case "$ARCH" in
  x86_64|amd64) COLIMA_ARCH=x86_64 ;;
  aarch64|arm64) COLIMA_ARCH=arm64 ;;
esac
curl -fsSL -o /tmp/colima "https://github.com/abiosoft/colima/releases/latest/download/colima-Linux-${COLIMA_ARCH}"
sudo install /tmp/colima /usr/local/bin/colima
```

Confirm:

```bash
limactl --version
colima version
```

## 2. Install SecretSpec and Herdr

```bash
curl -sSL https://install.secretspec.dev | sh
curl -fsSL https://herdr.dev/install.sh | sh   # optional but recommended
```

Ensure `"$HOME/.local/bin"` is on your `PATH` if the installers place binaries there.

## 3. Install `kaas`

### Debian package (recommended)

Download the `.deb` for your CPU from the
[releases](https://github.com/Stellar-Technology-Services/kaas/releases) page
(`amd64` or `arm64` to match `uname -m`):

```bash
sudo dpkg -i kaas_0.1.0_linux_amd64.deb   # or linux_arm64
```

This installs `kaas` to `/usr/bin/kaas`.

After `kaas` is on `PATH`, converge remaining host tools (apt packages already
present are skipped; Lima/Colima/SecretSpec install when missing):

```bash
kaas deps                 # show status for this OS
kaas deps --install
kaas deps --install --optional   # also Herdr
```

### Tarball

Alternatively, download the Linux archive and install manually:

```bash
tar -xzf kaas_0.1.0_linux_amd64.tar.gz   # or linux_arm64
sudo install -m 755 kaas /usr/local/bin/kaas
```

### From source

```bash
sudo apt install -y golang-go   # or install Go 1.22+ from https://go.dev/dl/
git clone git@github.com:Stellar-Technology-Services/kaas.git
cd kaas
make build
sudo install -m 755 dist/kaas /usr/local/bin/kaas
```

## 4. Create the Colima SSH identity

Colima creates a shared Lima SSH key on first start:

```bash
colima start
colima stop
```

Confirm the identity exists:

```bash
ls -l ~/.colima/_lima/_config/user
```

## 5. Initialize kaas

```bash
kaas init
kaas doctor
```

`kaas init` writes `~/.kaas/` (config, Ansible assets, SecretSpec manifest, SQLite registry).
On Linux, `kaas doctor` should report `kvm`, Colima, Lima, Ansible, and SecretSpec as `ok`.

## Next steps

Continue with [Getting Started](../getting-started/) to scaffold a project contract,
create a sandbox with `kaas up`, and deploy secrets.
