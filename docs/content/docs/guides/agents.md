---
title: "Use Codex or Claude Code"
weight: 50
---

# Agent workflow for Codex and Claude Code

Codex and Claude Code share one review workflow. The agent inspects repository evidence and proposes groups; the Go CLI scans, generates reports and checks policy. The package bundles no model API, server or hooks.

Build the CLI and put it on PATH before using the plugin:

```sh
go build -trimpath -o tech-radar ./cmd/tech-radar
```

The package is `plugin/tech-radar`. It contains a portable manifest, Codex/Claude compatibility manifests and one shared `review` skill. Version and identity are kept in sync across the manifests.

## Codex

From the project directory, register the local marketplace and install:

```sh
codex plugin marketplace add /path/to/tech-radar
codex plugin add tech-radar@tech-radar-local
```

Select Tech Radar in Codex and invoke its review skill. Provide the target repository or organization and an output directory. The skill is explicit-only in Codex so opening an unrelated repository does not start a technology review.

## Claude Code

Test the package in one session without installing it:

```sh
claude --plugin-dir /path/to/tech-radar/plugin/tech-radar
```

Invoke `/tech-radar:review`. For a persistent installation, add the local marketplace from Claude Code:

```text
/plugin marketplace add /path/to/tech-radar
/plugin install tech-radar@tech-radar-local
```

The shared skill is in the conventional `skills/review/SKILL.md` location. Claude's manifest is `.claude-plugin/plugin.json`; Codex has its own compatibility manifest and the portable root manifest. Host installation/cache behavior is separate from the Go binary; rebuild the binary after changing Go code.

## What the workflow does

Start with a repository, organization or repository list and an output directory. Supply any approved [enforcement standard](../reference/policy.md), exceptions and baseline. The agent collects evidence, investigates major choices and presents groups with source locations for review. It then generates the report using reviewed input files.

Interpretation can vary between runs; the saved inputs make CLI output reproducible. Keep private inputs and reports outside this public project. The bundled workflow in `plugin/tech-radar/skills/review/SKILL.md` covers handling untrusted repository text, reporting gaps and keeping proposed changes separate from approvals.

## Validation

```sh
claude plugin validate plugin/tech-radar --strict
claude plugin validate .claude-plugin/marketplace.json --strict
```

Codex discovers the local package through `.agents/plugins/marketplace.json`. Check discovery in the installed host. Manifest validation and Go tests do not test live agent behavior. A public listing is optional.

Host references: [OpenAI plugin packaging](https://developers.openai.com/plugins/build/plugins), [Claude plugin creation](https://code.claude.com/docs/en/plugins/create) and [Claude manifest reference](https://code.claude.com/docs/en/plugins/manifest-reference).

## What to give the agent

Supply the target, output location, existing radar, and any reviewed detection, grouping, display-policy, or enforcement inputs. State the task, for example, investigate unknown framework packages and propose groups, or explain violations under this standard.

Expect a review that names the main choices, cites finding IDs and source locations, explains proposed relationships, and retains uncertainty. The agent should produce proposed input files for review, then invoke the CLI with the reviewed versions. The original inventory and full evidence remain available throughout.

Use the same review process for an agent-authored grouping as for one written by a teammate. Model interpretation can vary. Saved reviewed input files make later CLI generation reproducible. [Detection and grouping](detect-and-group.md) explains those input roles, and [policy checks](check-policy.md) shows a deterministic enforcement run.
