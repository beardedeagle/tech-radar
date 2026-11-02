---
title: "tech-radar"
weight: 0
---

# tech-radar

A technology radar records the choices a team is willing to use, where those choices apply, and what the team plans to investigate or retire. Keeping that document current is harder than drawing the circles. Dependencies change, integrations hide in configuration, and a single framework can appear as a dozen packages.

tech-radar reads a repository, collects evidence, and produces a proposed radar. Related findings share an entry so the main report stays readable. The complete observations live in a JSON evidence file beside it. Your team reviews the proposed placements, records its decisions, and commits both files through its normal PR process.

The same CLI can collect several repositories into a shared checklist and check observed technology use against an approved standard. A shared Codex and Claude Code plugin helps investigate relationships that the deterministic scanner does not understand on its own.

[See tech-radar scan itself](docs/guides/tech-radar-example/index.md) for a real report, its evidence, and the display decisions behind it.

## Choose a starting point

| You want to | Start here |
| --- | --- |
| Produce your first report and understand the output | [Generate a first radar](docs/getting-started/first-radar.md) |
| Understand findings, profiles, groups, and proposed rings | [How the radar is built](docs/concepts/how-it-works.md) |
| Record decisions and update an existing report | [Review and update](docs/guides/review-and-update.md) |
| Survey an organization or a repository list | [Build a shared checklist](docs/guides/discover.md) |
| Find policy violations and technology drift | [Check policy](docs/guides/check-policy.md) |
| Look up a flag, input field, or limitation | [Reference](docs/reference/_index.md) |
| Change the tool | [Contributor guide](docs/contributors/_index.md) |

## What you need to know first

The scanner reads files. It does not run the target's builds, scripts, hooks, or dependency installers. Local scanning and report generation work offline. Explicit GitHub collection needs network access and credentials.

A detected dependency establishes presence in the scanned files. Source references can strengthen that evidence, but neither proves production deployment or organizational approval. The report keeps those distinctions visible. Unknown findings remain in the evidence even when the main radar has no useful placement for them.

## Terms used in this handbook

| Term | Meaning |
| --- | --- |
| Finding | One observed technology, identified by a stable ID such as `react` or `npm:some-package`. |
| Evidence | A relative file path, line, kind, and description supporting a finding. |
| Profile | A checklist selected from the languages and application patterns in a component. |
| Group | Related findings presented as one radar entry. Each member keeps its original evidence. |
| Display policy | Team decisions about names, quadrants, rings, scope, exclusions, and review status. |
| Enforcement standard | An approved allow, deny, or existing-only policy evaluated by `check`. |
| Context | Frozen observations from one or more repositories, with a derived shared checklist. |
| Baseline | An earlier context or check result used to compare observations and existing use. |

Read [the first-run walkthrough](docs/getting-started/first-radar.md) for a complete example. It uses fictional application files and produces a report you can open locally.
