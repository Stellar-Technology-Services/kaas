---
title: Secrets & SecretSpec
description: How kaas declares secrets in secretspec.toml, stores values in a keyring, and deploys them onto a sandbox VM.
---

Secrets are managed with [SecretSpec](https://secretspec.dev/): the **contract** (which keys exist) is committed in [`secretspec.toml`](https://github.com/Stellar-Technology-Services/kaas/blob/main/secretspec.toml); the **values** live in a provider (recommended: the OS keyring) and are never committed.

## Choose a credential model

SecretSpec remains the declaration and provider layer in both models. Choose
how each value becomes usable:

| Model | Choose it when | Guest can read the value? | Network meaning |
|---|---|---:|---|
| `kaas secrets deploy` | A trusted workspace or local/non-HTTP tool must possess the value | Yes | Normal guest networking |
| `proxy_preferred` | An HTTP client supports the proxy and CA, but compatibility matters more than enforcement | No | Proxy variables are configured, but bypass remains possible |
| `proxy_only` | Untrusted unprivileged code needs narrowly allowlisted HTTP(S) capabilities | No | nftables permits only the host proxy; guest root is out of scope |

A rule without `inject_secret` permits a destination without attaching a
credential. Running `kaas secrets deploy` explicitly always changes the trust
model: the selected scope becomes guest-readable, even if one of its keys also
appears in the egress policy.

See [Delegated Egress](./egress.md) for configuration, lifecycle, mixed-mode
scopes, limitations, and the proxy architecture.

## The contract: `secretspec.toml`

`kaas init` installs the SecretSpec contract at `~/.kaas/secretspec.toml`. All
`kaas secrets *` commands (including `deploy`) read that user-global file — not
the control repository that registered the sandbox. The copy in this repository
is the template that `kaas init` embeds.

`secretspec.toml` declares the project and its keys. Two profile conventions matter:

| Profile | Meaning |
|---|---|
| `default` | Shared keys for every sandbox |
| `<sandbox>` | Per-instance overrides (same name as `kaas create <name>`) |

```toml
[project]
name = "kaas"
revision = "1.0"

[providers]
local = "keyring://"

[profiles.default]
ANTHROPIC_API_KEY = { description = "Anthropic / Claude", required = false, providers = ["local"] }
GITHUB_TOKEN      = { description = "GitHub token", required = false, providers = ["local"] }
OPENAI_API_KEY    = { description = "OpenAI", required = false, providers = ["local"] }
OPENROUTER_API_KEY= { description = "OpenRouter", required = false, providers = ["local"] }
GOOGLE_API_KEY    = { description = "Google Gemini", required = false, providers = ["local"] }
DEEPSEEK_API_KEY  = { description = "DeepSeek", required = false, providers = ["local"] }

[scopes.agents]
secrets = ["ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "GOOGLE_API_KEY", "DEEPSEEK_API_KEY", "GITHUB_TOKEN"]

[scopes.github]
secrets = ["GITHUB_TOKEN"]
```

Keyring path: `secretspec/kaas/{profile}/{KEY}`.

## Setup (once)

```bash
secretspec config global init   # pick a keyring (recommended)
kaas secrets init                # verify secretspec.toml + print hints
```

## Store a value

```bash
# Shared key used by every sandbox (profile "default") — prompts interactively
kaas secrets set ANTHROPIC_API_KEY

# Per-sandbox override (profile = sandbox name)
kaas secrets set GITHUB_TOKEN --sandbox my-app
```

`set` uppercases the key and stores it via the provider for the chosen profile.

## Verify

```bash
kaas secrets check                  # check profile default
kaas secrets check --sandbox my-app # check a sandbox profile
```

## Deploy onto a sandbox

```bash
kaas secrets deploy my-app
```

`deploy` ([`internal/cli/secrets.go`](https://github.com/Stellar-Technology-Services/kaas/blob/main/internal/cli/secrets.go)):

1. Resolves SecretSpec for **`default ∪ <sandbox>`** (the sandbox profile wins on conflicts).
2. Converts the resolved values to Ansible vars and writes a temporary `kaas-secrets-*.yml` (`0600`, removed afterward).
3. Runs `ansible/deploy-secrets.yml` over SSH, which writes non-empty values to `~/.profile.d/agent-secrets.sh` (`0600`) and sources it from `.bashrc` and `.profile`.

The sandbox must already exist (`kaas up my-app`) so its SSH port and inventory resolve.

### After deploy

```bash
kaas ssh my-app
echo $ANTHROPIC_API_KEY   # present in the guest shell
```

The exported variables are picked up by the coding-agent CLIs (pi, omp).

## Scope filtering

```bash
kaas secrets deploy my-app --scope github   # only GITHUB_TOKEN (scopes.github)
kaas secrets deploy my-app --scope agents   # only the agents scope keys
```



## Key points

- **Declaration ≠ storage.** `~/.kaas/secretspec.toml` declares keys; values live in the keyring.
- **Profile merging.** `deploy` merges `default ∪ <sandbox>`, sandbox wins.
- **Two things to check if deploy fails:** `secretspec config global init` wasn't run, or the key isn't declared in `[profiles]`.


## Delegated SecretSpec values

When project `egress.mode` is `proxy_preferred` or `proxy_only`, `kaas up` does
not call the trusted guest deployment path. It resolves only keys referenced by
`inject_secret`, passes them to the host proxy over private stdin, and installs
non-secret placeholders in the guest. `kaas egress reload` rotates them over a
`0600` Unix socket. Basic values use `username:password` form. Running `kaas
secrets deploy` explicitly remains the trusted, guest-readable workflow.
