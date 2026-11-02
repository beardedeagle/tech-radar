---
title: "CLI"
weight: 10
---

# CLI reference

```text
tech-radar generate --repo PATH [options]
tech-radar scan --repo PATH [options]
tech-radar discover (--repo PATH ... | --org NAME | --repos FILE | --from FILE) [options]
tech-radar check (--repo PATH ... | --org NAME | --repos FILE | --from FILE) --policy FILE [options]
tech-radar catalog
tech-radar version
```

Top-level `help`, `--help`, and `-h` print usage. Each main command accepts `--help`. Generate, scan, discover, and check reject unexpected positional arguments.

## Generate and scan

Both commands scan one local repository and produce the same report model. `generate` defaults to Markdown; `scan` defaults to full JSON.

| Flag | Meaning | Default |
| --- | --- | --- |
| `--repo PATH` | Required readable repository directory. | None |
| `--name NAME` | Report title, overriding the display policy's repository name. | Policy name, saved report name, or `Repository` |
| `--format FORMAT` | `markdown`, `html`, or `json`. | Markdown for generate; JSON for scan |
| `--out FILE` | Output path, or `-` for stdout. | `-` |
| `--policy FILE` | Display decisions, not an enforcement standard. | No new decisions |
| `--previous FILE` | Prior Markdown, HTML, or JSON report. | No prior state |
| `--rules FILE` | Additional detection rules. | Built-in detection |
| `--grouping FILE` | Grouping catalog overrides. | Built-in families |
| `--context FILE` | Validated discovery context. | No shared context |
| `--self-contained` | Embed full evidence in Markdown or HTML. | False |
| `--force` | Replace an intact generated output after validation. | False |

Markdown and HTML written to files use a compact document and adjacent evidence JSON by default. Stdout and JSON contain full state. `--self-contained` changes Markdown and HTML file output only.

Saved decisions, exclusions, and display numbers carry forward through `--previous`. Explicit current policy replaces a whole saved decision for each supplied ID. Required rules, grouping, and context inputs must be supplied again. [Files and history](files.md) describes replay and write behavior.

## Discover

Select exactly one input mode:

| Selector | Meaning |
| --- | --- |
| Repeated `--repo PATH` | Local repositories. Exact duplicate normalized paths fail. |
| `--org NAME` | Accessible repositories on github.com, including forks and archived repositories. |
| `--repos FILE` | Schema-1 list of local paths and GitHub repository identities. |
| `--from FILE` | Rebuild a saved context offline without refreshing it. |

Other flags are `--rules FILE`, `--out FILE`, and `--force`. Output defaults to stdout; a named output requires a `.json` extension. `--from` cannot be combined with `--rules` because it uses saved observations.

Repository-list paths are relative to the list file. Local display names default to directory basenames. Duplicate identities fail context validation; supply unique names when necessary. See [shared discovery](../guides/discover.md) for the list schema and credentials.

## Check

The same source selectors apply to `check`. Its `--from` accepts an intact discovery context or prior check result.

| Flag | Meaning | Default |
| --- | --- | --- |
| `--policy FILE` | Required approved enforcement standard. | None |
| `--exceptions FILE` | Approved expiring overrides tied to that standard's fingerprint. | None |
| `--baseline FILE` | Prior check or context for drift and existing-only rules. | No history |
| `--as-of YYYY-MM-DD` | Evaluation date for approvals and exceptions. | Current UTC date |
| `--rules FILE` | Additional detection during fresh collection. | Built-in detection |
| `--out FILE` | JSON output path, or `-` for stdout. | `-` |
| `--force` | Replace an intact generated export. | False |

`--from` cannot be combined with `--rules`. Named outputs require `.json`. A previous output may be its own baseline; the output lock is held before reading it.

| Exit | Meaning |
| --- | --- |
| `0` | No observed violations or recorded coverage gaps. |
| `1` | Input, collection, validation, or output failed. |
| `2` | Violations found. Gaps may also be present. |
| `3` | Coverage gaps without confirmed violations. |

Drift alone does not change the exit code. `complete` is false when recorded gaps remain. Failed stdout or file writes return `1`.

## Catalog and version

`catalog` prints built-in technology IDs, names, quadrants, and package aliases as JSON. It does not enumerate every dependency the scanner can observe. Unknown packages retain their ecosystem-prefixed IDs in evidence.

`version` prints the CLI and language counter versions. These commands do not scan a repository.

## Output and input paths

Named report outputs accept `.md`, `.html`, or `.json`. Discover and check require `.json`. Match the report extension to its chosen format for readers. The CLI protects source manifests, explicit inputs, human documents, and symlink destinations from output replacement. Parent output directories are created when needed. Existing output requires `--force` and an intact generated export.

See [input formats](display-policy.md), [enforcement policy](policy.md), and [output safeguards](files.md) for validation and size limits.
