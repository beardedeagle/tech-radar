---
title: "Build the handbook"
weight: 30
---

# Build the handbook

The handbook uses Hugo and Hugo Book. Markdown lives under `docs/content`, navigation follows the content tree and page weights, and the theme is pinned in `docs/go.mod` with checksums in `docs/go.sum`.

The documentation module is separate from the CLI's root module. Theme dependencies do not enter the application build. No Node or Sass step is required by this theme version.

## Preview locally

Use Hugo 0.169.0, the version used to verify this handbook. From the project root:

```sh
hugo server --source docs --bind 127.0.0.1
```

Open `http://localhost:1313/`. Hugo rebuilds changed pages during preview.

If Hugo is not installed, you can run the pinned builder with Go 1.27 or later:

```sh
go run github.com/gohugoio/hugo@v0.169.0 server --source docs --bind 127.0.0.1
```

The CLI itself still requires Go 1.26.4. The higher version above applies to building this Hugo release from source. Module resolution may need network access on the first documentation build.

## Build static output

```sh
hugo --source docs
```

The output is `output/handbook`. This uses the project's existing ignored output directory, which the scanner also excludes. For a site hosted under a subpath, supply its actual URL with `--baseURL` when building. Hugo uses that base URL for navigation, assets, and search links. Publishing the directory is a separate deployment step.

## Publish to GitHub Pages

The Pages workflow builds and deploys the handbook when changes reach `main`. You can also run it manually from GitHub Actions. It uses the Pages site URL for `--baseURL` and uploads only `output/handbook`. GitHub Pages must use GitHub Actions as its build source.

The published handbook is at <https://beardedeagle.github.io/tech-radar/>. A failed build leaves the last successful deployment in place.

## Verify a change

1. Follow the walkthrough or guide commands with synthetic repositories. Check the expected findings, files, and exit codes.
2. Build the handbook. Portable Markdown links are configured as errors, so unresolved page links fail the build.
3. Follow the reading paths and cross-links in the generated site. Check page headings, narrow navigation, browser search, light appearance, and dark appearance.
4. Confirm the root CLI module and application source did not change during documentation-only work.

Use a tutorial for a first complete run, a guide for a team task, and reference for exact formats or limits. Explain what an example proves and what it leaves unknown. Preserve useful architecture explanations for readers extending the software. Research transcripts, implementation progress, and agent handoffs belong outside the public handbook.
