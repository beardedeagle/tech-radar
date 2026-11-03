---
title: "Releasing"
weight: 40
---

# Releasing

Choose the version yourself. The release workflow runs the shared checks, builds six archives, signs their provenance, then creates the tag and GitHub release. It does not choose versions or open version-bump PRs.

## Prepare a version

Update `Version` in `internal/radar/model.go` through a reviewed PR. Update version examples when they change. This value also identifies report generators and saved contexts, so treat compatibility as part of that review. Release packaging does not replace it with linker flags.

Keep the three plugin manifests under `plugin/tech-radar` and the plugin entry in `.claude-plugin/marketplace.json` on that same version. `just check-versions` checks them against the CLI source version. Local gates and CI run this check, and publishing checks again before creating a tag. Marketplace catalogs do not need a separate release version.

Version 0.3.0 also reads saved 0.2.0 and 0.2.1 contexts. Validation retains their original generator metadata and fingerprints. Newly built contexts use 0.3.0. Unsupported generator versions and edited contexts are rejected.

Merge the PR into `main`, then start a release from the project root:

```sh
just release 0.3.0
```

This dispatches the Release workflow on `main`. You can also choose **Run workflow** in GitHub Actions and enter the version. An optional `v` prefix is accepted. Prereleases such as `0.3.0-rc.1` are marked as prereleases. Build metadata is not accepted.

## What runs

The workflow calls CI before publishing. All build and check commands come from the Justfile. Publication requires a clean commit on `origin/main`, a version matching the compiled CLI, and an unused tag. Only the publish job gets repository write permission and permission to sign and upload attestations.

GoReleaser builds Linux, macOS, and Windows binaries for amd64 and arm64 without cgo. Archives include the license, README, and third-party notices. Windows uses ZIP; the other targets use tar.gz. A SHA-256 checksum file covers the archives. Cross-compilation checks that binaries build; native CI tests cover the three operating systems separately.

The preparation recipe creates a temporary local tag for packaging and removes it on exit. GitHub's SHA-pinned `actions/attest` signs provenance for all six archives and the checksum manifest using the workflow's short-lived identity. No stored private signing key is needed. The publishing recipe rechecks the release guards and archive checksums, then verifies every file against that run's attestation bundle. Verification requires this repository's release workflow on `main`, the exact source and signer commit, and a GitHub-hosted runner. A failed signature or identity check stops publication.

GitHub CLI creates the remote tag and release only after verification succeeds. GoReleaser itself has publishing disabled. If an upload fails after GitHub creates the release, inspect the release and its assets before deciding how to recover. Rerunning refuses an existing tag.

## Verify a download

For releases from 0.2.1 onward, verify each downloaded archive or checksum file with GitHub CLI:

```sh
gh attestation verify tech-radar_0.3.0_darwin_arm64.tar.gz \
  --repo beardedeagle/tech-radar \
  --cert-identity https://github.com/beardedeagle/tech-radar/.github/workflows/release.yml@refs/heads/main \
  --source-ref refs/heads/main --deny-self-hosted-runners
```

GitHub stores the attestations separately from the release downloads. These attestations verify artifact provenance; they do not provide macOS notarization or Windows Authenticode signing. Release 0.2.0 has checksums only.

## Test packaging

```sh
just release-snapshot
```

This writes archives and checksums under `output/release`. It creates no tag or release and needs no publishing token. Snapshot archive names identify the build; the executable still reports the source version.
