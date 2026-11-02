---
title: "How the radar is built"
weight: 10
---

# How the radar is built

A package list is useful evidence, but it makes a poor technology radar on its own. A web framework can bring several companion packages. A repository can also contain helper scripts, development dependencies, a frontend, and a backend with different architectural roles.

tech-radar handles these as separate steps:

```text
repository files
    -> findings and source evidence
    -> component profiles and importance assessment
    -> related findings grouped for presentation
    -> proposed radar placements
    -> team decisions recorded in display policy
```

Policy enforcement is a second use of the same observations:

```text
current observations + approved standard + optional baseline
    -> violations, coverage gaps, and technology drift
```

## Findings establish what was observed

The scanner reads supported manifests, matching direct dependency locks, tool pins, and selected source or configuration references. scc identifies languages and counts lines in eligible files.

A finding keeps its ID across runs. Known aliases map to a catalog technology, while unknown packages retain IDs such as `npm:some-package`. Every eligible occurrence remains available in full evidence. The report does not discard unknown dependencies just because it cannot place them intelligently.

Descriptions are sanitized observations, not copies of source lines. A path and line let a reviewer inspect the original file. Fingerprints identify the captured files and normalized evidence; they do not prove who supplied the source.

## Profiles put findings in context

A profile is a review checklist for a language and application pattern. A React component needs different questions from a BEAM application, a Go module, or an infrastructure repository. Profiles are selected from local evidence and applied at the nearest matching component.

The assessment separates importance from confidence. A platform or persistence technology can matter architecturally even when the evidence only establishes a declaration. A development helper can be well established while remaining supporting code.

| Importance | What it describes |
| --- | --- |
| Core architecture | Runtime, domain, data, platform, or application choices that shape the component. |
| Development and operations | Delivery, quality, observability, and operational workflows. |
| Supporting code | Dependencies or helpers that do not establish a primary architectural role. |
| Needs investigation | Observations for which the available rules cannot establish a useful role. |

The checklists are maintained candidates. Detecting a Go module does not prove that it runs an HTTP service. Detecting a package does not prove that it is enabled in production.

## Grouping makes the main report readable

A versioned catalog groups runtime companions, framework extensions, SDK modules, platform services, and workflow tools. It respects component ownership. Ambiguous shared findings remain separate.

The main report shows a representative and its members. Original IDs and evidence remain in the full report. A family's proposed placement follows its strongest use evidence, so the proposal does not claim every extension has equal maturity.

A decision on a member or representative prevents grouping from extending that decision to other findings. Exclusions still apply to their own IDs. Use [explicit grouping definitions](../reference/grouping.md) for relationships the maintained catalog does not know.

## Proposed placement and approval have different jobs

Adopt, Trial, Assess, and Caution describe a technology's intended use within a scope. The scanner can propose a placement from evidence. Only your recorded policy supplies the team's decision and approval metadata.

Established source use can suggest Adopt. Configuration can suggest Trial. Declarations or references can suggest Assess. These heuristics are visible suggestions for review. The tool cannot infer that your team has approved an unfamiliar package merely because it appears in many repositories.

Lifecycle is recorded separately. A team can maintain an existing deployment while discouraging new use. The [enforcement standard](../reference/policy.md) expresses that restriction with explicit repository, path, version, or existing-only conditions.

## Context carries a checklist between repositories

Discovery freezes observations from local repositories, a repository list, or a GitHub organization. It derives a shared catalog and profiles without turning popularity into importance. Each later scan still has to establish a technology's role in its own component.

The saved context supports offline replay. It does not carry executable private detection patterns or refresh membership automatically. See [shared discovery](../guides/discover.md).

## What a clean check means

`check` evaluates raw observations, including details omitted from the main radar. Drift reports changed observations; violations report conflict with the supplied standard. Either can occur without the other.

A successful check means no observed violations or recorded coverage gaps within the static scan. It is not a production inventory or proof that every integration was discovered. Unsupported syntax, unobserved files, and deployment state still need review. The [coverage reference](../reference/scanner.md) describes those limits.
