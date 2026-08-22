---
title: Local services and *.kaas.test
description: Publish guest HTTP apps on the host with stable Host-based names.
---

Coding sandboxes often fight over `:8000` and `:8888`. KaaS solves that with a
**service registry**, allocated host ports, SSH local forwards, and an optional
Host-based ingress URL under `*.kaas.test`.

Do **not** use `*.kaas.local`. On macOS, `.local` is reserved for Bonjour/mDNS
and fights unicast DNS. Prefer the reserved `kaas.test` zone (RFC 2606).

## Declare services in the project contract

In the repository `kaas.yaml`:

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

Service names must be lowercase DNS labels. Guest ports must be unique within
the project. After create/`up`, the sandbox **contract** owns these values — edit
`kaas.yaml` and re-adopt/reconcile if you need the persisted contract to change.

## Sync and URLs

```bash
kaas up my-app                 # syncs services after setup when declared; prints Published URLs
kaas services sync my-app      # allocate host ports, start ingress, SSH -L; prints Published URLs
kaas services list
kaas services open my-app api
```

Resolved hostnames look like:

```text
http://frontend.my-app.kaas.test
http://api.my-app.kaas.test
```

`kaas up` and `kaas services sync` print a short **Published URLs** footer for that
sandbox (plus the catalog URL). On a color TTY this is a bordered panel; with
`NO_COLOR`, `TERM=dumb`, pipes, or `--jsonl` it falls back to a plain table or
structured events. Use `kaas services list` with no filter to see every sandbox
at once.
Each service gets a unique **host** port (default base `19000`) and an SSH local
forward `hostPort → guest:guestPort`. The host ingress proxy on
`127.0.0.1:18000` routes by `Host` header. The catalog lives at
`http://kaas.test/` (or `http://127.0.0.1:18000/` before DNS is enabled).

## Enable local DNS (once)

```bash
kaas dns enable    # sudo: macOS /etc/resolver/kaas.test or systemd-resolved drop-in
kaas dns status
kaas dns disable
```

This only redirects the `kaas.test` zone to the loopback DNS stub (default
`127.0.0.1:5353`). It does not rewrite global DNS. The ingress daemon must be
running (`kaas services sync`) so the stub answers.

Without DNS, you can still use Host routing:

```bash
curl --resolve api.my-app.kaas.test:18000:127.0.0.1 \
  http://api.my-app.kaas.test:18000/
```

## Host runtime (user config)

In `~/.kaas/kaas.yaml` (never in the project file):

```yaml
ingress_runtime:
  root: ~/.kaas/ingress
  listen: 127.0.0.1:18000
  dns_listen: 127.0.0.1:5353
  domain: kaas.test
  host_port_base: 19000
```

## Lifecycle notes

- `kaas start` / `kaas up` sync services for that sandbox.
- `kaas stop` drops active forwards for stopped sandboxes (registry rows remain).
- `kaas destroy` removes service rows (CASCADE) and reloads ingress.
- Publishing is **local-only** — no public HTTPS sharing.

See [Configuration](./configuration.md), [Local web UI](./ui.md),
[CLI](../reference/cli.md), [Architecture](../reference/architecture.md), and the
[FAQ on local services](../faq.md#how-do-i-publish-guest-http-apps-without-port-fights).
