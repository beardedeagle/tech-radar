# Tech Radar plugin

Inspect technology choices, review proposed groups and check policy with Codex or Claude Code. Both hosts load the same [review workflow](skills/review/SKILL.md) and bundled [input reference](skills/review/references/inputs.md).

Build the Go CLI from the tech-radar project and put it on PATH. Provide a repo path, organization or repo list, an output directory and any approved policy, exceptions or baseline. Keep private inputs and results outside the public package. The workflow presents proposed groups for review; it cannot grant approval.

For one Claude Code session:

```sh
claude --plugin-dir /path/to/plugin/tech-radar
```

Then invoke `/tech-radar:review`.

The source project includes local marketplaces. For Codex:

```sh
codex plugin marketplace add /path/to/tech-radar
codex plugin add tech-radar@tech-radar-local
```

For Claude Code:

```text
/plugin marketplace add /path/to/tech-radar
/plugin install tech-radar@tech-radar-local
```

Host manifests handle discovery. No hooks or remote server are bundled.
