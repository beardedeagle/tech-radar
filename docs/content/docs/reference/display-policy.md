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
| `repository` | Optional report name. `--name` takes priority. |
| `rules` | Object of finding IDs and decisions. |

## Decision fields

| Field | Meaning |
| --- | --- |
| `name` | Optional display name. Required with a quadrant for a new manual ID unknown to the catalog. |
| `quadrant` | `Techniques`, `Platforms`, `Tools`, or `Languages & Frameworks`. |
| `ring` | `Adopt`, `Trial`, `Assess`, or `Caution`. |
| `scope` | Description of where the placement applies. Required for a placement. |
| `rationale` | Reason for the decision. Required for a placement or exclusion. |
| `status` | `proposed` or `approved`. A supplied ring with omitted status becomes proposed. |
| `owner` | Recorded decision owner. Required for approval. |
| `reference` | Decision or review reference. Required for approval. |
| `reviewed` | Date in `YYYY-MM-DD` format. Required for approval. |
| `lifecycle` | Separate free-text lifecycle, for example maintain, retire, or retired. |
| `exclude` | True omits this finding from the radar while retaining evidence and the decision. |

An exclusion cannot also recommend a ring or claim approval. Unknown fields, duplicate JSON keys, invalid rings or quadrants, and incomplete decisions fail validation.

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

A current entry replaces the whole saved decision for its ID. Omitted entries carry forward. Changed evidence or classification retains the decision and requests review. A new review date or reference acknowledges the team's subsequent review.

The CLI records supplied approval metadata without authenticating its authority. Review this file through the same process used to establish the team's standard.
