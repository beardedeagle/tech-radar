---
title: "Enforcement policy"
weight: 50
---

# Check technology policy

`check` compares observed technology use with an explicit approved standard. It also compares observations with a saved baseline to report drift. The policy file is authoritative; the radar is a presentation of decisions, not permission inferred from a ring.

```sh
tech-radar check --repo /path/to/repo --policy /path/to/standard.json \
  --as-of 2026-11-01 --out check.json

tech-radar check --org example-org --policy /path/to/standard.json --out check.json
tech-radar check --repos repositories.json --policy /path/to/standard.json --out check.json
tech-radar check --from context.json --policy /path/to/standard.json --out check.json
```

Choose one source selector. Repeated `--repo` accepts several local repositories. Organization/list capture uses the same pinned GitHub archives and collection limits as `discover`; collection errors stop the run rather than checking a partial organization. `--from` checks a saved context or prior check result without refreshing source or membership. Private detection rules use `--rules` during collection.

Check output is JSON containing policy identity/version/fingerprint, evaluation date, per-repository violations, coverage gaps, drift, applied exception IDs and the full captured observations. Evidence paths are relative. The result can serve as the next baseline. It contains private repository and dependency metadata when the source does.

## Policy format

The enforcement format is separate from the schema-1 display policy accepted by `generate --policy`. A display policy's free-text scope, exclusions, approvals or ring cannot substitute for machine-enforceable permission. Do not pass a report's suggested placements as an enforcement policy.

`examples/standard.json` is a draft. It deliberately fails enforcement until an authorized reviewer supplies approved status, owner, decision reference and review date. The tool records that supplied approval; it does not authenticate the reviewer.

An approved policy looks like:

```json
{
  "kind": "tech-radar-policy",
  "schema_version": 1,
  "id": "example-standard",
  "version": "1",
  "default": "deny",
  "approval": {
    "status": "approved",
    "owner": "Engineering",
    "reference": "DEC-1",
    "reviewed": "2026-11-01"
  },
  "rules": [
    {"id":"react-ui","technologies":["react"],"effect":"allow","paths":["web"]}
  ]
}
```

`default` must explicitly be `deny` or `allow`. Use `deny` for a strict allowlist. Every detected observation is checked, including supporting and development findings omitted from the concise radar. Grouping does not hide or authorize members.

Each rule requires a unique ID, exact `technologies` and an `effect`:

- `allow` permits matching observations.
- `deny` prohibits matching observations, taking priority over allows.
- `existing-only` permits observation locations already present in a comparable baseline. A baseline is required; it is not inferred from the current checkout.

Optional `repositories` contains exact snapshot identities such as `local:my-repo` or `github:example-org/api`. Optional `paths` contains relative file/directory prefixes, with `.` meaning the whole repository. `web` matches `web/package.json`, not `web-old/package.json`. Omitting either restriction means all repositories/paths for that rule. Trial permissions can be limited to an experiment's repository/path; maintenance permissions can use `existing-only`.

Use finding IDs from evidence or `catalog`. A known technology ID such as `react` covers the package aliases recognized as that technology. A raw package ID such as `npm:react-dom` matches that package's declaration/resolution specifically; it does not authorize other aliases or generic source references. There are no implicit family permissions or wildcard technology selectors.

Several applicable allows are alternatives. At least one must satisfy its version and baseline conditions. A matching deny takes priority. If permission may apply but cannot be established, the observation is a coverage gap. Otherwise a default-denied or condition-failing observation is a violation. `reason` can explain the rule.

## Versions

Allow and existing-only rules may supply `versions`, a SemVer constraint such as `>=19.0.0 <20.0.0`, `^19.0.0` or an exact version. These use Masterminds SemVer semantics, including its prerelease rules. Deny rules are unconditional within their technology/repository/path selectors; express permitted versions through allow constraints and default deny.

A concrete `major.minor.patch` declaration, direct lock resolution or supported runtime tool pin can be checked. A leading `v` is accepted. Unresolved declarations such as `^19`, `~> 1.8`, source references and versions using another ordering scheme produce coverage gaps rather than guessed comparisons. A direct lock resolution can resolve a range only for the same package and manifest directory, and only when it satisfies the declaration. npm ranges and supported literal Hex requirements use their own semantics: Hex `~> 1.8` permits 1.x from 1.8 onward, while `~> 1.8.0` stops before 1.9. A stale lock or unsupported declaration grammar remains a gap. Source references use the nearest available declaration/tool-pin boundary. The check does not establish installed or deployed versions.

## Exceptions

Pass `--exceptions FILE` for reviewed, expiring permission overrides. They bind to the exact `policy_fingerprint` emitted by a check; policy edits require exceptions to be reviewed and rebound deliberately.

```json
{
  "kind": "tech-radar-exceptions",
  "schema_version": 1,
  "policy_fingerprint": "<digest from check.json>",
  "exceptions": [{
    "id": "migration-window",
    "technologies": ["react"],
    "effect": "allow",
    "repositories": ["local:my-repo"],
    "paths": ["legacy-ui"],
    "approval": {
      "status": "approved",
      "owner": "Engineering",
      "reference": "EX-1",
      "reviewed": "2026-11-01"
    },
    "expires": "2026-11-30"
  }]
}
```

Exceptions require explicit repository identities and can further limit paths and versions. An applicable, approved exception overrides ordinary rules, including a deny. It is active from its review date through its expiry date, inclusive. Expired, future-reviewed or unproven version-conditioned exceptions grant no permission. Policy approval cannot be later than the evaluation date.

`--as-of` defaults to the current UTC date. Supply it explicitly for repeatable CI results or reproducing an earlier check. The evaluation date is saved in the output.

## Drift and coverage

```sh
tech-radar check --repo /path/to/repo --policy standard.json \
  --baseline previous-check.json --as-of 2026-11-01 --out next-check.json
```

The baseline may be an intact check result or discovery context. Repository identities must match; give local list entries stable names when checkout directory names differ. Drift lists added/removed findings, changed observed versions and changed evidence. Those are observation changes, not proof of deployment or semantic changes. An approved addition is drift without a violation; unchanged use can violate a revised policy.

Existing-only permission checks technology, package identity, evidence kind and relative path against the baseline. It does not establish historical adoption outside that baseline. Missing baseline evidence cannot establish existing use. A baseline with different detection rules/capture version or recorded capture gaps is incomparable. A capture with no eligible files remains incomplete when reused as a baseline; a repository explicitly recorded as empty is a distinct known state. Repositories absent from the current selection are listed separately, not described as deleted from the organization.

Capture warnings, skipped eligible files, unresolved versions, missing evidence and incomparable required history produce coverage gaps. Static scans cannot discover every integration or evaluate arbitrary dynamic code. An unreported unsupported construct is still a possible coverage limit; a clean result is limited to eligible observations, not a universal compliance certificate. The agent workflow should record additional gaps it discovers during inspection.

## CI and output safety

| Exit | Meaning |
| --- | --- |
| 0 | No observed violations or recorded coverage gaps |
| 1 | Input, collection, validation or output failed |
| 2 | Violations found; inspect coverage gaps as well |
| 3 | Coverage gaps without confirmed violations |

`complete` is false whenever recorded gaps remain, including when exit 2 reports violations. Drift alone does not fail the command. Failed stdout/file writes return 1.

Outputs use existing locking, input protection and edit-detection safeguards. Use `--force` only to replace an intact generated result. A prior output can also be its own baseline: the output lock is held before it is read. Policy, exceptions, rules and repository-list inputs cannot be overwritten. Loaded policy/exception/baseline/list files are excluded from live capture by content hash. Keep outputs outside the target repository so subsequent scans do not include old generated artifacts.

Policies/exceptions are limited to 2 MiB; check/context files to 128 MiB. Policies allow at most 1,024 rules, each with 512 technology IDs, 1,024 repository identities and 256 path prefixes. Exceptions are limited to 256 entries. IDs, selectors and constraints are bounded; unknown fields and duplicate JSON keys fail validation. Equivalent key/selector/rule ordering and whitespace produce the same policy fingerprint.

For a complete local example with a baseline, added dependency, violation, and frozen replay, follow [check policy and drift](../guides/check-policy.md).
