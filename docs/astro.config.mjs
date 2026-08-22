// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Local defaults match project Pages for Stellar-Technology-Services/kaas.
// CI may override SITE / BASE (see .github/workflows/docs.yml).
//const site = process.env.SITE || 'https://stellar-technology-services.github.io/kaas';
const site = 'https://stellar-technology-services.github.io/kaas';
//const base = process.env.BASE || '/kaas/';
const base = '/kaas/';
const githubRepo = process.env.GITHUB_REPOSITORY || 'Stellar-Technology-Services/kaas';

export default defineConfig({
	site,
	base,
	integrations: [
		starlight({
			title: 'KaaS — Koding Agent Application Sandbox',
			description:
				'Documentation for kaas — the Koding Agent Application Sandbox CLI and Ansible playbooks for disposable Colima Ubuntu VMs.',
			editLink: {
				baseUrl: `https://github.com/${githubRepo}/edit/main/docs`,
			},
			lastUpdated: true,
			sidebar: [
				{
					label: 'Start Here',
					items: [
						{ label: 'Overview', slug: 'overview' },
						{ label: 'Install on macOS', slug: 'guides/install-macos' },
						{ label: 'Install on Linux', slug: 'guides/install-linux' },
						{ label: 'Getting Started', slug: 'getting-started' },
						{ label: 'FAQ', slug: 'faq' },
					],
				},
				{
					label: 'Guides',
					items: [
						{ label: 'Ansible', slug: 'guides/ansible' },
						{ label: 'Secrets & SecretSpec', slug: 'guides/secrets' },
						{ label: 'Delegated Egress', slug: 'guides/egress' },
						{ label: 'Local services', slug: 'guides/services' },
						{ label: 'Local web UI', slug: 'guides/ui' },
						{ label: 'Configuration', slug: 'guides/configuration' },
						{ label: 'Toolchains & Coding Agents', slug: 'guides/toolchains' },
					],
				},
				{
					label: 'Reference',
					autogenerate: { directory: 'reference' },
				},
				{
					label: 'Project',
					items: [{ label: 'Development', slug: 'development' }],
				},
			],
		}),
	],
});
