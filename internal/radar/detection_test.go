package radar

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRulesAreCanonicalAndIsolatedBetweenRuns(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "lib/app.ex", "Example.CRM.call()\nExample.Scheduler.call()\n")
	a := `{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"Example.CRM","detail":"CRM reference"},{"id":"scheduler","name":"Example scheduler","quadrant":"Platforms","pattern":"Example.Scheduler","detail":"Scheduler reference"}]}`
	b := `{"patterns":[{"detail":"Scheduler reference","pattern":"Example.Scheduler","quadrant":"Platforms","name":"Example scheduler","id":"scheduler"},{"detail":"CRM reference","pattern":"Example.CRM","id":"hubspot"}],"schema_version":1}`
	rulesA, err := ReadDetectionRules([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	rulesB, err := ReadDetectionRules([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	first := scanTestRepository(t, root, ScanOptions{Rules: rulesA})
	second := scanTestRepository(t, root, ScanOptions{Rules: rulesB})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("JSON order or rule order changed effective output")
	}
	plain := scanTestRepository(t, root, ScanOptions{})
	for _, f := range plain.Findings {
		if f.ID == "hubspot" || f.ID == "scheduler" {
			t.Fatal("run-specific rules leaked into the default scanner")
		}
	}
	if plain.Source.RulesFingerprint != "" {
		t.Fatal("default scan inherited rules provenance")
	}
	policy := Policy{SchemaVersion: 1, Rules: map[string]Decision{"scheduler": {Ring: "Assess", Scope: "Candidate", Rationale: "Review this integration"}}}
	first.Findings = nil
	report := buildTestReport(t, first, policy, nil, "Example", BuildOptions{})
	if len(report.Findings) != 1 || report.Findings[0].Name != "Example scheduler" || report.Findings[0].Presence != "not detected" {
		t.Fatal("runtime catalog did not support an undetected policy candidate")
	}
}

func TestDetectionRulesRejectAmbiguousAndUnboundedInput(t *testing.T) {
	base := detectionPattern{ID: "hubspot", Pattern: "Example.CRM", Detail: "CRM reference"}
	many := make([]detectionPattern, 257)
	for i := range many {
		many[i] = detectionPattern{ID: "hubspot", Pattern: fmt.Sprintf("Example.CRM%03d", i), Detail: "CRM reference"}
	}
	for name, patterns := range map[string][]detectionPattern{
		"duplicate":                   {base, base},
		"oversized expression":        {{ID: "hubspot", Pattern: strings.Repeat("x", 4097), Detail: "Reference"}},
		"missing detail":              {{ID: "hubspot", Pattern: "Example"}},
		"catalog override":            {{ID: "hubspot", Name: "Different service", Pattern: "Example", Detail: "Reference"}},
		"conflicting custom metadata": {{ID: "custom", Name: "One", Quadrant: "Tools", Pattern: "One", Detail: "Reference"}, {ID: "custom", Name: "Two", Quadrant: "Tools", Pattern: "Two", Detail: "Reference"}},
		"too many rules":              many,
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(struct {
				SchemaVersion int                `json:"schema_version"`
				Patterns      []detectionPattern `json:"patterns"`
			}{1, patterns})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReadDetectionRules(data); err == nil {
				t.Fatal("accepted unsafe or ambiguous rules")
			}
		})
	}
}

func TestExplicitRulesCanRecognizeLiteralManifestReferences(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"@example/private-sdk":"1"}}`)
	rules, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"example-platform","name":"Example platform","quadrant":"Platforms","pattern":"@example/private-sdk","detail":"Explicit SDK mapping"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv := scanTestRepository(t, root, ScanOptions{Rules: rules})
	f := finding(t, inv, "example-platform")
	if len(f.Evidence) != 1 || f.Evidence[0].Path != "package.json" || f.Evidence[0].Kind != "configuration" {
		t.Fatal("literal manifest was not recognized as configuration evidence")
	}
	_ = finding(t, inv, "npm:@example/private-sdk")
}

func TestChangedRuntimeClassificationCannotSilentlyReuseApproval(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "lib/app.ex", "Example.Service.call()\n")
	rulesA, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"custom-service","name":"Example service","quadrant":"Platforms","pattern":"Example.Service","detail":"Integration reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv := scanTestRepository(t, root, ScanOptions{Rules: rulesA})
	decision := Decision{Ring: "Adopt", Scope: "Existing integration", Rationale: "Team review", Owner: "Team", Status: "approved", Reference: "PR #1", Reviewed: "2026-10-30"}
	previous := buildTestReport(t, inv, Policy{SchemaVersion: 1, Rules: map[string]Decision{"custom-service": decision}}, nil, "Example", BuildOptions{})
	rulesB, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"custom-service","name":"Renamed tool","quadrant":"Tools","pattern":"Example.Service","detail":"Integration reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv = scanTestRepository(t, root, ScanOptions{Rules: rulesB})
	next := buildTestReport(t, inv, Policy{}, &previous, "", BuildOptions{})
	if next.Decisions["custom-service"] != decision || !next.Findings[0].ReviewRequired {
		t.Fatal("runtime classification changed the meaning of an unflagged approval")
	}
	again, err := BuildWithOptions(inv, Policy{}, &next, "", BuildOptions{})
	if err != nil || !again.Findings[0].ReviewRequired {
		t.Fatalf("rerun silently cleared classification review: %v", err)
	}
}
