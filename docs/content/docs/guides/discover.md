---
title: "Build a shared checklist"
weight: 30
---

# Portfolio discovery

Use discovery to build a shared checklist from an organization, a repo list or local checkouts. Pass the resulting context file to each repo's scan.

```sh
tech-radar discover --org example-org --out context.json
tech-radar generate --repo /path/to/repo --context context.json --out TECHNOLOGY_RADAR.md
```

## Start with local repositories

```sh
./tech-radar discover --repo /path/to/api --repo /path/to/web --out context.json
./tech-radar generate --repo /path/to/api --context context.json --out API_RADAR.md
./tech-radar generate --repo /path/to/web --context context.json --out WEB_RADAR.md
```

Both reports use the same frozen checklist, while each repository still has to establish its own findings and roles. To verify offline replay:

```sh
./tech-radar discover --from context.json --out rebuilt-context.json
cmp context.json rebuilt-context.json
```

For an intact context, `cmp` exits successfully without output. Rebuild uses saved facts. It does not fetch new source or membership.

## Inputs

Choose one:

- `--org NAME`: repos visible to your github.com credentials.
- `--repos FILE`: a schema-1 JSON list of local `path` entries or `github: "owner/repo"` entries.
- Repeated `--repo PATH`: local checkouts.
- `--from FILE`: rebuild a saved context offline.

A repository list has this shape:

```json
{
  "schema_version": 1,
  "repositories": [
    {"path": "../api", "name": "example-api"},
    {"github": "example-org/web"}
  ]
}
```

Local list entries can supply a `name`. Otherwise, the directory basename is used. Names must be unique. Relative paths are resolved beside the list file.

GitHub discovery includes accessible archived repos and forks and records those flags. Each repo's default-branch commit is pinned before reading its archive. Empty repos are recorded. A failure to read a requested repo stops collection instead of returning a partial context.

Organization membership reflects what the credential can see during collection. Repos can change while the listing runs, so this isn't an atomic snapshot of the organization.

## What's saved

The context includes repo identities, revisions, eligible file hashes, findings, unknown dependencies, warnings and matching profiles. Absolute local paths and raw source/configuration values aren't saved. Repo names, relative paths and dependency names can still be private.

A saved context rebuilds without the original repos or GitHub access. Use the same generator/catalog versions. Repo order and absolute checkout location don't affect equivalent output. To pick up new membership or source revisions, collect again.

Hashes let us check that the saved inputs and derived checklist agree. They don't prove who supplied the source. Context imports and exports are limited to 128 MiB.

## Profiles

Profiles give the scan a checklist appropriate to the language and application. Available candidates are:

- BEAM web/data and CQRS/events.
- React web, Node GraphQL APIs and mobile clients.
- Python AI/API, Rails and PHP applications.
- Background workers.
- Infrastructure/GitOps.
- Plugin and skill artifacts.
- Go modules/services.
- JVM and .NET projects, and Rust packages.

These are starting points for review. A Go module doesn't establish an HTTP service, and a package declaration doesn't establish a deployment.

Each repo must match a profile locally. Repo frequency is recorded, but doesn't determine importance, placement or approval. Unknown findings remain available for investigation. Development-only declarations don't establish a runtime profile.

The context doesn't include executable detection rules. Supply private `--rules` files separately during discovery and later repo scans. Policy still records team decisions.

All profiles and manifest readers are also available without context. When updating a report, supply its context again. Classification changes flag saved decisions for review.

Current captures use `portfolio-3`, which fixes profile selection for one-character component paths and omits redundant built-in catalog definitions. Saved `portfolio-1` and `portfolio-2` contexts still import and export unchanged using their original assessment rules. Collect again to apply current rules; offline re-export does not upgrade an old context. Drift comparisons across capture versions report a coverage gap; capture both revisions with the same version to compare them.

## GitHub access and limits

Requests use the official `go-gh/v2` library and existing github.com credentials. Keychain-only authentication may use installed `gh` to retrieve a token. Requests don't invoke `gh api`. Enterprise hosts aren't supported.

Archives are read into a private temporary directory and removed after capture. Links are skipped. Traversal paths, duplicate files, multiple archive roots and other unsafe archive content stop collection. Repo code, builds and hooks aren't executed.

| Per-repo limit | Maximum |
| --- | ---: |
| Compressed archive | 512 MiB |
| Expanded archive | 2 GiB |
| Archive entries | 100,000 |
| Eligible file | 2 MiB |
| Path length | 4,096 bytes |
| Metadata response | 16 MiB |
| Error response | 64 KiB |
| Request duration | 10 minutes |

Organization listings are limited to 1,000 pages of 100 repos. Limits that would truncate collection cause an error. Unsupported manifest syntax and skipped eligible files produce warnings.

## Code

`internal/radar` captures findings and hashes, reads manifests, builds contexts and applies profiles. `internal/discovery` lists remote repos and reads pinned archives. The CLI selects sources and protects input/output files.

The main interfaces are `CaptureRepository`, `BuildContext`, `ReadContext` and `discovery.Collect`. Archive filtering uses `PortfolioPathEligible` so local and remote capture follow the same file rules. See [testing](../contributors/testing.md) for coverage and manual integration checks.
