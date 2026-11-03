---
title: "Build the handbook"
weight: 30
---

# Build the handbook

The handbook uses Hugo and Hugo Book. Markdown lives under `docs/content`, navigation follows the content tree and page weights, and the theme is pinned in `docs/go.mod` with checksums in `docs/go.sum`.

The documentation module is separate from the CLI's root module. Theme dependencies do not enter the application build. No Node or Sass step is required by this theme version.

## Preview locally

From the project root:

```sh
just docs-serve
```

Open `http://localhost:1313/`. Hugo rebuilds changed pages during preview.

The recipe installs pinned Hugo 0.169.0 under `output/tools` using the automation Go toolchain. The CLI itself still requires Go 1.26.7. Toolchain and module downloads need network access on the first documentation build.

## Build static output

```sh
just docs-build
```

The output is `output/handbook`. This uses the project's existing ignored output directory, which the scanner also excludes. The default URL is the published site. To build for another host or subpath, set `HUGO_BASEURL` when running the recipe. Hugo uses that URL for navigation, assets, and search links. Publishing the directory is a separate deployment step.

## Publish to GitHub Pages

The Pages workflow builds and deploys the handbook when changes reach `main`. You can also run it manually from GitHub Actions. It uses the Pages site URL for `--baseURL` and uploads only `output/handbook`. GitHub Pages must use GitHub Actions as its build source.

The published handbook is at <https://beardedeagle.github.io/tech-radar/>. A failed build leaves the last successful deployment in place.

## Verify a change

1. Follow the walkthrough or guide commands with synthetic repositories. Check the expected findings, files, and exit codes.
2. Build the handbook. Portable Markdown links are configured as errors, so unresolved page links fail the build.
3. Follow the reading paths and cross-links in the generated site. Check page headings, narrow navigation, browser search, light appearance, and dark appearance.
4. Confirm the root CLI module and application source did not change during documentation-only work.

Use a tutorial for a first complete run, a guide for a team task, and reference for exact formats or limits. Explain what an example proves and what it leaves unknown. Preserve useful architecture explanations for readers extending the software. Research transcripts, implementation progress, and agent handoffs belong outside the public handbook.
