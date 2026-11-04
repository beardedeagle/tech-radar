package radar

import (
	"strings"
	"testing"
)

func inventoryForTests() Inventory {
	return Inventory{Source: Source{Fingerprint: strings.Repeat("a", 64), Files: 1}, Warnings: []string{}, Findings: []Finding{{ID: "phoenix", Name: "Phoenix", Quadrant: "Languages & Frameworks", Evidence: []Evidence{{"mix.exs", 4, "declaration", "hex package phoenix: ~> 1.8"}}, EvidenceCount: 1, Presence: "detected", ReviewRequired: true}, {ID: "python", Name: "Python", Quadrant: "Languages & Frameworks", Evidence: []Evidence{{"bin/check.py", 1, "source reference", "Source file extension .py"}}, EvidenceCount: 1, Presence: "detected", ReviewRequired: true}}}
}

// Check policy typos and incomplete approvals.
func TestPolicyValidation(t *testing.T) {
	bad := []string{
		`{"schema_version":1,"rulse":{}}`,
		`{"schema_version":9,"rules":{}}`,
		`{"schema_version":1,"rules":{"phoenix":{"ring":"Maintain"}}}`,
		`{"schema_version":1,"rules":{"phoenix":{"ring":"Adopt","status":"approved"}}}`,
		`{"schema_version":1,"rules":{"phoenix":{"exclude":true,"ring":"Adopt"}}}`,
		`{"schema_version":1,"rules":{"phoenix":{"ring":"Adopt","ring":"Caution"}}}`,
		`{"schema_version":1,"rules":{"phoenix":{"quadrant":"Things"}}}`,
		`{"schema_version":1,"rules":{}} {}`,
	}
	for _, input := range bad {
		if _, err := ReadPolicyForModel([]byte(input), RecommendationModel); err == nil {
			t.Errorf("accepted invalid policy %s", input)
		}
	}
	p, err := ReadPolicyForModel([]byte(`{"schema_version":1,"repository":"ExampleApp","rules":{"phoenix":{"ring":"Adopt","scope":"ExampleApp web","rationale":"Established application framework","owner":"Team","status":"approved","reference":"PR #1","reviewed":"2026-10-30"},"python":{"exclude":true,"rationale":"Outside the approved application stack"}}}`), RecommendationModel)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rules["phoenix"].Status != "approved" {
		t.Fatal("lost explicit approval")
	}
}

func TestPreservesDecisionsAndExclusionsAndFlagsEvidenceChanges(t *testing.T) {
	policy := Policy{SchemaVersion: 1, Repository: "ExampleApp", Rules: map[string]Decision{"phoenix": {Ring: "Adopt", Scope: "ExampleApp", Rationale: "Existing framework", Owner: "Team", Status: "approved", Reference: "PR #1", Reviewed: "2026-10-30"}, "python": {Exclude: true, Rationale: "Team exclusion"}}}
	inv := inventoryForTests()
	first := buildTestReport(t, inv, policy, nil, "", BuildOptions{})
	if len(first.Findings) != 1 || len(first.Excluded) != 1 || first.Excluded[0] != "python" {
		t.Fatalf("exclusion lost: %#v", first)
	}
	next := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	if next.Findings[0].Decision.Status != "approved" || next.Findings[0].ReviewRequired {
		t.Fatalf("unchanged decision not preserved: %#v", next.Findings)
	}
	inv.Findings[0].Evidence[0].Detail = "hex package phoenix: ~> 2.0"
	next = buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	if !next.Findings[0].EvidenceChanged || !next.Findings[0].ReviewRequired || next.Findings[0].Decision.Reference != "PR #1" {
		t.Fatalf("changed evidence silently inherited approval: %#v", next.Findings)
	}
	inv.Findings = inv.Findings[1:]
	next = buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	if len(next.Findings) != 1 || next.Findings[0].Presence != "not detected" || !next.Findings[0].ReviewRequired {
		t.Fatalf("disappearing decision silently dropped: %#v", next.Findings)
	}
}

func TestExplicitPolicyOverridesPriorAndAllowsManualCandidates(t *testing.T) {
	first := buildTestReport(t, inventoryForTests(), Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": {Ring: "Adopt", Scope: "ExampleApp", Rationale: "First proposal"}}}, nil, "ExampleApp", BuildOptions{})
	p := Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": {Ring: "Caution", Scope: "Existing deployments", Rationale: "New team decision"}, "custom-tool": {Name: "Custom candidate", Quadrant: "Tools", Ring: "Assess", Scope: "Investigation", Rationale: "Team proposal"}}}
	next := buildTestReport(t, inventoryForTests(), p, &first, "", BuildOptions{})
	if finding(t, Inventory{Findings: next.Findings}, "phoenix").Decision.Ring != "Caution" {
		t.Fatal("current rule lost")
	}
	candidate := finding(t, Inventory{Findings: next.Findings}, "custom-tool")
	if candidate.Presence != "not detected" || len(candidate.Evidence) != 0 {
		t.Fatal("invented evidence for manual candidate")
	}
	_, err := BuildWithOptions(inventoryForTests(), Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenxi": {Ring: "Adopt", Scope: "ExampleApp", Rationale: "Typo"}}}, nil, "", BuildOptions{})
	if err == nil {
		t.Fatal("unknown typo became a manual recommendation")
	}
}

func TestRecommendationRequiresScopeAndRationale(t *testing.T) {
	for _, d := range []Decision{{Ring: "Adopt"}, {Ring: "Adopt", Scope: "ExampleApp"}} {
		if _, err := BuildWithOptions(inventoryForTests(), Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": d}}, nil, "", BuildOptions{}); err == nil {
			t.Fatalf("accepted unexplained recommendation %#v", d)
		}
	}
}

func TestStaleApprovalStaysFlaggedUntilExplicitReview(t *testing.T) {
	p := Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": {Ring: "Adopt", Scope: "ExampleApp", Rationale: "Existing framework", Owner: "Team", Status: "approved", Reference: "PR #1", Reviewed: "2026-10-30"}}}
	inv := inventoryForTests()
	a := buildTestReport(t, inv, p, nil, "ExampleApp", BuildOptions{})
	inv.Findings[0].Evidence[0].Detail = "hex package phoenix: ~> 2.0"
	b := buildTestReport(t, inv, p, &a, "", BuildOptions{})
	c := buildTestReport(t, inv, p, &b, "", BuildOptions{})
	if !finding(t, Inventory{Findings: c.Findings}, "phoenix").ReviewRequired {
		t.Fatal("rerun silently cleared a stale approval")
	}
	d := p.Rules["phoenix"]
	d.Reference = "PR #2"
	p.Rules["phoenix"] = d
	reviewed := buildTestReport(t, inv, p, &c, "", BuildOptions{})
	if finding(t, Inventory{Findings: reviewed.Findings}, "phoenix").ReviewRequired {
		t.Fatal("explicit new review was not applied")
	}
}
