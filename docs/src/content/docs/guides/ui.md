---
title: Local web UI
description: Optional localhost dashboard for sandboxes, published services, and events.
---

KaaS ships an optional **loopback-only** web dashboard for operators who prefer a
browser over `kaas list` / `kaas services list`. It does **not** replace the
ingress catalog at `http://kaas.test/` — that remains the published-app index.
The UI is the management console (registry + host status + sync).

Default listen address: `127.0.0.1:18181` (avoids ingress `18000`, egress
`18080`, and service host ports `19000+`). Non-loopback binds are rejected.

## Quick start

```bash
kaas ui                 # foreground (Ctrl-C stops); same as kaas ui serve
kaas ui start           # background daemon (pid + log under ~/.kaas/ui)
kaas ui status
kaas ui open            # print URL; open in a browser when possible
kaas ui stop
```

Then open [http://127.0.0.1:18181/](http://127.0.0.1:18181/).

## What you see

| Page | Content |
|---|---|
| Overview | Sandbox counts, ingress/DNS status, host tool checks, recent sandboxes/services |
| Sandboxes | Desired/observed state, SSH port, resources, setup/egress |
| Sandbox detail | Workspace, contract hash, published URLs, recent events; **Sync services** |
| Services | All `*.kaas.test` rows (sandbox, URL, guest/host ports) |
| Events | Recent lifecycle events |

Tables on Overview, Sandboxes, and Services refresh every few seconds via HTMX.

## Host config (`~/.kaas/kaas.yaml`)

Never put `ui_runtime` in a project `kaas.yaml` — it is rejected like other
host-owned keys.

```yaml
ui_runtime:
  root: ~/.kaas/ui
  listen: 127.0.0.1:18181
```

| Key | Default | Purpose |
|---|---|---|
| `ui_runtime.root` | `~/.kaas/ui` | Pid/log directory for `kaas ui start` |
| `ui_runtime.listen` | `127.0.0.1:18181` | Loopback HTTP bind |

Override once with `--listen` on `serve` / `start`.

## Homebrew service (macOS)

The formula snippet in `packaging/homebrew/kaas.rb` includes a `service` block.
After the tap formula is synced:

```bash
brew services start kaas
brew services stop kaas
```

That runs `kaas ui serve --listen 127.0.0.1:18181` under launchd. You can still
use `kaas ui start` / `stop` without Homebrew.

## Security note (v1)

Loopback only: no TLS, no auth. Do not expose the port remotely. Destroy/fork
are intentionally omitted from the UI.
