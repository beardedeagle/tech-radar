# tech-radar

Point it at a repo, review what it finds, and commit the radar with your code.

tech-radar builds a [Thoughtworks-style technology radar](https://www.thoughtworks.com/en-us/radar) from repository evidence. It identifies languages, frameworks, tools, platforms, and integrations, then proposes which choices belong on the main radar. Related findings share an entry. The full observations remain in a separate JSON file.

For a decision-led radar, `--model lifecycle` places technologies in Experiment, Adopt, Hold, or Retire using supplied lifecycle decisions. Candidates without a lifecycle decision remain visible as Needs decision. The default `recommendation` model keeps the proposed rings described above. See the [lifecycle walkthrough](https://beardedeagle.github.io/tech-radar/docs/guides/lifecycle/).

Teams record their decisions in explicit policy files and update the report through PRs. The CLI also collects several repositories into a shared checklist and checks observed use for policy violations and drift. A shared Codex and Claude Code plugin can investigate repository documentation and explicitly scoped Google Drive material through host integrations, then prepare proposed inputs for review.

## Build and run

Requires Go **1.26.7 or later** and [just](https://just.systems/). From the project root:

```sh
just build
./tech-radar generate --repo /path/to/repo --format html --out RADAR.html
```

Open `RADAR.html` in a browser. Hover or focus a marker for details and select its number to inspect the entry. The report follows system light or dark appearance and includes an explicit theme choice.

Commit the report and its linked `RADAR.evidence.<sha256>.json` file. The default recommendation view uses a radar in HTML and a matrix with findings in Markdown. Lifecycle defaults to a decision table. Use `--view radar|table` to choose either layout. JSON keeps the full report. `--self-contained` embeds evidence in Markdown or HTML when a one-file export is needed.

The [first-radar walkthrough](https://beardedeagle.github.io/tech-radar/docs/getting-started/first-radar/) creates a synthetic application, runs the scan, explains the proposed rings, and shows the files you should see. It needs no Node runtime, package installation, or GitHub access.

## Read the handbook

The [handbook](https://beardedeagle.github.io/tech-radar/) provides reading paths, worked examples, explanations, and reference:

| Task | Documentation |
| --- | --- |
| Understand findings, profiles, groups, and proposals | [How the radar is built](https://beardedeagle.github.io/tech-radar/docs/concepts/how-it-works/) |
| Build a lifecycle radar from supplied decisions | [Lifecycle walkthrough](https://beardedeagle.github.io/tech-radar/docs/guides/lifecycle/) |
| Record decisions and update a report | [Review and update](https://beardedeagle.github.io/tech-radar/docs/guides/review-and-update/) |
| Recognize internal names or group unfamiliar packages | [Detection and grouping](https://beardedeagle.github.io/tech-radar/docs/guides/detect-and-group/) |
| Collect local repositories or a GitHub organization | [Shared discovery](https://beardedeagle.github.io/tech-radar/docs/guides/discover/) |
| Enforce a standard and inspect drift | [Policy walkthrough](https://beardedeagle.github.io/tech-radar/docs/guides/check-policy/) |
| Use an agent to investigate the evidence | [Codex and Claude Code](https://beardedeagle.github.io/tech-radar/docs/guides/agents/) |
| Look up flags, schemas, limits, and replay behavior | [Reference](https://beardedeagle.github.io/tech-radar/docs/reference/) |
| Resolve errors or unexpected findings | [Troubleshooting](https://beardedeagle.github.io/tech-radar/docs/guides/troubleshooting/) |

## Review model and coverage

Proposed rings describe observed use and remain subject to team review. They do not establish approval or deployment. Display policy records decisions; the separate enforcement standard expresses machine-checkable permission.

The scanner reads supported manifests, matching direct locks, tool pins, language statistics, and selected source or configuration references. It skips tests, dependency and build trees, credential files, symlinks, binary content, and oversized files. It never runs target builds, scripts, or hooks. Local generation works offline; explicit GitHub collection needs network access and credentials. See [coverage](https://beardedeagle.github.io/tech-radar/docs/reference/scanner/) for readers and limits.

## Contribute and preview the docs

See [contributing](.github/CONTRIBUTING.md) for PR guidelines and the [code of conduct](.github/CODE_OF_CONDUCT.md) for community expectations. Report vulnerabilities privately through the [security policy](SECURITY.md).

Run `just setup` once to install pinned tools and Lefthook gates. `just ci` runs the checks used by GitHub Actions, including race tests, documentation and release builds, action dependency verification, and CodeQL. The [contributor guide](https://beardedeagle.github.io/tech-radar/docs/contributors/) covers setup and checks. [Releasing](https://beardedeagle.github.io/tech-radar/docs/contributors/releases/) explains how to publish a manually chosen version.

The handbook uses Hugo with Hugo Book in a separate documentation module. Preview it with:

```sh
just docs-serve
```

Open `http://localhost:1313/`. [Documentation development](https://beardedeagle.github.io/tech-radar/docs/contributors/documentation/) covers static builds and verification. Hugo is not required to run the CLI.

[Third-party notices](THIRD_PARTY_NOTICES.md) list the application's dependencies and attribution.
