---
title: "Detection rules"
weight: 30
---

# Detection rules

`scan`, `generate`, and fresh `discover` or `check` runs accept `--rules FILE`. Rules add recognition to the maintained catalog and built-in patterns.

```json
{
  "schema_version": 1,
  "patterns": [{
    "id": "example-client",
    "name": "Example client",
    "quadrant": "Platforms",
    "pattern": "\\bExampleClient\\b",
    "detail": "Example client reference",
    "documentation": false
  }]
}
```

## Pattern fields

| Field | Meaning |
| --- | --- |
| `id` | Stable technology ID, up to 128 bytes. |
| `name` | Display name up to 120 bytes, required for new IDs. |
| `quadrant` | One of the four radar quadrants, required for new IDs. |
| `pattern` | Go regular expression, 1 to 4,096 bytes. |
| `detail` | Sanitized evidence description, 1 to 240 bytes. |
| `documentation` | True selects eligible documentation instead of eligible code or configuration lines. Default false. |

Patterns run line by line after existing file selection and comment handling. They cannot make an excluded file eligible or evaluate target code. A custom rule still applies to supported manifests that have their own declaration reader. Lockfiles supply direct resolution evidence and are not scanned for custom reference patterns.

Known catalog IDs keep their maintained name and quadrant. Rules may omit those fields or repeat their exact values; conflicting definitions fail. New IDs require consistent names and quadrants across all their patterns.

## Validation and fingerprints

The file requires schema version `1` and a `patterns` array of at most 256 entries. Unknown fields, duplicate JSON keys, duplicate detection patterns, invalid IDs, conflicting metadata, invalid expressions, and expressions matching empty text fail validation.

The canonical fingerprint ignores JSON formatting and pattern order. Pattern text, descriptions, documentation mode, and effective custom definitions are part of the input's meaning. The report records that fingerprint, not executable pattern definitions.

The supplied file is excluded from scanning by its raw content hash. Output cannot overwrite it. Keep private names and matching conventions in an explicit team input outside the public tool source.

## Removing added rules

When a previous report records custom detection, updating requires `--rules` again. To remove those rules deliberately, supply:

```json
{"schema_version":1,"patterns":[]}
```

This does not disable built-in recognition. Changed custom classification requests review of affected saved decisions.

The CLI applies its ordinary input read limit of 16 MiB. Per-pattern limits still apply. See [the worked example](../guides/detect-and-group.md) and [scan coverage](scanner.md).
