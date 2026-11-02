---
title: "Build the CLI"
weight: 10
---

# Build the CLI

tech-radar requires Go 1.26.4 or later and [just](https://just.systems/). Build it from the project root:

```sh
just build
./tech-radar version
```

The version command identifies the CLI and its pinned language-counting engine:

```text
tech-radar 0.2.0 (scc/v4 v4.1.0)
```

The examples in this handbook run `./tech-radar` from that directory and pass the target repository separately. You can put the executable on your `PATH` and use `tech-radar` instead. The executable does not need to live inside the repository being scanned.

## Local and remote runs

Local scans need a readable checkout. Git is optional; when available, the report records the current commit as well as a fingerprint of the files actually read. Use a checkout that is not being edited during collection.

GitHub organization and repository-list collection use github.com credentials from `GH_TOKEN`, `GITHUB_TOKEN`, or existing GitHub CLI configuration. Keychain-only authentication may need the installed `gh` executable to retrieve the token. GitHub Enterprise hosts are not supported.

Hugo is needed only to build this documentation site. It is not needed to run the CLI. See [build the handbook](../contributors/documentation.md) if you are changing these pages.

Continue with [your first radar](first-radar.md).
