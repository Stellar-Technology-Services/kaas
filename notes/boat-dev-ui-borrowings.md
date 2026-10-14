# What the kaas web UI can borrow from boat.dev
_Date: 2026-10-14. Sources: docs.boat.dev (quickstart, concepts, snapshots, named snapshots, resume-and-fork, environments, setup, readiness, auto-stop, always-on services, background commands, command history, integrated agents, conversations, parallel-agents recipe) against the current_ `internal/ui`_,_ `internal/sandbox/lifecycle`_,_ `internal/state`_, and CLI reference._
## 1. The gap in one sentence
Boat's "easy" is almost entirely **lifecycle ergonomics**, not a fancy dashboard: one command with good defaults (`boat new`), a tiny verb set (`exec / ssh / stop / resume / fork`), a disk-vs-settings split (named snapshot vs environment), and every command emitting machine-readable events. Its dashboard is thin. kaas already has most of the underlying machinery (desired/observed state, contract, fork, events, services) — the UI just exposes it read-only with one button (`Sync services`, `internal/ui/handlers.go:70`). The highest-leverage move is to make the UI a **driver of the existing CLI**, not a new control plane.
## 2. Concept mapping (what kaas already has)
| Boat word | kaas equivalent today | Gap |
|---|---|---|
| Sandbox (`bx_…`) | Sandbox name = Colima profile = SecretSpec profile = hostname | none |
| `boat new` | `kaas up <name>` (needs a name, project `kaas.yaml`, Ansible) | no defaults path, no UI |
| Size `small/default/large/xlarge` | `--cpus/--memory/--disk` or `defaults:` | no named presets |
| Stop / Resume (same id, disk kept) | `kaas stop` / `kaas start` | not in UI |
| Fork (source unaffected) | `kaas fork` — **source is briefly stopped** (`lifecycle/fork.go:107`) | not in UI; UX must say so |
| Named snapshot → `boat new --from` | nothing (a stopped sandbox *is* a template, via `limactl clone`) | no "template" role, no "new from" |
| Automatic per-minute snapshot | nothing; Lima `snapshot` is experimental, qemu-only | **do not borrow** (VZ) |
| Environment (repos, secrets, setup script, creds, versioned) | Contract (`sandbox_contracts`) + SecretSpec profile + egress policy; contract hash = version | not surfaced in UI |
| "Safe for third parties" switch | `egress.mode: proxy_only` + skip `secrets deploy` | not a single preset |
| Setup script status `pending/running/done/failed` + error tail | `setup_status` + events; no captured output | no logs to show |
| Auto-stop TTL / `boat extend` | nothing | missing — and *more* valuable on a laptop |
| `boat exec --detach / --attach / --kill`, `boat history`, `boat ps` | nothing (SSH only) | missing |
| `boat prompt` / conversations | Pi / Oh My Pi installed via Ansible; no runner | missing |
| Readiness `starting → ready → hydrated` | observed state + `up` phases (create→deps→provision→agents→secrets→checkout→setup) | UI shows states, not phases |
| `--json` events on every command | `--jsonl` on every command | UI doesn't use it (uses `CombinedOutput`) |
| Start-rate limits / `--fail-fast` | laptop CPU/RAM | no host-capacity view |
| Public URLs | `*.kaas.test` loopback ingress | deliberate; **do not borrow** |
## 3. Recommendations, prioritised ### P0 — the enabling piece: a job runner in the UI daemon Everything below needs the UI to start a `kaas` subcommand **in the background**, keep its JSONL stream, and let HTMX poll it. Today `handleSandboxSync` blocks the request on `exec.Command(...).CombinedOutput()` and returns 502 on failure (`internal/ui/handlers.go:85-91`). That pattern cannot host a 5-minute `kaas up`.
- Add a `jobs` table (`id, sandbox, command, status, started_at, finished_at, log_path`) in a new migration; log to `~/.kaas/ui/jobs/<id>.jsonl`.
  
- Spawn `kaas <verb> <name> --jsonl` (never parse human text). Parse the `message` / `result` records to drive a phase strip; keep raw stderr for the "show log" drawer.
  
- Reuse `sandbox_operations` claims so a double-click can't start two creates — this is boat's `Idempotency-Key` idea, which kaas already has in the store (`state/migrations/001_init.sql:64`).
  
- Routes: `POST /jobs`, `GET /jobs/{id}` (partial), `GET /jobs/{id}/log`.
  
### P1 — "sandbox in two minutes" from the browser
1. **New sandbox form** (`/sandboxes/new`) mirroring `boat new`:
  

- name auto-suggested (adjective-noun, validated against the identity grammar);
  
- **size presets** `small / default / large` → `cpus/memory_gib/disk_gib` (boat's 4 vCPU / 8 GB default maps onto kaas's existing `defaults:` block);
  
- optional repo URL + branch (writes `workspace.sources[0]`), optional `--from <template>` (see P2), optional auto-stop (see P2);
  
- a **trust preset**: "Trusted (deploy secrets)" vs "Untrusted (proxy_only, no guest secrets)" — this is boat's single "safe for third parties" switch, and kaas already has both halves.
  
- Submit → P0 job running `kaas up` → redirect to detail page with live phase strip.
  

2. **Lifecycle buttons on the detail page, gated by observed state**: Start, Stop, Fork, Destroy. Boat refuses commands with `409 sandbox_not_ready`; do the same by disabling buttons for transitional states and showing a spinner. Destroy requires typing the name (the `-y` confirmation moved into the browser). This reverses the v1 "destroy/fork intentionally omitted" note in `docs/src/content/docs/guides/ui.md:71`.
  
3. **Fork dialog must be honest about kaas's model**: "Source `my-app` will be stopped for the clone and restored to its prior state afterwards. Secrets and egress are not copied — run `kaas secrets deploy` / `kaas egress configure` on the fork." Boat's equivalent caveats (fork doesn't copy TTL; named snapshot doesn't copy environment) are what keep users from being surprised; copy that tone.
  
4. **Readiness badge** instead of two raw state pills: `Ready` = observed running + setup ok + services synced; otherwise `Starting (phase N/7)`, `Stopped`, `Setup failed`, `Needs credentials`. Derive from `events` + `sandboxes` columns — no new state needed.
  
5. **Connect panel** with copyable commands — boat's `current` shorthand in UI form: `kaas ssh my-app`, `kaas cp …`, published URLs, and an "Open in herdr" link when `herdr` is on PATH (`kaas deps --optional` already knows about it).
  
6. **Host capacity meter on Overview**: total host CPU/RAM vs sum of running sandboxes' `cpus/memory_gib`. Boat exposes plan caps and `429 rate_limited`; the local analog is "you are about to start a 4th 8 GiB VM on a 36 GiB laptop". Warn, don't block.
  
### P2 — borrow boat's snapshot model with what Lima can actually do
7. **Templates = stopped golden sandboxes.** Lima can't do live snapshots on VZ, but `limactl clone` of a _stopped_ instance is exactly `boat snapshot <id> <name>` + `boat new --from <name>`:
  

- "Save as template" on a detail page → stop, `kaas fork my-app tpl-web-stack`, mark the row `role = template`, `desired = stopped` (new nullable column).
  
- "New from template" in the create form → `kaas fork tpl-web-stack <new-name>` then run only the `secrets/egress → checkout → setup` phases (skip provision/agents).
  
- A **Templates** page listing them with size, created-at, and "Update" (re-clone from a running sandbox under the same name, like boat's "save the same name again").
  
- Document boat's rule verbatim: _what goes in a Dockerfile goes in a template; what goes in_ `.env` _goes in the environment (contract + SecretSpec)_. kaas's contract model already enforces the second half.
  
- Feasibility check before committing: whether `limactl clone` on APFS uses `clonefile` (instant) or a full copy (minutes for a 64 GiB sparse disk). That decides whether "New from template" feels like boat's "few seconds".
  

8. **Auto-stop TTL.** Add `auto_stop_at` to `sandboxes`; create form default "1 h after start" (boat's default), options 4 h / 12 h / never; row shows countdown; **Extend** button sets `now + TTL` (boat semantics: extend sets remaining time, doesn't add). The UI daemon (already long-running via `kaas ui start`) owns the ticker and runs `kaas stop --jsonl`. Boat's exception — "a running agent prompt pushes the deadline" — maps to "a running job pushes the deadline" once P3 exists.
  
9. **Setup logs and re-run.** Boat gives `setupStatus` + `setupError` tail and writes `/tmp/routine-logs/`. Capture `scripts/kaas-setup` output to `~/.kaas/logs/<sandbox>/setup.log` (and provision/agents likewise), show the last 40 lines on failure, add a **Re-run setup** button (→ `kaas setup`). Same for each `up` phase so the phase strip is clickable.
  
10. **Environment panel + drift.** Show the persisted contract redacted: sources, services, secret **key names**, egress mode/policy hash, contract hash. Compare against the discovered project `kaas.yaml` and show "contract drift" with a **Re-adopt** action. This is boat's "running sandboxes stay on their version until you upgrade them", and kaas's resume-authority rule already guarantees the safe half.
  
11. **Pre-stop hook.** Boat's "pre-stop command" → optional `scripts/kaas-prestop` run before `kaas stop`. Small, and pairs with auto-stop.
  
### P3 — commands and agents without a terminal
12. **Detached command runner** (`kaas exec <name> --detach|--attach|--kill|--status` plus a "Run" box on the detail page). Implement over SSH with `nohup … > ~/.kaas-runs/<id>.log 2>&1 & echo $!`, store rows in a `commands` table (boat keeps `command, cwd, mode, pid, status, exit_code, started/ended`), stream the log tail with HTMX polling (boat has no SSE either — it polls ~1/s). Kill = `kill -TERM -<pgid>`, then `-KILL` after 5 s, exactly as boat documents.
  
13. **History +** `ps`: list past commands with exit codes (survives stop because the log lives on the guest disk and the row lives in SQLite); a "Processes" tab running `ps -eo pid,pcpu,rss,user,cmd --sort=-pcpu | head -12` on demand.
  
14. **Prompt box** = run `pi -p "<prompt>"` (or OMP) through #12 with `cwd` = `workspace.sources[0].directory`. List sessions from the harness's own session dir; don't build a conversation store — boat's point is that the harness's native session _is_ the memory and it survives stop/fork because it's on disk.
  
15. **Credentials readiness**: boat fails fast with `provider_not_configured`. Show a "Credentials: deployed / delegated (proxy_only) / none" chip with the exact fix command, so a first prompt never fails silently.
  
16. **Service health**: probe each allocated host port; if a service is declared but not listening after `start`, show boat's advice — "a process you started by hand does not come back after stop; make it a systemd unit" — with a copyable unit snippet pre-filled from `workspace.services`.
  
## 4. Explicitly not borrowing
- Per-minute automatic snapshots / point-in-time restore — not available on Colima VZ.
  
- Public HTTPS URLs / share codes — kaas is loopback-only by design.
  
- Billing, seats, organisations, hydration progress — no local analog.
  
- A hosted agent server inside the guest — overkill; SSH + nohup + the harness's own session files cover the single-operator case.
  
## 5. Docs to update when implementing (per AGENTS.md)
- `docs/src/content/docs/guides/ui.md` — new pages/actions, drop the "destroy/fork omitted" note, document auto-stop and templates.
  
- `docs/src/content/docs/reference/cli.md` + README table — `kaas exec`, `kaas template`, `--from`, `--auto-stop`, `--size`.
  
- `docs/src/content/docs/guides/configuration.md` — `defaults.sizes`, `auto_stop`.
  
- `docs/src/content/docs/reference/architecture.md` — UI job runner, `jobs` / `commands` tables, template role.
  
- A short **Concepts** page modelled on boat's (one table: sandbox, stop, start, fork, template, contract, trusted/untrusted) — boat's glossary with a provider-mapping table is one of its clearest pages.
