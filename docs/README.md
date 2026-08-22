# KaaS Docs

Starlight documentation site for [kaas — the KaaS](../README.md).

Published site: [https://stellar-technology-services.github.io/kaas/](https://stellar-technology-services.github.io/kaas/)

## Get started

```bash
cd docs
npm install
npm run dev        # http://localhost:4321
```

## Build

```bash
cd docs
npm run build      # → docs/dist (site/base default to /kaas/)
npm run preview    # preview the production build
```

## Deploy (GitHub Pages)

Docs deploy automatically on `v*` tag push (and via workflow dispatch) through
[`.github/workflows/docs.yml`](../.github/workflows/docs.yml).

Public URL: **https://stellar-technology-services.github.io/kaas/**

One-time GitHub setup:

1. Open the repo **Settings → Pages**.
2. Set **Source** to **GitHub Actions**.
3. Under **Environments → github-pages → Deployment branches and tags**, allow
   tags matching `v*` (in addition to `main` if present).
4. Push a tag (or run **Deploy docs** manually from the Actions tab):

```bash
git tag v0.1.0
git push origin v0.1.0
```

The same tag also triggers the binary release workflow (Darwin and Linux).

## Structure

- **Framework**: [Astro](https://astro.build) + [Starlight](https://starlight.astro.build)
- **Config**: `astro.config.mjs` (`site`, `base`, title, sidebar, edit links)
- **Content**: Markdown in `src/content/docs/`, frontmatter validated by `src/content.config.ts`
- **Layout**: `src/layouts/`, styling overrides in `src/styles/custom.css`

`site` defaults to `https://stellar-technology-services.github.io/kaas` and `base` to `/kaas/` so
asset and page URLs match GitHub project Pages for `Stellar-Technology-Services/kaas`.

## Adding a page

Create a `.md` file under `src/content/docs/` with YAML frontmatter (`title`,
`description`), then add it to the `sidebar` in `astro.config.mjs`.
