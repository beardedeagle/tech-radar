---
title: "Scan coverage"
weight: 60
---

# Scan coverage

Files are read through Go's root-confined filesystem API. Symlinks, tests, dependency/build/generated directories, credential filenames, binary content and files over 2 MiB are skipped. Size and binary skips produce warnings.

The scanner reads:

- Literal Mix dependencies and matching direct Hex lock entries.
- npm dependency sections, Node engine constraints and matching package-lock entries.
- Direct Go module requirements and `.tool-versions` pins.
- Docker, Helm, Terraform and GitHub Actions definitions.
- Selected framework, platform, integration and documentation patterns.
- scc language and line counts for eligible files.

All scans also read literal Python, Ruby, PHP, Maven, Gradle, .NET, Cargo and plugin manifests. Unsupported dependency syntax produces warnings. Nothing evaluates build files, resolves dependency includes or runs repo code. scc's config loader and directory walker aren't used.

| Manifest | Supported observations | Limits |
| --- | --- | --- |
| `pyproject.toml`, requirements files | Literal project/Poetry declarations; development and conditional scopes | No includes, dynamic arrays or source resolution |
| `Gemfile` | Literal gem names/versions, groups and conditions | No Ruby evaluation or gemspec expansion |
| `composer.json` | `require` and `require-dev` | No Composer execution or transitive resolution |
| `pom.xml` | Project/profile dependencies with literal coordinates | No dependency management, plugins, inheritance or property resolution |
| `build.gradle`, `build.gradle.kts` | Literal Maven coordinates in direct dependency statements | No version catalogs, plugins, map notation or arbitrary build logic |
| `.csproj`, `.fsproj`, `.vbproj` | Package/framework references and Web SDK declaration | No imports, central version resolution, expressions or wildcard expansion |
| `Cargo.toml` | Literal dependency tables, inline tables, renamed crates and target conditions | No workspace inheritance or source resolution |

XML manifests reject directives and stop at 64 nested levels or 65,536 elements. Gradle nesting is limited to 64 levels. All readers use the existing 2 MiB file limit. Development and conditional declarations remain evidence; they don't establish an unrestricted runtime. Package presence alone doesn't establish deployment.

Lockfiles supply direct version evidence. They aren't scanned for custom reference patterns or included in code-volume counts. Transitive packages don't automatically become radar entries.

Source files are processed as they're read. Lockfiles are read again after declarations are collected; a changed hash stops the scan. Optional Git metadata lookup times out after five seconds. SIGINT and SIGTERM cancel remote discovery so temporary source can be removed.

Unknown dependencies keep their IDs, such as `hex:some_package` or `npm:some-package`, until the team classifies them. Source excerpts and raw configuration values aren't exported. Dependency source URLs are omitted when they aren't version strings.

Built-in framework syntax rules apply to their relevant source languages. For Go, the native parser removes comments and regexp pattern arguments from built-in matching. It follows direct parameter forwarding through named functions in the same file. Ordinary quoted environment keys, permissions and configuration values remain eligible. Custom rules retain their explicit line-matching behavior.

This is a static scan. The Go filter does not resolve cross-file wrappers, method calls, or every string assignment, and incomplete syntax receives best-effort filtering. Other languages do not receive this Go regexp analysis. Strings, aliases, macros and dynamic configuration can still need manual review. It doesn't inspect production, build a full call graph or produce an SBOM. JSON key locations can be approximate when a dependency appears in several sections.

## Detection, assessment and grouping

The maintained readers cover more technologies than the profile and grouping catalogs. An observed dependency can therefore appear in full evidence without appearing as a main radar entry. Its ID and source locations remain available for team review and explicit rules.

A scan does not prove absence. Unsupported syntax, ignored directories and limits can leave technologies unobserved. Size and binary skips produce recorded warnings; an unsupported construct that a reader does not recognize can remain an unreported coverage limit.

See [how findings become a radar](../concepts/how-it-works.md), [detection inputs](detection.md), and [grouping definitions](grouping.md).
