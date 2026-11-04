---
title: "Review and update"
weight: 10
---

# Review and update a radar

A generated report starts the review. Use the evidence to decide which technologies deserve a place, what each placement means for this repository, and who owns the decision.

## Review the proposed entries

This page starts with the default `recommendation` model. For Experiment, Adopt, Hold, and Retire decisions, use the [lifecycle walkthrough](lifecycle.md).

Start with the main findings and inspect each entry's source locations in the linked evidence. Check grouped members as well as the representative. A shared vendor or similar name does not establish that independently owned systems should be reviewed together.

| Ring | Team intent |
| --- | --- |
| Adopt | Use within the recorded scope. |
| Trial | Try within a limited scope and review the results. |
| Assess | Investigate before committing to use. |
| Caution | Review the recorded concerns before proceeding. |
| Needs review | No placement is recorded or proposed. This is a report category, not a fifth ring. |

Recommendation rings do not have to be followed in order. In this model, record lifecycle states such as maintain, retire, or retired in the separate `lifecycle` field.

## Record a decision

Create a display policy file using IDs from the evidence report or `catalog`. This example records proposed choices for the application in the [first-run walkthrough](../getting-started/first-radar.md):

```json
{
  "schema_version": 1,
  "repository": "Example UI",
  "rules": {
    "react": {
      "ring": "Adopt",
      "scope": "This application's web UI",
      "rationale": "Used in application source and maintained by the UI team",
      "status": "proposed"
    },
    "typescript": {
      "ring": "Adopt",
      "scope": "Application TypeScript source",
      "rationale": "The team uses TypeScript for authored frontend code",
      "status": "proposed"
    }
  }
}
```

A proposed placement needs a ring, scope, and rationale. Once an authorized team review actually approves it, set `status` to `approved` and supply `owner`, `reference`, and `reviewed`. The reviewed date is `YYYY-MM-DD`. Do not invent those fields to make a draft look approved. The tool checks the record's shape; it cannot authenticate an approver.

A display policy can also set a name or quadrant, exclude a finding with a rationale, or introduce a manual candidate. [Display policy reference](../reference/display-policy.md) describes each field.

## Generate the update

Edit policy rather than the generated document. Pass the existing report as `--previous`:

```sh
./tech-radar generate --repo /path/to/application \
  --policy /path/to/team-policy.json \
  --previous /path/to/reports/RADAR.html \
  --format html --out /path/to/reports/RADAR.html --force
```

Supply the same `--rules`, `--grouping`, and `--context` files if the prior report used them. The command stops when a required input is missing. To remove added detection or grouping deliberately, supply an empty `patterns` or `families` array.

Current policy replaces the whole saved entry for each supplied ID. Entries omitted from the current file retain their saved decisions and exclusions. To change a saved decision, include that ID with the complete replacement you intend.

Changed evidence or classification keeps the decision and requests review. After the team reviews the change, update its decision reference or reviewed date. A finding that disappears stays recorded as **not detected** with its previous evidence and decision.

## Read placement changes

In radar view, an outer circle means a new radar entry. An arc means a ring or quadrant change, with details in the tooltip. A plain marker means unchanged placement or unavailable comparison history; the tooltip identifies which.

Table view records changes and review reasons in its rows. Use `--view table` to select it. An omitted view inherits a same-model previous report, otherwise recommendation defaults to radar and lifecycle defaults to table. View-only changes preserve decisions, evidence, and placement history.

Evidence changes alone do not count as placement movement. An unchanged regeneration preserves the existing comparison and badges. Switching models starts a fresh placement comparison, so a recommendation ring does not become lifecycle movement. A family can retain its comparison even when a different detected member becomes its representative.

Keep the evidence companion beside the document. Without it, saved decisions still carry forward, but older compact exports can lack inferred placement history and source locations for findings that disappeared.

## Submit the decision through a PR

Commit the updated document, its linked evidence file, and the reviewed policy inputs appropriate for that repository. Keep private organization inputs in their own authorized storage. Review the changed decisions, memberships, warnings, and missing findings before accepting the PR.

Direct edits to generated prose stop reimport so those edits cannot be silently discarded. Preserve an edited copy, express its intended decisions in policy, and regenerate from an intact prior report. See [files and history](../reference/files.md) for write protection and recovery cases.
