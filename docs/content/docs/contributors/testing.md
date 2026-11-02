---
title: "Testing"
weight: 20
---

# Testing

Install [just 1.54.0](https://just.systems/) and GitHub CLI, then run from the project root:

```sh
just setup
just ci
```

`just setup` installs pinned development tools under `output/tools` and installs Lefthook without replacing existing hooks. Pre-commit runs `just check`; pre-push runs `just ci`. Use `just fmt` to fix formatting. Windows development uses Git Bash.

The Justfile owns the commands. CI runs those same recipes, with native race tests on Linux, macOS, and Windows using the minimum Go version, plus Linux on the current pinned Go version. Documentation and release builds run on Linux. Source-installed automation tools use their own pinned Go toolchain without changing the application's module.

CI keeps the automation Go cache separate from native tests and shares it with publishing. The automation job saves the pinned tool binaries; security, Pages and publishing jobs restore them. Tool cache keys include the OS, architecture, Justfile and bootstrap installer, with no fallback to older keys. Cache misses use the same Just recipes to rebuild; all checks still run.

## Choosing tests

Use temporary repositories and fake GitHub responses. Run the affected package while working, then the full checks above:

- Scanner, manifests, profiles, grouping and saved reports: `just test ./internal/radar`.
- Remote collection, archives and transport: `just test ./internal/discovery`.
- Flags, input/output files, cancellation and exit codes: `just test ./cmd/tech-radar`.

Assert the finding or output exists before checking its properties. Tests should fail when the behavior disappears, not just when a forbidden result appears. Test stored data and user text separately from generated wording.

## Security checks

`just security` verifies the workflow dependency lockfile against GitHub and runs CodeQL's security-extended queries for Go and GitHub Actions. CodeQL downloads a checksum-verified bundle on first use, about 700 MB on Linux and Windows or 1.3 GB on macOS. Results go to `output/codeql/*.sarif`; CI uploads them to the repository's code scanning view. Query findings need review; the CLI exits successfully when analysis completes, even when it finds alerts.

Run `just actions-lock` after changing an action reference and review `.github/workflows/actions.lock`. Every remote action must belong to GitHub or this project and use a full commit SHA. The pre-commit check verifies lockfile coverage offline; `just actions-verify` also checks remote provenance and needs GitHub authentication. GitHub also validates the committed lockfile when starting workflows. Enterprise policy controls for requiring lockfiles are separate from these checks.

The generation recipe uses `--no-narrow --relock` to retain SHA references and rebuild the dependency records with matching keys. Leave the machine-generated lockfile intact.

## Contracts to preserve

- Generated files and runtime inputs don't become scan evidence. Source reads stay confined; archives reject traversal, unsafe entries and size-limit violations. Credentials and source values stay out of exports.
- All eligible evidence remains available. Unknown dependencies, unsupported syntax and skipped-file warnings stay visible.
- Profiles use the nearest component and respect development scopes. Grouping keeps component ownership and never extends approval.
- Decisions and exclusions survive replay and disappearing findings. Changed evidence or classification requests review; explicit policy still takes priority.
- Saved inputs rebuild offline. Keep old-format fixtures readable and byte-identical on re-export. A new rules version must not silently reinterpret an old context.
- Output locks, no-clobber writes and input/manual-edit protections stay intact, including failure and process-death paths. Never run target scripts, builds or hooks.
- Policy checks evaluate raw observations. Preserve deny precedence, scoped/version permissions, dated exceptions, baseline requirements, coverage gaps and CI exit codes.
- Radar changes preserve marker spacing, placement comparisons, theme controls, keyboard access and evidence links.

Evidence descriptions currently carry package/version/scope and wiring signals used by policy and assessment code. Preserve that stored format. Change human-facing wording in display functions; a structured-evidence migration needs its own compatibility work.

## Manual checks

The suite uses fake GitHub responses. For live collection:

1. Discover a public repo or an organization you can access.
2. Rebuild its context with `--from` and compare the output.
3. Generate a report, inspect the radar and follow finding/evidence links.
4. Check desktop/mobile widths, light/dark/system themes, keyboard tooltips and print output.

Run output-lock subprocess tests on each supported OS and filesystem. Cross-builds check compilation, not locking or writes, particularly on NFS/SMB. Avoid editing an export from another program while regenerating it. The two [accepted limitations](../reference/accepted-limitations.md) remain won't fix.

Compare representative reports before and after scanner changes, including findings, evidence, decisions and replay. Keep private fixtures and results outside the project.

Dynamic manifests, aliases, macros and deployment state can need manual review. Organization listings depend on credential access and can change during collection. Saved contexts contain findings and hashes, not source files or proof of authenticity. Commit each report's evidence companion to retain history for findings that disappear.

See [architecture](architecture.md) for parser/file limits and [portfolio discovery](../guides/discover.md) for GitHub limits.

For handbook changes, also follow [documentation verification](documentation.md#verify-a-change).
