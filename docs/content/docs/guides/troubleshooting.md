---
title: "Troubleshooting"
weight: 60
---

# Troubleshooting

Start with the command's error and the input or output path it names. Preserve your existing report before trying a recovery step.

## A file already exists

An existing destination requires `--force`, and forced replacement requires an intact generated export. Human documents and edited reports cannot be overwritten this way.

For an intended update, pass the intact report as `--previous` and regenerate with the reviewed policy. For an edited document, keep its edits and move intended decisions into policy first. Use a new output filename if you need both versions.

If the error says the output changed during generation, another program modified it after the run began. Keep that version and review the conflict. Do not bypass the protection.

## Another run is writing the output

The `.lock` file coordinates active writers. Its presence alone does not mean a process still holds the lock; the operating system releases the lock when the handle closes or the process ends.

Wait for the active run to finish and retry. Do not delete the lock file while another run may be using it. Two reports can also contend while creating a shared evidence companion.

## Updating asks for rules, grouping, or context

The prior report recorded those inputs. Pass them again so a missing file cannot silently change classification. An empty `patterns` or `families` array deliberately removes runtime detection or grouping overrides. Replacing or changing context requires the corresponding saved or newly collected context.

## A saved context does not match its rebuilt checklist

The file may have been edited, truncated, or produced under an unsupported format or generator version. Keep it. Use an intact saved copy with the matching version, or collect again from the repositories. Hashes detect inconsistency; they cannot reconstruct missing source evidence.

`discover --from` replays saved observations. It does not recover omitted files, add newly supported readers, or refresh membership.

## The radar has fewer entries than the evidence

The human report highlights architectural and workflow choices and groups related findings. Supporting and unknown findings remain in full JSON. Inspect that file before treating an omitted entry as missing detection.

Use explicit detection for unfamiliar names, grouping for established relationships, and display policy for team decisions. A manually added technology remains not detected until supported evidence establishes it.

## A ring or change symbol looks unexpected

Suggested Adopt requires established-use evidence. Configuration can suggest Trial; declarations can suggest Assess. A proposal records the scanner's heuristic, not your team's decision. Override it through reviewed display policy when the team's actual choice differs.

Every marker is new on a first report. New means new to this radar, not recently adopted. Evidence changes do not necessarily move a placement. Plain markers can mean unchanged placement or missing comparison history; inspect the tooltip and keep the evidence companion available.

## Policy checks return 2 or 3

Exit `2` means observed violations and may include gaps. Exit `3` means gaps without confirmed violations. Read `violations`, `coverage_gaps`, and `drift` separately.

For unknown versions, inspect concrete declarations, tool pins, and direct lock entries in the same component. Unsupported ranges or stale locks remain gaps. For existing-only rules, provide a comparable baseline. A different rules fingerprint, capture version, or incomplete baseline prevents a reliable comparison.

Check that you supplied an approved enforcement standard, not a display policy. A draft standard or incomplete approval record fails validation. Use an explicit evaluation date when reproducing exception or approval behavior. Never invent approval metadata or change a baseline to suppress a finding.

## GitHub collection fails

Check github.com credential access to the requested repositories. Organization membership is limited to what those credentials can see. Inaccessible or failed requested repositories stop collection instead of returning a partial result.

A repository explicitly recorded as empty differs from a captured repository with no eligible files. The latter has unknown coverage. Enterprise hosts are unsupported. See [collection limits](discover.md#github-access-and-limits) before retrying archive or response-limit failures.

For a reproducible report, include the CLI version, exact command with private paths redacted, input schema versions, and the error. Keep private rules, source, credentials, and company reports out of public issues.
