---
title: Kaas — Koding Agent Application Sandbox
template: splash
hero:
  title: Kaas — Koding Agent Application Sandbox
  tagline: >
    Disposable Colima Ubuntu VMs for coding agents — toolchains, secrets, and
    coding agents (Pi, Oh My Pi) behind one CLI (`kaas`).
  actions:
    - text: Install on macOS
      link: ./guides/install-macos/
      icon: right-arrow
      variant: primary
    - text: Install on Linux
      link: ./guides/install-linux/
      icon: right-arrow
      variant: secondary
    - text: Get started
      link: ./getting-started/
      icon: right-arrow
      variant: minimal
    - text: CLI Reference
      link: ./reference/cli/
      icon: bars
---

<div class="home-cards">
  <a class="home-card" href="./guides/install-macos/">
    <h3>Install on macOS</h3>
    <p>Homebrew or a Darwin release binary, Colima/Lima, and <code>kaas init</code>.</p>
  </a>
  <a class="home-card" href="./guides/install-linux/">
    <h3>Install on Linux</h3>
    <p>KVM via apt, Colima/Lima release binaries, and <code>kaas doctor</code>.</p>
  </a>
  <a class="home-card" href="./getting-started/">
    <h3>Getting Started</h3>
    <p>Scaffold a project contract, create your first sandbox, add secrets, and tear it down.</p>
  </a>
  <a class="home-card" href="./guides/ansible/">
    <h3>Ansible</h3>
    <p>Why playbooks are required, what they own, and how to extend the guest baseline.</p>
  </a>
  <a class="home-card" href="./guides/secrets/">
    <h3>Secrets &amp; SecretSpec</h3>
    <p>Declare API keys, store values in a keyring, and choose trusted guest injection or delegated use.</p>
  </a>
  <a class="home-card" href="./guides/egress/">
    <h3>Delegated Egress</h3>
    <p>Keep HTTP credentials host-side and inject them only at approved destinations.</p>
  </a>
  <a class="home-card" href="./guides/services/">
    <h3>Local services</h3>
    <p>Publish guest HTTP apps on unique host ports and optional <code>*.kaas.test</code> names.</p>
  </a>
  <a class="home-card" href="./faq/">
    <h3>FAQ</h3>
    <p>Who owns Ansible, how projects customize Kaas, and why the baseline is shared.</p>
  </a>
  <a class="home-card" href="./guides/configuration/">
    <h3>Configuration</h3>
    <p><code>kaas.yaml</code>, environment variables, precedence, and every default.</p>
  </a>
  <a class="home-card" href="./guides/toolchains/">
    <h3>Toolchains &amp; Agents</h3>
    <p>System packages, Go, Node, Rust, Python, Herdr, and the Pi / Oh My Pi coding agents.</p>
  </a>
  <a class="home-card" href="./reference/cli/">
    <h3>CLI Reference</h3>
    <p>Every <code>kaas</code> command, its flags, and lifecycle examples.</p>
  </a>
  <a class="home-card" href="./reference/architecture/">
    <h3>Architecture</h3>
    <p>How Kaas orchestrates Colima, Ansible, SecretSpec, egress, and local service ingress.</p>
  </a>
  <a class="home-card" href="./reference/playbooks/">
    <h3>Playbooks</h3>
    <p>The Ansible playbooks, <code>ansible/hosts.ini</code>, <code>ansible/ansible.cfg</code>, and group variables.</p>
  </a>
  <a class="home-card" href="./development/">
    <h3>Development</h3>
    <p>Build, test, vet, and publish the CLI.</p>
  </a>
</div>
