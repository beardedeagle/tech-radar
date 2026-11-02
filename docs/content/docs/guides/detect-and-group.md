---
title: "Add detection and grouping"
weight: 20
---

# Add detection and grouping

Detection and grouping solve different problems. A detection rule finds additional evidence for a technology. A grouping definition puts already observed technologies under one main radar entry. Team policy records decisions about those findings.

Use explicit input files so a repository can carry its reviewed choices without changing the public scanner. The examples below use the temporary directory from [your first radar](../getting-started/first-radar.md).

## Recognize a project-specific name

Suppose your application uses a client named `ExamplePayments`. A maintained catalog cannot know every internal name. Add a source reference to the demo:

```sh
cat > "$radar_demo/app/src/payments.ts" <<'APP'
const ExamplePayments = { charge: () => Promise.resolve() };
ExamplePayments.charge();
APP
```

This is synthetic source for the scan example. It does not implement an external payment integration.

Create an additional recognition rule:

```sh
cat > "$radar_demo/payments.rules.json" <<'RULES'
{
  "schema_version": 1,
  "patterns": [{
    "id": "example-payments",
    "name": "Example payments client",
    "quadrant": "Platforms",
    "pattern": "\\bExamplePayments\\.charge\\(",
    "detail": "ExamplePayments client call"
  }]
}
RULES
```

Run the scan with that file:

```sh
./tech-radar scan --repo "$radar_demo/app" \
  --rules "$radar_demo/payments.rules.json" \
  --out "$radar_demo/reports/payments-inventory.json"
```

The result includes `example-payments` with a source location in `src/payments.ts`. The description records what matched; it does not prove deployment or account configuration.

Rules add to built-in detection. New IDs need a name and quadrant, while existing catalog IDs retain their maintained metadata. Patterns run on eligible lines. They do not expand the file allowlist or evaluate code. See [detection rules](../reference/detection.md) for the complete format.

## Group unfamiliar packages

Create a separate manifest with three fictional packages from one UI framework:

```sh
mkdir -p "$radar_demo/kit"
cat > "$radar_demo/kit/package.json" <<'APP'
{
  "dependencies": {
    "@example-ui/core": "1.0.0",
    "@example-ui/dialog": "1.0.0",
    "@example-ui/button": "1.0.0"
  }
}
APP
```

A scan retains them as unknown package IDs. You have established that these packages belong to the same framework, so define the relationship explicitly:

```sh
cat > "$radar_demo/ui.grouping.json" <<'GROUPS'
{
  "schema_version": 1,
  "families": [{
    "id": "example-ui",
    "name": "Example UI framework",
    "quadrant": "Languages & Frameworks",
    "significance": "system_defining",
    "relationship": "framework_extensions",
    "scope": "component",
    "members": ["npm:@example-ui/core", "npm:@example-ui/dialog", "npm:@example-ui/button"]
  }]
}
GROUPS
```

Generate a report:

```sh
./tech-radar generate --repo "$radar_demo/kit" \
  --grouping "$radar_demo/ui.grouping.json" \
  --format html --out "$radar_demo/reports/UI-KIT.html"
```

The main report has one framework entry with three members. The evidence still contains all three original findings. The first detected member in the supplied order represents the group. Declaration-only evidence suggests Assess.

Use component scope for a framework owned by one application. Repository scope combines a shared runtime, platform, or workflow across components. Ambiguous component ownership stays separate. An explicit decision on a parent or member prevents the grouping from granting that decision to other members.

## Keep updates reviewable

Commit reviewed inputs with the appropriate project or policy repository. Keep private service names and organization-specific families outside the public tool source. When updating a report that used these inputs, pass the files again with `--rules` and `--grouping`.

An empty `patterns` array deliberately removes added detection rules. An empty `families` array removes runtime grouping overrides and restores built-in families. Both inputs are fingerprinted by meaning, so whitespace changes do not create a new classification.

The [grouping reference](../reference/grouping.md) covers prefixes, override semantics, validation, and limits. The [agent workflow](agents.md) can investigate additional relationships and prepare proposed inputs for review.
