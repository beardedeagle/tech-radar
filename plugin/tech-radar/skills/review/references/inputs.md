# Runtime inputs

The CLI validates these separate formats. Detection and grouping do not grant permission; `check` requires an approved enforcement standard.

## Detection rules

`scan`, `generate` and live `discover`/`check` accept explicit `--rules FILE`. Existing IDs retain catalog names/quadrants. New IDs need a name and quadrant.

```json
{
  "schema_version": 1,
  "patterns": [{
    "id": "example-client",
    "name": "Example client",
    "quadrant": "Platforms",
    "pattern": "\\bExampleClient\\b",
    "detail": "Example client reference"
  }]
}
```

Patterns use Go regular expressions over eligible lines and add to built-in detection. Use descriptions without source values or credentials. Unsupported files and matches outside scan coverage remain gaps.

## Grouping

`scan` and `generate` accept explicit `--grouping FILE`. Use actual finding IDs from the current evidence. Example:

```json
{
  "schema_version": 1,
  "families": [{
    "id": "example-framework",
    "name": "Example framework",
    "quadrant": "Languages & Frameworks",
    "significance": "system_defining",
    "relationship": "framework_extensions",
    "scope": "component",
    "members": ["npm:example-core", "npm:example-router"]
  }]
}
```

Quadrants: `Techniques`, `Platforms`, `Tools`, `Languages & Frameworks`.

Significance: `system_defining`, `workflow_enabling`, `implementation_support`.

Relationships: `framework_extensions`, `runtime_companions`, `platform_services`, `workflow_tools`, `sdk_modules`.

Scope: `component` or `repository`. Framework ownership usually needs component scope. Exact member order chooses the preferred detected representative. Optional `prefixes` matches canonical package IDs. Definitions overlay complete built-in families by ID; `{"id":"react","disabled":true}` disables that family. Overlapping ownership fails validation.

## Display decisions

The `--policy` input to `generate` records display decisions, with schema version 1 and a `rules` object keyed by finding ID. A proposed choice needs ring, scope and rationale. Approved choices additionally require status, owner, reference and reviewed date. Excluding a display finding does not exempt it from enforcement.

## Enforcement standard

The `--policy` input to `check` is a different format. Draft it as proposed and leave approval metadata unset until the user supplies actual authorization:

```json
{
  "kind": "tech-radar-policy",
  "schema_version": 1,
  "id": "example-standard",
  "version": "1",
  "default": "deny",
  "approval": {"status":"proposed","owner":"","reference":"","reviewed":""},
  "rules": [{"id":"example-ui","technologies":["react"],"effect":"allow","paths":["web"]}]
}
```

Proposed standards fail enforcement. Approved metadata requires `status: approved`, an owner, reference and `reviewed: YYYY-MM-DD`. The tool cannot verify approval authority.

Effects: `allow`, `deny`, `existing-only`. Default must explicitly be `allow` or `deny`. Deny wins over ordinary allows. Optional `repositories` uses exact identities from captured snapshots; `paths` uses relative file/directory prefixes. Optional `versions` is a SemVer constraint for allow/existing-only rules. A concrete supported pin is required; unresolved ranges or other ordering schemes remain gaps. A lock resolution must match the package/component and demonstrably satisfy the declaration; stale locks cannot grant permission. Existing-only requires a comparable baseline containing the technology/package at the same evidence path/kind.

Technology IDs are exact, with no implicit family approval or wildcard selectors. Canonical technology IDs cover their recognized aliases; raw package IDs match only that declaration/resolution. Use explicit scoped permissions to express a Trial experiment or maintenance restriction. Do not infer those permissions from a ring.

## Approved exceptions

A separate `--exceptions` file requires kind `tech-radar-exceptions`, schema version 1, the exact current `policy_fingerprint` and an `exceptions` array. Each exception is an allow rule with explicit repository identities, approval metadata and `expires: YYYY-MM-DD`. Optional paths/versions can narrow it further. It can override a deny only while active, from its actual review date through expiry inclusive. Create one only with explicit authorization.

## Updates and checks

Preserve reviewed inputs. Supply rules/grouping/context again when a prior radar used them. Baselines for `check` are intact generated check JSON or discovery context JSON. Use a stable explicit `--as-of` date for reproducibility. Results cover the saved static observations; they do not establish deployment or universal compliance.
