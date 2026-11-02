---
title: "Check policy and drift"
weight: 40
---

# Check policy and drift

Use `check` when you already have a technology standard and want to find observed use that conflicts with it. The checker evaluates the full findings, including supporting dependencies that the main radar omits.

This example continues the [first-run walkthrough](../getting-started/first-radar.md). It first captures a compliant baseline, then adds a dependency the standard does not permit. Keep the same shell and `$radar_demo` directory.

## 1. Supply an approved standard

The following standard is fictional and applies only to the demo. For a real repository, use your team's actual reviewed policy and approval record.

```sh
cat > "$radar_demo/standard.json" <<'STANDARD'
{
  "kind": "tech-radar-policy",
  "schema_version": 1,
  "id": "example-ui-standard",
  "version": "1",
  "default": "deny",
  "approval": {
    "status": "approved",
    "owner": "Example UI team",
    "reference": "Example decision 1",
    "reviewed": "2026-11-01"
  },
  "rules": [
    {
      "id": "ui-stack",
      "technologies": ["react", "typescript"],
      "effect": "allow"
    }
  ]
}
STANDARD
```

This permits the recognized React and TypeScript findings and denies other observed technologies by default. React's recognized package aliases are covered by its technology ID. This file is an enforcement standard, distinct from the display policy used to place entries on a radar.

## 2. Capture a baseline

```sh
./tech-radar check --repo "$radar_demo/app" \
  --policy "$radar_demo/standard.json" --as-of 2026-11-01 \
  --out "$radar_demo/reports/baseline.json"
echo "$?"
```

The exit code is `0`. The JSON contains the captured observations, policy identity and fingerprint, evaluation date, and empty violation and coverage-gap lists. An explicit date makes the example reproducible; otherwise the command uses the current UTC date.

## 3. Add a dependency outside the standard

Replace the demo manifest with one that also declares Lodash:

```sh
cat > "$radar_demo/app/package.json" <<'APP'
{
  "private": true,
  "dependencies": {"react": "19.1.0", "react-dom": "19.1.0", "lodash": "4.17.21"},
  "devDependencies": {"typescript": "5.8.3"}
}
APP
```

Run the checker against the earlier baseline:

```sh
./tech-radar check --repo "$radar_demo/app" \
  --policy "$radar_demo/standard.json" \
  --baseline "$radar_demo/reports/baseline.json" --as-of 2026-11-01 \
  --out "$radar_demo/reports/check.json"
echo "$?"
```

The command writes a result and exits `2`. Its violations include technology `npm:lodash` with code `not-permitted`, and its drift includes that added technology. This is a completed evaluation that found a violation, not a failed scan.

The main radar need not promote Lodash to an architectural finding for enforcement to see it. Presentation and checking operate on different views of the same evidence.

## 4. Interpret the result

| Result | Meaning |
| --- | --- |
| Violation | Observed use does not satisfy the supplied permission rules. |
| Drift | An observation changed compared with the baseline. A permitted addition can still be drift. |
| Coverage gap | The tool cannot establish a required fact, such as a supported version pin or comparable baseline. |
| Failed run | Input, collection, validation, or output failed. No successful evaluation is claimed. |

Exit `2` can include both violations and gaps. Exit `3` means gaps without confirmed violations. Inspect the JSON instead of treating every nonzero exit as the same failure.

## 5. Reproduce a check offline

Use saved observations with the same inputs and date:

```sh
./tech-radar check --from "$radar_demo/reports/check.json" \
  --policy "$radar_demo/standard.json" \
  --baseline "$radar_demo/reports/baseline.json" --as-of 2026-11-01 \
  --out "$radar_demo/reports/replayed-check.json"
```

This still exits `2` because the saved observations still contain the violation. It does not refresh the repository or organization membership.

## Apply the workflow to a team

Keep the approved standard separate from repository capture and submit proposed changes through your review process. An approved, expiring exception can authorize a bounded migration. An `existing-only` rule can permit continued use at baseline locations while prohibiting new locations. Neither should be created merely to turn a failing check green.

See [enforcement policy](../reference/policy.md) for rule precedence, selectors, versions, exceptions, limits, and baseline compatibility. Use the same capture version for both revisions when comparing drift across a tool upgrade.
