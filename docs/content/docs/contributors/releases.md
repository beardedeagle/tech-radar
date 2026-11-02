---
title: "Releasing"
weight: 40
---

# Releasing

Choose the version yourself. The release workflow runs the shared checks, builds six archives, then creates the tag and GitHub release. It does not choose versions or open version-bump PRs.

## Prepare a version

Update `Version` in `internal/radar/model.go` through a reviewed PR. Update version examples when they change. This value also identifies report generators and saved contexts, so treat compatibility as part of that review. Release packaging does not replace it with linker flags.

Merge the PR into `main`, then start a release from the project root:

```sh
just release 0.2.0
```

This dispatches the Release workflow on `main`. You can also choose **Run workflow** in GitHub Actions and enter the version. An optional `v` prefix is accepted. Prereleases such as `0.2.0-rc.1` are marked as prereleases. Build metadata is not accepted.

## What runs

The workflow calls CI before publishing. All build and check commands come from the Justfile. Publication requires a clean commit on `origin/main`, a version matching the compiled CLI, and an unused tag. Only the publish job gets repository write permission.

GoReleaser builds Linux, macOS, and Windows binaries for amd64 and arm64 without cgo. Archives include the license, README, and third-party notices. Windows uses ZIP; the other targets use tar.gz. A SHA-256 checksum file covers the archives. Cross-compilation checks that binaries build; native CI tests cover the three operating systems separately.

The publishing recipe creates a temporary local tag to prepare the release, removes it on exit, and uses GitHub CLI to create the remote tag and release after packaging succeeds. GoReleaser itself has publishing disabled. If an upload fails after GitHub creates the release, inspect the release and its assets before deciding how to recover. Rerunning refuses an existing tag.

## Test packaging

```sh
just release-snapshot
```

This writes archives and checksums under `output/release`. It creates no tag or release and needs no publishing token. Snapshot archive names identify the build; the executable still reports the source version.
