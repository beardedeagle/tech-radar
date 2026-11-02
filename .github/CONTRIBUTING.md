# Contributing to tech-radar

I welcome ideas, issues and pull requests. A couple of ground rules:

- Keep the change focused and explain what it fixes.
- Format and document code where applicable.
- Add tests that prove the behavior. Coverage numbers aren't the goal.
- Run `just ci` before pushing. Fix formatting with `just fmt`.

Run `just setup` once to install the pinned tools and local hooks. You'll also need Go, just, GitHub CLI and jq; the [contributor guide](https://beardedeagle.github.io/tech-radar/docs/contributors/testing/) covers setup and checks.

Changes to `main` go through PRs. Branch protection requires the Linux, macOS, Windows, Current Go, Automation and Security checks to pass, with the branch up to date before merging. Resolve review conversations and use a squash or rebase merge. Deleting `main` or force-pushing its history is blocked, subject to the repository's configured bypass permissions.

All repo automation belongs in the Justfile. Lefthook and GitHub Actions use those recipes. Remote actions must be owned by GitHub or beardedeagle and pinned to full commit SHAs. Run `just actions-lock` after updating action references, review the generated lockfile, then run `just actions-verify`.

For dependency updates, check any matching version constants and the [release version declarations](https://beardedeagle.github.io/tech-radar/docs/contributors/releases/). Preserve saved reports and contexts; a version bump must not silently break replay. Dependabot opens update PRs for both Go modules and GitHub Actions. Tool versions pinned only in the Justfile still need a manual update.

Use small, public or synthetic examples in issues and tests. Keep private repository names, credentials and generated evidence out of this repo. Report vulnerabilities through the [security policy](../SECURITY.md).

Participation follows the [code of conduct](CODE_OF_CONDUCT.md). Questions can go through the [question form](https://github.com/beardedeagle/tech-radar/issues/new?template=question.yml) or randy@heroictek.com.
