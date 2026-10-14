---
title: Local web UI
description: Optional localhost dashboard that drives the kaas CLI — create, start, stop, fork, destroy, and watch jobs.
---

KaaS ships an optional **loopback-only** web dashboard for operators who prefer a
browser over the terminal. It does **not** replace the ingress catalog at
`http://kaas.test/` — that remains the published-app index. The UI is the
management console: registry + host status + lifecycle actions.

The UI never re-implements lifecycle logic. Every button starts one
`kaas <verb> … --jsonl` process in the background (a **job**), records it in
SQLite, and polls it with HTMX. What you can do in the browser is exactly what
you can do at the shell, with the same contract, event trail, and safety rules.

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
| Overview | Sandbox/running/service counts, ingress/DNS status, **jobs running**, **host capacity meter** (vCPU and memory promised to running sandboxes vs. what the host has), host tool checks, recent sandboxes/services |
| Sandboxes | One row per working sandbox with a **Readiness** badge, desired/observed state, **auto-stop countdown**, SSH port, resources, egress |
| Templates | Stopped golden sandboxes saved with **Save as template…**; deploy from one or remove it |
| New sandbox | Form that runs `kaas up` with a suggested name, **size preset**, **trust preset**, **start from** (fresh VM or a template), **auto-stop**, and optional workspace |
| Sandbox detail | Readiness + **creds** chip + lifecycle buttons, auto-stop card with **Extend**, connect panel (copyable commands), **Environment** card (redacted contract + drift + **Re-adopt**), **Phase logs**, active job with live log tail, published URLs with **health** and systemd snippets, **Run in the guest** (detached command box, agent prompt box, command list with kill, processes), recent jobs, recent events |
| Services | All `*.kaas.test` rows (sandbox, URL, guest/host ports) |
| Events | Recent lifecycle events |
| Jobs | Every background CLI job started from the UI, with status and log link |

Tables on Overview, Sandboxes, and Services refresh every few seconds via HTMX.
While a job runs, the sandbox header and job card poll every 2–3 s.

## Readiness badge

The badge collapses registry facts into one word, the way a cloud sandbox
dashboard shows `starting → ready`. It never calls the provider; it is derived
from `desired`/`observed` state, `setup_status`, recent events, and the active job:

| Badge | Meaning |
|---|---|
| **Ready** | Observed running and `scripts/kaas-setup` finished (or every `up` phase reported ok/skipped) |
| **Starting** `phase N/6` | Observed running but `up` phases are still completing (create → provision → agents → secrets/egress → checkout → setup) |
| **Working** | A background job (`stop`, `fork`, `setup`, …) is running for this sandbox |
| **Running** | VM is up but the guest was never provisioned |
| **Stopped** | Disk, registry, and contract preserved; **Start** resumes it |
| **Setup failed** | `scripts/kaas-setup` failed; fix the cause and **Re-run setup** |
| **Failed** / **Absent** / **Unknown** | Provider trouble; see events, run `kaas list --refresh` |

## Lifecycle actions

Buttons appear only when the action is valid for the current state; transitional
states and an active job disable all of them (the server also returns `409` if a
stale page submits one).

| State | Buttons |
|---|---|
| Running | Stop, Extend, Re-run setup, Sync services, Fork…, Save as template…, Destroy… |
| Stopped | Start, Fork…, Save as template…, Destroy… |
| Failed / unknown | Start, Destroy… |
| Template | Deploy from this (on the Templates page), Remove… |

- **Fork…** asks for a new name and states the kaas model plainly: the source is
  **stopped briefly** for the clone (Lima cannot clone a running VM) and then
  restored; SecretSpec values, the host egress runtime, and the auto-stop TTL
  are **not** copied.
- **Save as template…** runs `kaas template save <sandbox> <template>`: the
  copy stays stopped and appears on the Templates page; the create form then
  offers it under **Start from**.
- **Destroy…** requires typing the sandbox name (the browser equivalent of
  `-y`); an optional **force** checkbox maps to `--force`. On a template it runs
  `kaas template rm`.
- **Extend** sets the auto-stop deadline to *now + duration* (`kaas extend
  --for`); it does not add to the remaining time. **never** disables it.
- **Re-adopt from project** appears when the Environment card detects drift;
  it runs `kaas contract adopt`.
- The job is attributed to the sandbox, so a double-click cannot start two
  operations. The CLI's own `sandbox_operations` claims remain the authority.

## Auto-stop

The UI daemon is the auto-stop enforcer. Every 30 s it compares each running
sandbox's deadline with the clock and starts a `kaas stop` job for overdue ones
that have no job in flight (a running job pushes the deadline, the way a running
agent turn does on boat.dev). The default TTL comes from `defaults.auto_stop`
(built-in `never`); the create form offers `never / 1h / 4h / 12h / 1d / 7d`.
Templates are never auto-stopped. Without `kaas ui start` (or `brew services
start kaas`) deadlines are recorded but nothing enforces them.

## Environment and drift

The Environment card is the redacted persisted contract: workspace sources,
declared services, secret **key names** (never values), egress mode and hosts,
and the contract hash. When the sandbox's project directory still exists, the
UI resolves the contract that `kaas.yaml` would produce today and lists
field-level differences. Resume commands keep the persisted contract regardless;
**Re-adopt** is the explicit accept.

## Phase logs

Provision, agents, pubkey, and setup tee their Ansible/SSH output to
`<paths.logs_dir>/<sandbox>/<phase>.log`. The sandbox page lists the files and
shows the tail of the newest one, so a failed `setup` can be read without
re-running it. **Re-run setup** starts `kaas setup` as a job.

## Run in the guest

Two boxes on a running sandbox wrap `kaas exec … --detach`:

- **Command** — a shell command with a working directory (defaults to the
  workspace). It starts detached in its own session; the list below refreshes
  every 4 s with status, exit code, and a 20-line log tail, and offers **Kill**
  (SIGTERM to the process group, SIGKILL after 5 s). Output lives on the guest
  under `~/.kaas-runs/<id>.log`; `kaas exec <name> --attach <id>` streams it.
- **Prompt an agent** — picks a harness (`pi`, `omp`, `prime-agent`) and runs
  `<harness> -p '<prompt>'` in the workspace, detached. The harness's own
  session directory is the conversation memory; the UI stores nothing.

A **Processes** disclosure runs `kaas ps` on demand (busiest first). Both boxes
are disabled unless the sandbox is running.

The **creds** chip next to the readiness badge says whether a prompt can
authenticate: `deployed` (last `secrets` event ok), `delegated (proxy_only)`
(host proxy running), or `none` / `deploy failed` / `proxy down` with the exact
command to fix it — the local equivalent of boat.dev's
`provider_not_configured`.

## Service health

Each published service row on the sandbox page is probed on the allocated host
port (300 ms TCP dial). `listening` means the SSH forward is up and the guest
answers; `down` means the service is declared but nothing listens — typically a
dev server that was started by hand and did not survive a stop/start. For each
down service the page offers a ready-to-paste **systemd unit** so the service
comes back by itself, which is the same advice boat.dev gives for always-on
services.

## New sandbox form

The form mirrors `boat new`: pick a name (one is suggested), a size, a trust
level, and optionally a workspace. It submits a `kaas up` job and redirects to
the sandbox page, which shows the live phase strip until the row exists.

| Field | Flag it becomes |
|---|---|
| Size `small` / `default` / `large` | `--size` (presets from `defaults.sizes`; `default` = `defaults.cpus/memory_gib/disk_gib`) |
| Trust **Trusted** | (none) — current behaviour; `kaas secrets deploy` writes values into the guest |
| Trust **Untrusted** | `--trust untrusted` — delegated egress, `proxy_only`, default deny; GitHub hosts allowed without a token so checkout works |
| Start from **Template** | `--from <template>` — clone the template disk, skip provision/agents |
| Auto-stop | `--auto-stop <ttl>` — `never`, `1h`, `4h`, `12h`, `1d`, `7d` |
| Project directory on this host | the job's working directory, so that project's `kaas.yaml` applies |
| Repository URL + branch | `--repo` / `--branch` — primary `workspace.sources[0]` under `~/workspace/<repo>` without a `kaas.yaml` |

A size is marked **tight** when starting it would push memory promised to
running sandboxes past 85 % of host RAM. This is advisory; nothing is blocked.

## Jobs

Each job row stores the sandbox, verb, args, status (`queued` → `running` →
`ok` | `failed`), the last `message` record from the JSONL stream as its phase,
an exit code, and a log path. Logs are written to `~/.kaas/ui/jobs/<id>.log`
(stdout JSONL plus stderr prefixed with `! `). The **Jobs** page lists them;
`/jobs/<id>/log` serves the raw file.

Jobs run from the sandbox's recorded project directory when it still exists,
otherwise from `$HOME`, so no unrelated `kaas.yaml` is discovered by accident.

Short reads — command status and tails, the process list, detaching a guest
command — call `kaas … --jsonl` synchronously with a 45 s cap instead of
creating a job; only long lifecycle work becomes a job.

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
| `ui_runtime.root` | `~/.kaas/ui` | Pid/log directory for `kaas ui start`; job logs under `jobs/` |
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

## Security note

Loopback only: no TLS, no auth. Do not expose the port remotely. Anyone who can
reach the port can start, stop, fork, and destroy sandboxes exactly as the local
user could at the shell; destroy still requires typing the name.
