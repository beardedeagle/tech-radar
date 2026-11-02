---
title: "Files and history"
weight: 70
---

# Files and history

The default file export separates the human document from the full evidence:

| File | Contents |
| --- | --- |
| `RADAR.md` or `RADAR.html` | Main findings, team decisions, comparison state, and evidence link. |
| `RADAR.evidence.<sha256>.json` | Full findings, evidence, decisions, supporting observations, and language counts. |
| `<output>.lock` | Stable file used to coordinate writers. It is not the evidence or a policy input. |

The evidence filename includes its full SHA-256. Evidence is written first so a failed export cannot leave a document linking to an unwritten file. Existing evidence must match byte for byte. Old evidence is not deleted automatically.

JSON and stdout contain full report state. `--self-contained` embeds that state in Markdown or HTML for a one-file export.

## Saved reports

Markdown and HTML carry a saved snapshot and body checksum. The snapshot preserves decisions, exclusions, display numbers, review state, and displayed placements. The checksum detects direct edits to generated prose and stops reimport instead of discarding them.

The adjacent evidence file retains source locations and history for findings that disappear. A missing companion permits decision replay; a present companion with a bad hash stops it. Older compact exports may lack inferred placement history without their companion.

Schema-1 reports and full self-contained exports remain readable. Current reports use schema 2. Newly recognized package aliases retain their old raw package IDs when those IDs already exist in saved reports, including their decisions and exclusions.

## Contexts and check results

Contexts freeze repo identities, revisions, observations, hashes, warnings, and a derived checklist. `discover --from` validates and re-exports those frozen observations without contacting GitHub or reading the original checkout.

Current captures use `portfolio-3`. Saved `portfolio-1` and `portfolio-2` contexts rebuild under their original assessment rules and re-export unchanged. Re-export is not an upgrade. Fresh collection applies current rules.

Check results retain captured observations and an edit-detection digest, so they can serve as a baseline or frozen `check --from` input. Drift comparisons require comparable detection inputs and capture versions. Capture both revisions with matching versions when evaluating a tool upgrade.

## Output replacement

New outputs refuse to overwrite an existing destination. On filesystems supporting the operation, a hard link publishes the staged file with no clobber. `--force` can replace an intact generated export only after validating its original identity and content.

The CLI protects source manifests, explicit inputs, human documents, and symlink output paths. It takes the output lock before reading saved state and holds it through generation and publication. A run that cannot take a needed lock stops; retry after the other writer finishes.

Lock files remain after release so later writers use the same file object. Never remove one while a run may use that output. Ignore the particular generated lock filenames in version control if needed. Do not ignore all dependency lockfiles.

Other editors can ignore the lock. The final check cannot prevent every save racing with replacement. Avoid editing an export during regeneration, or choose a distinct output filename. The [accepted limitations](accepted-limitations.md) document the two remaining cases and their disposition.

## Reproducibility and privacy

Equivalent eligible files, Git revision, explicit inputs, previous state, and tool versions produce equivalent output. Absolute checkout paths, creation order, modification times, and scan timestamps do not enter the radar. Use an explicit `--as-of` date for repeatable checks because approval and exception dates affect enforcement.

The checkout is not locked while scanning. Lockfiles are read after declarations and their captured hashes are checked again; a changed lockfile stops the scan. Use a stable checkout for reproducible collection.

Input size limits are 16 MiB for report, display-policy, detection, and repository-list reads; 2 MiB for grouping and enforcement policy or exceptions; and 128 MiB for context and check-result reads. The relevant format's field and collection limits also apply.

Exports omit absolute local paths, raw source excerpts, and credential values. Repository identities, relative paths, dependency names, decision rationale, and ownership metadata can still be private. Store the report and its inputs with the appropriate repository or authorized policy storage.
