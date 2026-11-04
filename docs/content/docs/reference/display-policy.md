---
title: "Display policy"
weight: 20
---

# Display policy

`generate --policy` and `scan --policy` accept a schema-1 JSON object whose `rules` map is keyed by finding ID. This format records presentation and review decisions. `check --policy` uses a separate [enforcement format](policy.md).

```json
{
  "schema_version": 1,
  "repository": "Example application",
  "rules": {
    "react": {
      "ring": "Adopt",
      "scope": "Application web UI",
      "rationale": "Reviewed framework choice for this application",
      "status": "approved",
      "owner": "Example UI team",
      "reference": "Example decision 1",
      "reviewed": "2026-11-01"
    }
  }
}
```

The approval fields above are illustrative. Real approval records identify the actual owner, review, and decision reference.

## Top-level fields

| Field | Meaning |
| --- | --- |
| `schema_version` | Required value `1`. |
| `model` | Optional `recommendation` or `lifecycle`. It must agree with an explicit `--model`. When omitted, generation inherits the previous report model or uses `recommendation`. |
| `repository` | Optional report name. `--name` takes priority. |
| `rules` | Object of finding IDs and decisions. |

## Decision fields

| Field | Meaning |
| --- | --- |
| `model` | Optional source model, `recommendation` or `lifecycle`. A policy decision must match the selected report model. Saved decisions record it when carried across models. |
| `name` | Optional display name. Required with a quadrant for a new manual ID unknown to the catalog. |
| `quadrant` | `Techniques`, `Platforms`, `Tools`, or `Languages & Frameworks`. |
| `ring` | Recommendation placement, `Adopt`, `Trial`, `Assess`, or `Caution`. Lifecycle placement uses `lifecycle`. |
| `scope` | Description of where the placement applies. Required for a placement. |
| `rationale` | Reason for the decision. Required for a placement or exclusion. |
| `status` | `proposed` or `approved`. A supplied ring or canonical lifecycle placement with omitted status becomes proposed. |
| `owner` | Recorded decision owner. Required for approval. |
| `reference` | Decision record, such as a repository docs path, ADR identifier, PR URL, or Drive document URL. Required for recommendation approval. Lifecycle permits it to be missing and records the review gap on subsequent generations. |
| `reviewed` | Date in `YYYY-MM-DD` format. Required for approval. |
| `lifecycle` | In recommendation mode, separate free text such as maintain, retire, or retired. In lifecycle mode, `Experiment`, `Adopt`, `Hold`, or `Retire`, matched case-insensitively and saved with canonical spelling. |
| `exclude` | True omits this finding from the radar while retaining evidence and the decision. |

An exclusion cannot also recommend a ring, supply a canonical lifecycle placement in lifecycle mode, or claim approval. Unknown fields, duplicate JSON keys, invalid rings or quadrants, and incomplete decisions fail validation.

## Lifecycle decisions

Set top-level `"model": "lifecycle"` or pass `--model lifecycle`. A proposed lifecycle placement needs `lifecycle`, `scope`, and `rationale`. Approval also needs `status: approved`, `owner`, and a valid `reviewed` date. Lifecycle permits a missing `reference` for both statuses. The CLI records supplied metadata, and the team establishes approval authority.

Without `--previous`, missing decision material creates no missing-material review gap. Other review reasons still apply, including proposed status, absent lifecycle placement, and source absence. With any `--previous`, every included technology without a nonblank `reference` has `missing_decision_material: true` and requires review. This applies to unchanged input and to a prior recommendation report. Excluded findings retain their evidence and decisions outside this review set.

The CLI consumes the supplied reference without fetching it. Check that the cited record actually supports the lifecycle state and scope. A link alone establishes neither adoption nor authority. See the [worked lifecycle example](../guides/lifecycle.md).

## Exclusions

```json
{
  "schema_version": 1,
  "rules": {
    "shell": {"exclude": true, "rationale": "Reviewed as local helper scripts"}
  }
}
```

Excluded languages remain in language statistics. Exclusion from the radar does not exempt an observation from enforcement.

## Manual candidates

A known catalog ID can be proposed even when undetected. A new ID needs a name and quadrant:

```json
{
  "schema_version": 1,
  "rules": {
    "example-queue": {
      "name": "Example queue",
      "quadrant": "Platforms",
      "ring": "Assess",
      "scope": "Potential background work",
      "rationale": "Investigate queue ownership and delivery requirements"
    }
  }
}
```

It remains **not detected** and has no invented evidence. Manual candidates do not expand scanner recognition; [detection rules](detection.md) do that.

## Replay

A current entry replaces the whole saved decision for its ID. Omitted entries carry forward. Changed evidence or classification retains the decision and requests review. A new review date or reference acknowledges the team's subsequent review. In lifecycle mode, clearing either field does not acknowledge an evidence change, and a missing reference still creates the subsequent-generation gap.

An explicit model switch preserves supplied decisions with their source model and resets placement comparison. The document labels historical status, rationale, scope, lifecycle notes and supplied ownership and review records separately from active placement. A recommendation lifecycle note remains historical even when its text matches a lifecycle state. An inferred recommendation has its own suggested status and rationale.

A switch to recommendation requires explicit policy replacements for carried approvals. A switch to lifecycle requires replacements for carried approvals with canonical-looking lifecycle notes. Recommendation approvals without such a note can remain historical and unplaced. Each replacement supplies the whole target-model decision. Recommendation Trial, Assess, and Caution never translate into lifecycle states.

The CLI records supplied approval metadata without authenticating its authority. Review this file through the same process used to establish the team's standard.
