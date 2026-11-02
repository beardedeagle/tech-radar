---
title: "Grouping"
weight: 40
---

# Group related findings

Grouping puts related findings under one radar entry. Every original ID, evidence reference and team decision stays in the full report. A group suggests what to review together; it doesn't approve the technology or claim every member has the same maturity.

The built-in catalog is `internal/radar/data/grouping.json`. It includes families for BEAM, React, Vue, Angular, Django, Rails, Laravel, Spring, ASP.NET Core, Tokio, cloud SDKs and common workflow tools. Language counting covers more languages than the maintained family catalog. Unrecognized findings remain in the evidence; a team can classify them through policy or add detection and grouping inputs.

## Add a family

The [worked grouping example](../guides/detect-and-group.md#group-unfamiliar-packages) supplies a complete family. The source project also includes `examples/grouping.json`. Pass a reviewed file with:

```sh
tech-radar generate --repo /path/to/repo \
  --grouping /path/to/team.grouping.json --out TECHNOLOGY_RADAR.md
```

Each family needs:

| Field | Meaning |
| --- | --- |
| `id` | Stable family identity |
| `name` | Display name |
| `quadrant` | One of the four radar quadrants |
| `significance` | `system_defining`, `workflow_enabling` or `implementation_support` |
| `relationship` | `framework_extensions`, `runtime_companions`, `platform_services`, `workflow_tools` or `sdk_modules` |
| `scope` | `component` or `repository` |
| `members` | Exact finding IDs, in representative preference order |
| `prefixes` | Package ID prefixes, including their ecosystem, such as `npm:@example-framework/` |

Supply members, prefixes or both. Use IDs from the evidence report; known packages may use catalog IDs rather than ecosystem-prefixed names. Exact member aliases normalize to those catalog IDs during validation. Prefixes match canonical finding IDs. NuGet IDs are lowercase. An unknown package can become a main finding through its family while retaining its raw ID and evidence.

The first detected member in preference order represents the family. If only prefixes match, the lexically first matching ID represents it. Family and component identity preserve placement comparisons when that representative changes. Older reports fall back to shared members where the comparison is unambiguous.

## Scope and decisions

Component scope uses manifest boundaries. Findings with evidence shared across several components stay separate when ownership is ambiguous. A development-only declaration in another component doesn't override an unrestricted declaration. Repository scope deliberately combines a shared platform, runtime or workflow across components. Choose it only when that is the intended review boundary.

An explicit decision on the representative or a member prevents the group from extending that decision to other findings. Exclusions still apply. Development-only framework members don't establish runtime importance. A group's proposal follows its strongest member use evidence, and all proposals still need team review.

## Override and replay

A runtime family with an existing ID replaces that entire built-in definition. To disable a built-in family:

```json
{"schema_version":1,"families":[{"id":"react","disabled":true}]}
```

An empty families array removes runtime overrides and uses the built-in catalog. Supply `--grouping` again when updating a report that used it. Reports record the effective catalog digest and a canonical input fingerprint. Whitespace, object key order, family order and prefix order don't change the fingerprint; member order matters because it chooses the representative. Grouping changes request review of saved approvals when their assessment changes.

Inputs are explicit. Nothing searches for private organization preferences automatically. The supplied grouping file is excluded from scanning by content hash and protected against output overwrite.

## Validation

Unknown fields, approval fields, duplicate keys, invalid IDs and ambiguous memberships fail before scanning. An exact member or overlapping prefix can't belong to two families. Disabled entries may contain only `id` and `disabled`, and must name an existing family.

Inputs are limited to 2 MiB and 256 runtime families. The effective catalog is limited to 256 families, 8,192 exact members and 2,048 prefixes. Each family can have at most 512 members and 128 prefixes. Names are limited to 120 bytes, family IDs to 128 bytes, and member IDs/prefixes to 256 bytes. Family definitions contain data only, with no executable expressions or radar approval fields.
