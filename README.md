# tech-radar

Point it at a repo, review what it finds, and commit the radar with your code.

tech-radar builds a [Thoughtworks-style technology radar](https://www.thoughtworks.com/en-us/radar) from repository evidence. It identifies languages, frameworks, tools, platforms, and integrations, then proposes which choices belong on the main radar. Related findings share an entry. The full observations remain in a separate JSON file.

Teams record their decisions in explicit policy files and update the report through PRs. The CLI also collects several repositories into a shared checklist and checks observed use for policy violations and drift. A shared Codex and Claude Code plugin can investigate unfamiliar relationships and prepare proposed inputs for review.

## Build and run

Requires Go **1.26.4 or later**. From the project root:

```sh
go build -trimpath -o tech-radar ./cmd/tech-radar
./tech-radar generate --repo /path/to/repo --format html --out RADAR.html
```

Open `RADAR.html` in a browser. Hover or focus a marker for details and select its number to inspect the entry. The report follows system light or dark appearance and includes an explicit theme choice.

Commit the report and its linked `RADAR.evidence.<sha256>.json` file. Markdown output uses a matrix and findings table. JSON keeps the full report; `--self-contained` embeds evidence in Markdown or HTML when a one-file export is needed.

The [first-radar walkthrough](https://beardedeagle.github.io/tech-radar/docs/getting-started/first-radar/) creates a synthetic application, runs the scan, explains the proposed rings, and shows the files you should see. It needs no Node runtime, package installation, or GitHub access.

## Read the handbook

The [handbook](https://beardedeagle.github.io/tech-radar/) provides reading paths, worked examples, explanations, and reference:

| Task | Documentation |
| --- | --- |
| Understand findings, profiles, groups, and proposals | [How the radar is built](https://beardedeagle.github.io/tech-radar/docs/concepts/how-it-works/) |
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

Run `go test -race ./...` and `go vet ./...` for application changes. The [contributor guide](https://beardedeagle.github.io/tech-radar/docs/contributors/) explains architecture, compatibility, and checks.

The handbook uses Hugo with Hugo Book in a separate documentation module. With Go 1.27 or later, run the pinned builder:

```sh
go run github.com/gohugoio/hugo@v0.169.0 server --source docs --bind 127.0.0.1
```

Open `http://localhost:1313/`. [Documentation development](https://beardedeagle.github.io/tech-radar/docs/contributors/documentation/) gives the pinned builder command, static build, and verification steps. Hugo is not required to run the CLI.

[Third-party notices](THIRD_PARTY_NOTICES.md) list the application's dependencies and attribution.
