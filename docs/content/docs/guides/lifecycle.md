---
title: "Build a lifecycle radar"
weight: 15
---

# Build a lifecycle radar

Use `--model lifecycle` to place technologies from supplied lifecycle decisions. The scanner supplies candidates and source evidence. Candidates without a lifecycle decision remain visible as Needs decision.

The default `recommendation` model keeps Adopt, Trial, Assess, and Caution with evidence-based placement proposals. `--view radar|table` chooses the layout. Lifecycle defaults to table and recommendation defaults to radar. `--format markdown|html|json` chooses the encoding for either model.

The lifecycle method follows [Scaling the Practice of Architecture, Conversationally](https://martinfowler.com/articles/scaling-architecture-conversationally.html).

## Choose a lifecycle state

| State | Team intent to record |
| --- | --- |
| Adopt | Use within the recorded scope. The innermost ring. |
| Experiment | Evaluate within a limited scope and review the results. |
| Hold | Continue existing use while directing or restricting new work elsewhere, within the recorded scope. |
| Retire | Withdraw use within the recorded scope. The outermost ring. |
| Needs decision | A candidate has no lifecycle placement. This is a review category. |

The rings run outward from Adopt through Experiment and Hold to Retire. Supply the state in `lifecycle`, with case-insensitive input such as `experiment` saved as `Experiment`. Keep the scope and rationale specific to the component. Source frequency supplies no lifecycle approval.

## Create proposed inputs

From the tech-radar project root, complete the [first-radar setup](../getting-started/first-radar.md) in the same shell so `$radar_demo` points to the sample application. Add a repository decision document for React. It records a proposal awaiting team review.

```sh
mkdir -p "$radar_demo/app/docs/decisions"
cat > "$radar_demo/app/docs/decisions/react.md" <<'DECISION'
# React UI experiment

Status is proposed, awaiting team review.
Scope is this application's web UI.
Evaluate React in a limited UI experiment before deciding on wider use.
DECISION
```

Copy the bundled example policy to the run directory.

```sh
cp examples/lifecycle.policy.json "$radar_demo/lifecycle.policy.json"
```

It contains these proposed decisions.

```json
{
  "schema_version": 1,
  "model": "lifecycle",
  "repository": "Example UI",
  "rules": {
    "react": {
      "lifecycle": "Experiment",
      "scope": "Application web UI",
      "rationale": "Propose a limited UI experiment for team review",
      "status": "proposed",
      "reference": "docs/decisions/react.md"
    },
    "typescript": {
      "lifecycle": "Hold",
      "scope": "Application TypeScript source",
      "rationale": "Propose continuing existing TypeScript use while restricting new work pending a team decision",
      "status": "proposed"
    }
  }
}
```

React cites the proposal document. TypeScript deliberately has no reference so the example shows the first-generation exception. Both placements remain proposed and require team review.

A reference can be a repository documentation path, ADR identifier, PR URL, Drive document URL, or another explicit decision record. Read it and establish its status and scope before claiming it supports approval. The CLI consumes supplied references and does not fetch their contents. An agent can investigate repository docs and explicitly scoped Drive documents through available host integrations. See the [agent workflow](agents.md).

## Generate the initial baseline

```sh
./tech-radar generate --repo "$radar_demo/app" --model lifecycle \
  --policy "$radar_demo/lifecycle.policy.json" \
  --format html --out "$radar_demo/reports/LIFECYCLE.html"
```

Open `LIFECYCLE.html`. The default table groups entries by quadrant and technology family, with colored state badges, scope, owner, decision reference, review date, and changes or review reasons. Native disclosure controls expose available evidence and grouped members. Proposed decisions leave approval fields unset. The table supports keyboard access and light or dark appearance.

Both views show the same architectural and workflow choices, plus any technologies with explicit team decisions. Supporting dependencies stay in the full evidence file. Using `--self-contained` embeds that evidence without adding inventory rows to the table.

No `--previous` means this is the initial lifecycle baseline. React appears in Experiment and TypeScript in Hold. Missing TypeScript decision material creates no missing-material review gap on this generation. Proposed status still requires review. Candidates with no lifecycle placement stay in Needs decision, and source absence or other review reasons still apply.

Commit the report and its linked evidence file when the inputs are ready for your repository's review process. Keep private decision material in authorized storage.

## Generate a subsequent review

```sh
./tech-radar generate --repo "$radar_demo/app" \
  --policy "$radar_demo/lifecycle.policy.json" \
  --previous "$radar_demo/reports/LIFECYCLE.html" \
  --format html --out "$radar_demo/reports/LIFECYCLE-next.html"
```

The policy supplies the model, so this command can omit `--model`. TypeScript now has a missing-decision-material gap because its reference is blank. React's reference avoids that gap, while its proposed status still requires review. In the evidence JSON, `missing_decision_material: true` identifies this gap separately from `review_required`, source presence, and evidence changes.

Every generation with `--previous` is subsequent, including unchanged input or a previous recommendation report. Passing the initial baseline as `--previous` activates the missing-material check. Identical later reruns preserve this state.

After an authorized review, record the actual decision and its owner, scope, rationale, and review date. Lifecycle permits a missing reference even for an approved decision, so the missing-material gap remains on subsequent runs until a nonblank reference is supplied. The recommendation model continues to require a reference for approval. See the [display-policy fields](../reference/display-policy.md).

## Choose another view

To see the same lifecycle decisions on a radar, explicitly select that view.

```sh
./tech-radar generate --repo "$radar_demo/app" --view radar \
  --policy "$radar_demo/lifecycle.policy.json" \
  --previous "$radar_demo/reports/LIFECYCLE-next.html" \
  --format html --out "$radar_demo/reports/LIFECYCLE-radar.html"
```

The lifecycle model carries forward. Changing the view preserves decisions, source evidence, and placement history. The radar uses the same Experiment, Adopt, Hold, and Retire placements. Missing-material review gaps remain. Passing an initial baseline as `--previous` would still activate those gaps even if the only requested change were the view.

Without `--view`, a same-model previous report supplies its selected view. With no comparable previous model, generation uses the selected model's default. A model switch therefore defaults to table for lifecycle or radar for recommendation, unless you supply a view explicitly. View is report metadata and has no display-policy field.

For Markdown, table view creates a row-per-entry decision table. Radar view retains the quadrant/ring matrix and findings register. JSON retains the full report, including `model`, `view`, and lifecycle `baseline` metadata. The [files reference](../reference/files.md) describes compact snapshots and evidence companions.

## Preserve scope and history

A current policy entry replaces the whole saved decision for that ID. Omitted entries carry forward. Supply any required detection, grouping, and context inputs again. Member lifecycle decisions, rationales, and approval scopes must match for grouping. A parent decision supplies no approval to other members. Keep independent operational boundaries separate.

An omitted model inherits an explicit policy model, then a previous report model, then `recommendation`. An explicit CLI model must agree with an explicit policy model. To switch models, pass the intended model and compatible policy inputs. Carried approvals require explicit target-model replacements before they can approve target placement. If carried records cannot satisfy target validation, the CLI requests complete replacements. Trial, Assess, and Caution never translate into lifecycle states.

Movement compares placements within the same model. A switch starts a fresh comparison. New candidates, lifecycle changes, evidence changes, and entries no longer detected remain distinguishable in the report. Old schema-1 and schema-2 recommendation reports remain readable, including their free-text lifecycle values.

## Check permission separately

`generate --policy` records display decisions. `check --policy` evaluates an approved enforcement standard against observations. Lifecycle labels and missing-decision-material review gaps belong to radar review. The [enforcement workflow](check-policy.md) has its own rules, coverage gaps, baselines, and exit codes.
