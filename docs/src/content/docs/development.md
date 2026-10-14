---
title: Development
description: Build, test, vet, and publish the kaas CLI.
---

## Make targets

```bash
make            # build ./dist/kaas (default target)
make build      # go build -o dist/kaas ./cmd/kaas
make test       # go test ./...
make vet        # go vet ./...
make check      # vet + test
make install    # install dist/kaas to /usr/local/bin/kaas
make clean      # remove dist/
make run        # build + print kaas version
```

Or directly:

```bash
go test ./...            # run tests
go build -o dist/kaas ./cmd/kaas
```

## Layout

```
cmd/kaas/                   CLI entrypoint
internal/cli/               commands (Cobra GroupID help sections by stage)
internal/config/            kaas.yaml loading
internal/state/             SQLite registry
internal/colima/            Colima wrappers
internal/ansible/           playbook runner + inventory
internal/secretspec/        SecretSpec CLI client
internal/egress/            Delegated egress policy + host proxy
internal/ingress/           Host HTTP ingress, DNS stub, SSH forwards (`*.kaas.test`)
internal/ui/                Loopback dashboard: HTMX templates, readiness view-model, background job runner, auto-stop ticker
internal/sandbox/           port allocation
internal/sandbox/identity   sandbox identity types, name grammar, constructors, state mapping
internal/sandbox/lifecycle  create/start/stop/destroy/fork/reconcile; desired vs observed
internal/sandbox/contract   sandbox contract types, resolver, hash, SQLite snapshots
```

## Canonical workspace assets

`kaas.example.yaml` and `secretspec.toml` are embedded into the compiled CLI by
`assets.go`; `kaas init` writes those files into `~/.kaas`. Ansible playbooks
are not embedded — `kaas init` clones a git repository into `~/.kaas/ansible/`
(default `https://github.com/kaas-dev/ansible`). The in-repo `ansible/` tree
remains for local development and syncing with that upstream repository.

## Technology notes

- **Language**: Go (`go.mod` module `github.com/Stellar-Technology-Services/kaas`, Go 1.26.5).
- **CLI**: `spf13/cobra`.
- **Config**: `knadh/koanf/v2` (YAML + env + structs providers).
- **CLI output**: `github.com/charmbracelet/lipgloss` for terminal-friendly
  Published URLs / services tables (plain ASCII fallback for `NO_COLOR`,
  `TERM=dumb`, and non-TTY).
- **State DB**: `modernc.org/sqlite` (pure-Go, no cgo). Migrations are
  numbered SQL files under `internal/state/migrations/` and apply in order on
  open (`001` core, `002` services, `003` UI jobs, `004` templates/auto-stop,
  `005` guest commands).
- **Tests**: unit tests live beside each package (e.g. `internal/config/config_test.go`).
  UI tests drive the HTTP server against a fake `kaas` shell script so no VM is needed.


## Releases

Releases are built by GitHub Actions with [GoReleaser](https://goreleaser.com/)
when a `v*` tag is pushed. User-facing notes live in [`CHANGELOG.md`](https://github.com/Stellar-Technology-Services/kaas/blob/main/CHANGELOG.md).
The tag's leading `v` is removed from the package version: pushing `v0.1.0`
produces release artifacts with version `0.1.0` and sets `kaas version` to
`0.1.0`.

The release workflow cross-compiles with `CGO_ENABLED=0` and publishes Darwin
and Linux archives for `arm64` and `amd64`, plus Debian/Ubuntu `.deb` packages
for Linux `amd64` and `arm64`. Users do not need a compiler to install the
binaries.

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub creates a release containing four archives, two `.deb` packages, and
`checksums.txt`. On Linux, prefer the package:

```bash
sudo dpkg -i kaas_0.1.0_linux_amd64.deb   # or linux_arm64
```

On macOS (or for a manual Linux install), extract the archive and place `kaas`
on `PATH`:

```bash
tar -xzf kaas_0.1.0_darwin_arm64.tar.gz   # or linux_amd64, etc.
install -m 755 kaas "$HOME/.local/bin/kaas"
```

The release workflow is `.github/workflows/release.yml`; its build settings
are in `.goreleaser.yaml`. Locally you can smoke-test packages with
`goreleaser release --snapshot --clean` (look for `dist/kaas_*_linux_*.deb`).

## Documentation site

The Starlight docs under `docs/` publish to
[https://stellar-technology-services.github.io/kaas/](https://stellar-technology-services.github.io/kaas/) when a `v*`
tag is pushed (workflow: `.github/workflows/docs.yml`).

Enable **Settings → Pages → Source = GitHub Actions** once, and allow tag
pattern `v*` on the `github-pages` environment. Local preview:

```bash
cd docs
npm install
npm run dev
```

See the repository [`docs/README.md`](https://github.com/Stellar-Technology-Services/kaas/blob/main/docs/README.md) for build and deploy details.
