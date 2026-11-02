---
title: "Architecture"
weight: 10
---

# Implementation architecture

## Code layout

| Package or file | Responsibility |
| --- | --- |
| `internal/radar/scan.go` | Read eligible files, collect evidence and hash the source |
| `languages.go` | Use scc for language detection and line counts |
| `catalog.go`, `detection.go` | Built-in technologies and user-supplied rules |
| `synthesis.go`, `portfolio_profiles.go` | Match profiles and assess importance |
| `grouping.go`, `data/grouping.json` | Validate family definitions and group related findings |
| `policy.go` | Validate decisions and carry forward previous reports |
| `check_policy.go`, `check.go` | Validate explicit enforcement policy and evaluate observations/drift |
| `comparison.go` | Compare placements and preserve badges on unchanged replays |
| `document.go`, `render.go` | Render reports and read saved state |
| `chart.go`, `theme.go` | Marker tooltips, change symbols and report appearance |
| `portfolio.go`, `portfolio_manifests.go` | Save repo findings, read additional manifests and build contexts |
| `internal/discovery` | List GitHub repos, pin commits and read archives |
| `cmd/tech-radar` | Parse commands and safely read/write files |

## Follow a run through the code

`cmd/tech-radar` validates flags and runtime inputs, takes the output lock, and chooses the command. `internal/discovery` collects remote repositories when requested. Local and extracted repositories both pass through `internal/radar` file selection and scanning.

The scanner emits observations with relative paths, descriptions, source hashes and language counts. Profiles assess each observation in its nearest component. Grouping derives the main findings, while policy preserves team decisions and previous state. Rendering produces the document and full evidence JSON.

`check` evaluates the raw observations against its own enforcement policy. Presentation grouping and proposed rings do not participate in permission checks.

## Keep stored formats compatible

Descriptions and assessment strings appear in stored fingerprints. Some evidence descriptions also carry package, version, scope and wiring information used by policy checks. Change display labels in the rendering layer. A stored-format change needs a versioned reader and old-format fixtures.

Contexts rebuild from their saved observations under the original catalog version. Current reports apply current assessment rules and request review when a saved classification changes. Placement comparisons use family and component identity, so replacing a group's representative does not automatically create a new entry.

## Keep responsibilities separate

Manifest readers interpret literal declarations and retain scope. They do not evaluate target code or resolve dependency includes. Archive handling owns extraction safety and limits. The CLI owns output locking, protected inputs and publishing files. These checks serve different trust boundaries and must survive refactoring.

Run the [maintenance checks](testing.md) after changes. The [coverage reference](../reference/scanner.md) describes supported readers, and [files and history](../reference/files.md) describes the public write and replay contract.
