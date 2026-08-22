---
title: Delegated Egress
description: Why KaaS can keep HTTP credentials out of a guest, how the host proxy works, and when to use delegated egress instead of guest-injected secrets.
---

KaaS supports two deliberately different ways to make a SecretSpec value useful
inside a sandbox:

1. **Guest-injected secret:** `kaas secrets deploy` writes the real value into
   `~/.profile.d/agent-secrets.sh` in the VM.
2. **Delegated egress credential:** the real value stays in a host-owned proxy;
   the guest receives a placeholder and can use the credential only in an
   approved HTTP(S) request.

These are different trust models, not interchangeable command spellings.

## Why delegated egress exists

A coding agent, dependency installer, build hook, or checked-out repository runs
as the guest user. If that user can read a token from `env`, a profile, a
credential file, or another process, the workload possesses the token and can
copy it elsewhere. SecretSpec protects storage and declaration on the host, but
ordinary deployment cannot make a value secret from the process that receives
it.

Delegated egress changes the capability from:

> “The guest possesses `GITHUB_TOKEN`.”

into:

> “The guest may ask KaaS to apply `GITHUB_TOKEN` to an HTTP request for
> `api.github.com`.”

This reduces accidental disclosure and blocks direct reading of the configured
value. It also prevents the proxy from attaching the credential to an unlisted
host. It is **not general data-loss prevention**: guest code can send other data
to any allowed destination and can still perform whatever operations the
delegated credential authorizes at that destination.

## Choose per secret

| Use delegated egress when… | Use guest injection when… |
|---|---|
| The credential is used as HTTP bearer, HTTP basic, or a named request header | A local process must read the actual value |
| The destination host list is stable and reviewable | The protocol is SSH, a database protocol, UDP, mTLS, signing, encryption, or another non-HTTP flow |
| The workload or repository is not fully trusted | The workspace and all guest code are trusted |
| Keeping the token out of `env`, files, argv, Ansible, and SQLite is important | Compatibility is more important than preventing guest possession |
| TLS interception is compatible with the client | The client pins certificates or cannot trust the KaaS proxy CA |

Typical delegated candidates include GitHub API, npm registry, and HTTP
provider API tokens. Typical guest-injected values include local test secrets,
database passwords consumed by a local process, SSH keys, signing keys, mTLS
private keys, or credentials embedded in a protocol KaaS does not understand.

Prefer a **hybrid policy** rather than forcing every value through the proxy.
Give each credential the narrowest mechanism compatible with its real use.

## Configure delegated egress

Declare only key names and destinations in the project-owned `kaas.yaml`:

```yaml
egress:
  schema_version: 1
  mode: proxy_only
  default: deny
  rules:
    # Reachable, but no credential is attached.
    - host: github.com

    # Reachable and receives GITHUB_TOKEN as a bearer credential.
    - host: api.github.com
      inject_secret:
        key: GITHUB_TOKEN
        type: bearer
```

Store values normally with SecretSpec:

```bash
kaas secrets set GITHUB_TOKEN --sandbox my-app
```

Then inspect and start the sandbox:

```bash
kaas egress plan
kaas up my-app
kaas egress status my-app
kaas egress verify my-app       # proxy_only only
```

For a `basic` rule, store the SecretSpec value as `username:password`. For a
named header, use `type: header` plus `header: X-Header-Name`. Declare any new
key in `secretspec.toml` before setting it. Project policy still contains only
the key name.

## Egress modes

| Mode | Behavior | Security meaning |
|---|---|---|
| `open` | Normal guest networking; `kaas up` uses the traditional guest-secret deployment flow | No egress boundary |
| `proxy_preferred` | Host proxy, public CA, proxy variables, and placeholders are installed | Compatibility mode; a process can bypass proxy variables |
| `proxy_only` | Same proxy setup plus a pinned nftables output/forward policy | Enforced for unprivileged guest workloads |

`proxy_only` permits loopback, SSH reply traffic, and TCP to the resolved host
proxy IP and port. Direct IP connections, direct DNS, UDP, and clean processes
without proxy variables remain blocked. Run `kaas egress verify` after setup or
network changes.

The nftables boundary lives inside the VM. Guest root or a process with
`CAP_NET_ADMIN` can remove it, so KaaS does not claim protection from guest
root. Stronger protection requires an enforcement point outside the guest.

## How the proxy works

```text
SecretSpec provider (host)
        │ resolve referenced keys only
        ▼
kaas CLI ── private stdin / 0600 Unix socket ──► host egress proxy
                                                     │
Guest receives:                                      │ match host policy
- public proxy CA                                    │ inject credential
- HTTP(S)_PROXY                                      ▼
- non-secret placeholders ── HTTP(S) request ──► approved destination
```

1. KaaS resolves only `inject_secret.key` values from `default ∪ <sandbox>`.
2. Initial values enter the child proxy over inherited stdin. Rotation uses a
   per-sandbox `0600` Unix control socket.
3. The proxy keeps credentials in memory. It matches the HTTP destination,
   overwrites the configured bearer/basic/named header, and denies unlisted
   hosts under `default: deny`.
4. For HTTPS, the proxy terminates TLS with a per-sandbox CA and creates a new
   TLS connection upstream. The private CA key remains host-only; Ansible
   installs only the public certificate in the guest.
5. KaaS persists only redacted metadata: mode, proxy port, status, CA
   fingerprint, and policy hash.
6. `stop` terminates the proxy, `start` restores it, `destroy` removes its
   host-private directory, and `fork` does not inherit proxy credentials or
   runtime metadata.

## Rotation and diagnosis

```bash
kaas egress reload my-app       # re-resolve and atomically replace values
kaas egress status my-app       # redacted status and CA identity
kaas egress plan                # review hosts and key names, never values
kaas events my-app              # redacted lifecycle events
```

If an authenticated request fails, check—in order—the effective plan, whether
the SecretSpec key resolves for the sandbox profile, proxy status, redirects to
additional hosts, client trust of the public CA, and finally provider-side token
permissions or expiry.

## Mixing delegated and guest-injected values

When `egress.mode` is `proxy_preferred` or `proxy_only`, `kaas up` intentionally
**skips automatic `kaas secrets deploy`** so delegated values cannot accidentally
travel through Ansible into the guest.

If the same sandbox also needs guest-readable values:

1. Put guest-readable and delegated keys in separate SecretSpec scopes.
2. Configure only the delegated keys under `egress.rules[].inject_secret`.
3. Explicitly deploy only the guest scope:

```bash
kaas secrets deploy my-app --scope guest-runtime
```

Never deploy a scope containing a delegated key. The explicit `secrets deploy`
command remains a trusted-workspace escape hatch and does not automatically
filter keys referenced by the egress policy.

For example:

```toml
[scopes.delegated-http]
secrets = ["GITHUB_TOKEN", "NPM_TOKEN"]

[scopes.guest-runtime]
secrets = ["LOCAL_DATABASE_PASSWORD"]
```

This split makes the trust decision reviewable: key names in
`delegated-http` remain proxy-held, while values in `guest-runtime` are
intentionally readable by guest code.

## Limitations

- Only HTTP(S) bearer, basic, and named-header credentials are modeled.
- Certificate-pinned clients and some bundled trust stores may reject TLS
  interception.
- Allowed hosts may include redirects, CDNs, download hosts, or registry storage
  endpoints; keep the reviewed list complete but narrow.
- The proxy is part of the availability path for authenticated HTTP(S).
- Delegation limits possession and destination; it does not reduce the
  provider-side permissions of the token. Use narrowly scoped tokens too.
