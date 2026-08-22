# AGENTS.md — Instructions for coding agents

These instructions are read by coding agents (pi, and
other AGENTS.md-aware tools) whenever they work in this repository.

## Golden rule: keep docs in sync with code

**After every significant code change, update the documentation in the same
change set.** A change that touches behavior, commands, configuration,
secrets, or architecture is **not complete** until both of these are updated:

1. **The Starlight docs site** — `docs/src/content/docs/**/*.md`
2. **The README** — `README.md`

### What counts as "significant"

Any change that would alter what a user or maintainer needs to know. When you
make one of these, update the matching page (then the README if it's affected):

| Change | Update |
|---|---|
| New / changed CLI command or flag | `docs/src/content/docs/reference/cli.md` and the README command table |
| Config key, default, or env var changed | `docs/src/content/docs/guides/configuration.md` |
| Secrets / SecretSpec change | `docs/src/content/docs/guides/secrets.md` |
| Toolchain or coding-agent added/removed | `docs/src/content/docs/guides/toolchains.md` |
| Playbook / inventory / `ansible.cfg` change | `docs/src/content/docs/reference/playbooks.md` and `docs/src/content/docs/guides/ansible.md` when ownership or extension guidance changes |
| Architecture or control-flow change | `docs/src/content/docs/reference/architecture.md` |
| Build / tests / layout change | `docs/src/content/docs/development.md` |

### Always

- Update the page(s) above in the **same change** as the code.
- Reflect new commands, flags, config keys, and defaults in the README
  (`README.md`) command table and defaults section.
- When a change makes the top-level flow or a command's purpose different,
  also refresh `docs/src/content/docs/overview.md` and
  `docs/src/content/docs/getting-started.md`.

### How to verify the docs still build

The Starlight site must keep building after every documentation change:

```bash
cd docs
npm run build
```

When you add a new page, register it in the `sidebar` inside
`docs/astro.config.mjs`.

The docs site deploys to GitHub Pages on `v*` tags via
`.github/workflows/docs.yml` (see `docs/README.md`).
