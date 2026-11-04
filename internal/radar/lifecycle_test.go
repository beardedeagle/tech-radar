package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func lifecyclePolicy(reference string) Policy {
	return Policy{Model: LifecycleModel, Rules: map[string]Decision{"phoenix": {Lifecycle: "aDoPt", Scope: "web application", Rationale: "Team decision", Owner: "Web team", Status: "approved", Reference: reference, Reviewed: "2026-11-03"}}}
}

func TestLifecycleBaselineBecomesSubsequentAndThenStable(t *testing.T) {
	inv := inventoryForTests()
	first := buildTestReport(t, inv, lifecyclePolicy(""), nil, "Example", BuildOptions{})
	if first.Model != LifecycleModel || !first.Baseline {
		t.Fatalf("missing baseline model: %#v", first)
	}
	f := finding(t, Inventory{Findings: first.Findings}, "phoenix")
	if f.Decision.Lifecycle != "Adopt" || f.MissingDecisionMaterial || f.ReviewRequired || f.Proposal != nil {
		t.Fatalf("baseline invented review gap or inference: %#v", f)
	}
	if pending := finding(t, Inventory{Findings: first.Findings}, "python"); first.Model.ring(pending) != "Needs decision" || !pending.ReviewRequired {
		t.Fatal("source evidence supplied lifecycle authority")
	}
	for _, format := range []string{"json", "markdown", "html"} {
		data, err := Render(first, format)
		if err != nil {
			t.Fatal(err)
		}
		prior := readTestReport(t, data)
		second := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{})
		f = finding(t, Inventory{Findings: second.Findings}, "phoenix")
		if second.Baseline || !f.MissingDecisionMaterial || !f.ReviewRequired || f.EvidenceChanged || f.Decision.Status != "approved" {
			t.Fatalf("unchanged baseline missed material gap: %#v", f)
		}
		third := buildTestReport(t, inv, Policy{}, &second, "", BuildOptions{})
		if !reflect.DeepEqual(second, third) {
			t.Fatal("identical later generation changed state")
		}
		a, _ := Render(second, format)
		b, _ := Render(third, format)
		if !bytes.Equal(a, b) {
			t.Fatal("identical later generation changed output")
		}
	}
}

func TestLifecycleReferencesChangesAndModelSwitch(t *testing.T) {
	inv := inventoryForTests()
	old := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"phoenix": {Ring: "Trial", Lifecycle: "maintain", Scope: "web", Rationale: "Team proposal"}}}, nil, "", BuildOptions{})
	switched := buildTestReport(t, inv, Policy{}, &old, "", BuildOptions{Model: LifecycleModel})
	f := finding(t, Inventory{Findings: switched.Findings}, "phoenix")
	if f.Decision != old.Decisions["phoenix"] || switched.Model.ring(f) != "Needs decision" || f.PlacementChange != nil || !f.MissingDecisionMaterial {
		t.Fatalf("model switch fabricated lifecycle: %#v", f)
	}
	approved := buildTestReport(t, inv, lifecyclePolicy("docs/adr-1.md"), &switched, "", BuildOptions{})
	f = finding(t, Inventory{Findings: approved.Findings}, "phoenix")
	if f.MissingDecisionMaterial || f.ReviewRequired || f.Decision.Status != "approved" {
		t.Fatalf("supplied decision lost: %#v", f)
	}
	inv.Findings[0].Evidence[0].Detail = "changed declaration"
	changed := buildTestReport(t, inv, Policy{}, &approved, "", BuildOptions{})
	if f = finding(t, Inventory{Findings: changed.Findings}, "phoenix"); !f.EvidenceChanged || !f.ReviewRequired || f.MissingDecisionMaterial {
		t.Fatalf("evidence and material gaps confused: %#v", f)
	}
	inv.Findings = inv.Findings[1:]
	removed := buildTestReport(t, inv, Policy{}, &changed, "", BuildOptions{})
	if f = finding(t, Inventory{Findings: removed.Findings}, "phoenix"); f.Presence != "not detected" || !f.ReviewRequired || f.Decision.Lifecycle != "Adopt" {
		t.Fatal("removed decision lost")
	}
}

func TestLifecycleValidationAndOriginalCompatibility(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":1,"model":"lifecycle","rules":{"phoenix":{"lifecycle":"invented"}}}`,
		`{"schema_version":1,"model":"lifecycle","rules":{"phoenix":{"lifecycle":"Adopt","status":"approved","owner":"Team","scope":"web","rationale":"decision"}}}`,
		`{"schema_version":1,"rules":{"phoenix":{"ring":"Adopt","status":"approved","scope":"web","rationale":"decision","owner":"Team","reviewed":"2026-11-03"}}}`,
		`{"schema_version":1,"model":"unknown","rules":{}}`,
	} {
		if _, err := ReadPolicyForModel([]byte(body), RecommendationModel); err == nil {
			t.Fatalf("accepted invalid policy %s", body)
		}
	}
	if _, err := BuildWithOptions(inventoryForTests(), lifecyclePolicy(""), nil, "", BuildOptions{Model: RecommendationModel}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatal("conflicting explicit models accepted")
	}
	if _, err := ReadPolicyForModel([]byte(`{"schema_version":1,"rules":{"phoenix":{"lifecycle":"custom legacy text"}}}`), RecommendationModel); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleGroupingRequiresEveryMemberDecisionToAgree(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	d := Decision{Lifecycle: "Hold", Scope: "UI component", Rationale: "Review migration", Status: "approved", Owner: "UI team", Reviewed: "2026-11-03", Reference: "docs/adr-ui.md"}
	p := Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}
	grouped := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	f := finding(t, Inventory{Findings: grouped.Highlights}, "react")
	if !contains(f.Members, "npm:@radix-ui/react-dialog") || f.Decision != d || grouped.Decisions["npm:@radix-ui/react-dialog"] != d {
		t.Fatal("identical supplied decisions failed to group or were lost")
	}
	if _, err := Render(grouped, "json"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"lifecycle", "scope", "rationale", "owner", "reference", "reviewed", "status"} {
		changed := d
		switch field {
		case "lifecycle":
			changed.Lifecycle = "Retire"
		case "scope":
			changed.Scope = "Dialogs only"
		case "rationale":
			changed.Rationale = "Separate migration"
		case "owner":
			changed.Owner = "Another team"
		case "reference":
			changed.Reference = "docs/another-adr.md"
		case "reviewed":
			changed.Reviewed = "2026-11-02"
		case "status":
			changed.Status = "proposed"
		}
		p.Rules["npm:@radix-ui/react-dialog"] = changed
		separate := buildTestReport(t, inv, p, nil, "", BuildOptions{})
		parent := finding(t, Inventory{Findings: separate.Highlights}, "react")
		child := finding(t, Inventory{Findings: separate.Highlights}, "npm:@radix-ui/react-dialog")
		if contains(parent.Members, child.ID) || child.Decision != changed {
			t.Fatalf("group concealed differing %s", field)
		}
	}
	delete(p.Rules, "npm:@radix-ui/react-dialog")
	unapproved := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	child := finding(t, Inventory{Findings: unapproved.Highlights}, "npm:@radix-ui/react-dialog")
	if unapproved.Model.ring(child) != "Needs decision" || child.Decision.Status == "approved" {
		t.Fatal("parent decision approved child")
	}
}

func TestLifecycleMovementAndCrossModelApprovals(t *testing.T) {
	inv := inventoryForTests()
	p := lifecyclePolicy("docs/adr-1.md")
	first := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	d := p.Rules["phoenix"]
	d.Lifecycle = "Experiment"
	p.Rules["phoenix"] = d
	second := buildTestReport(t, inv, p, &first, "", BuildOptions{})
	if f := finding(t, Inventory{Findings: second.Findings}, "phoenix"); f.PlacementChange == nil || f.PlacementChange.Kind != "moved-out" || f.PlacementChange.PreviousRing != "Adopt" {
		t.Fatalf("lost lifecycle movement: %#v", f)
	}
	if _, err := BuildWithOptions(inv, Policy{}, &second, "", BuildOptions{Model: RecommendationModel}); err == nil || !strings.Contains(err.Error(), "replacement") {
		t.Fatal("cross-model approval did not require explicit replacement")
	}
	replacement := Policy{Rules: map[string]Decision{"phoenix": {Ring: "Trial", Scope: "web", Rationale: "reviewed target model decision", Status: "approved", Owner: "Team", Reference: "PR #2", Reviewed: "2026-11-03"}}}
	switched := buildTestReport(t, inv, replacement, &second, "", BuildOptions{Model: RecommendationModel})
	if f := finding(t, Inventory{Findings: switched.Findings}, "phoenix"); f.PlacementChange != nil || f.Decision != replacement.Rules["phoenix"] {
		t.Fatal("explicit switch lost decision or fabricated movement")
	}
	inv.Findings = append(inv.Findings, Finding{ID: "new-tool", Name: "New tool", Quadrant: "Tools", Presence: "detected", Evidence: []Evidence{}})
	withNew := buildTestReport(t, inv, Policy{}, &switched, "", BuildOptions{Model: LifecycleModel})
	if f := finding(t, Inventory{Findings: withNew.Findings}, "new-tool"); f.PlacementChange == nil || f.PlacementChange.Kind != "new" || !f.MissingDecisionMaterial {
		t.Fatal("model switch hid newly observed technology")
	}
	if f := finding(t, Inventory{Findings: withNew.Findings}, "phoenix"); f.PlacementChange != nil {
		t.Fatal("switch marked existing technology as new")
	}
}

func TestLifecycleCompactAndEscapedRoundTrips(t *testing.T) {
	inv, scanErr := ScanWithOptions(beamFixture(t), ScanOptions{})
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	p := lifecyclePolicy(`docs/<script>decision</script>|record`)
	baseline := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	second := buildTestReport(t, inv, Policy{}, &baseline, "", BuildOptions{})
	for _, format := range []string{"markdown", "html"} {
		for _, compact := range []bool{false, true} {
			data := renderTestExport(t, second, format, compact)
			body := strings.SplitN(string(data), snapshotStart, 2)[0]
			if strings.Contains(body, "<script>decision</script>") || !strings.Contains(body, "Adopt") || !strings.Contains(body, "Needs decision") || !strings.Contains(body, "missing decision material") || !strings.Contains(body, "https://martinfowler.com/articles/scaling-architecture-conversationally.html") {
				t.Fatal("wrong lifecycle labels, gaps or escaping")
			}
			recovered := readTestReport(t, data)
			if recovered.Model != LifecycleModel || recovered.Baseline || !finding(t, Inventory{Findings: recovered.Findings}, "python").MissingDecisionMaterial {
				t.Fatal("snapshot lost lifecycle review state")
			}
			again := buildTestReport(t, inv, Policy{}, &recovered, "", BuildOptions{})
			if !reflect.DeepEqual(second, again) {
				t.Fatal("compact replay changed state")
			}
		}
	}
}

func TestLegacyReportsKeepFreeTextLifecycleAndRecommendationMeaning(t *testing.T) {
	original := buildTestReport(t, inventoryForTests(), Policy{Rules: map[string]Decision{"phoenix": {Ring: "Trial", Lifecycle: "custom <script>phase</script>", Scope: "web", Rationale: "Team decision"}}}, nil, "", BuildOptions{})
	original.Model = ""
	for _, schema := range []int{1, 2} {
		original.SchemaVersion = schema
		for _, format := range []string{"markdown", "html", "json"} {
			data, err := Render(original, format)
			if err != nil {
				t.Fatal(err)
			}
			recovered := readTestReport(t, data)
			next := buildTestReport(t, inventoryForTests(), Policy{}, &recovered, "", BuildOptions{})
			f := finding(t, Inventory{Findings: next.Findings}, "phoenix")
			if next.Model != RecommendationModel || f.Decision.Lifecycle != original.Decisions["phoenix"].Lifecycle || ring(f) != "Trial" || f.MissingDecisionMaterial {
				t.Fatal("legacy report interpretation changed")
			}
		}
	}
}

func TestContextRejectsLifecycleReviewStateInCapturedObservations(t *testing.T) {
	s := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19"}}`)
	s.Inventory.Findings[0].MissingDecisionMaterial = true
	if _, err := BuildContext([]RepositorySnapshot{s}); err == nil || !strings.Contains(err.Error(), "observations") {
		t.Fatal("capture accepted lifecycle review state")
	}
}

func TestLifecycleGroupMaterialGapsAndExclusionsRemainIndependent(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	d := Decision{Lifecycle: "Hold", Quadrant: "Languages & Frameworks", Scope: "UI component", Rationale: "Team review", Status: "approved", Owner: "UI team", Reviewed: "2026-11-03"}
	p := Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}
	first := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	if f := finding(t, Inventory{Findings: first.Highlights}, "react"); f.MissingDecisionMaterial || f.ReviewRequired || len(f.Members) != 2 {
		t.Fatalf("baseline group state incorrect: %#v", f)
	}
	second := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	if f := finding(t, Inventory{Findings: second.Highlights}, "react"); !f.MissingDecisionMaterial || !f.ReviewRequired {
		t.Fatal("group lost member review gaps")
	}
	empty := inv
	empty.Findings = nil
	excluded := buildTestReport(t, empty, Policy{Rules: map[string]Decision{"react": {Exclude: true, Rationale: "Team excludes obsolete UI"}}}, &second, "", BuildOptions{})
	if len(excluded.ExcludedFindings) != 1 || excluded.ExcludedFindings[0].MissingDecisionMaterial {
		t.Fatal("excluded technology kept included-material gap")
	}
	if _, err := Render(excluded, "json"); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleReferenceAloneDoesNotPlaceOrApprove(t *testing.T) {
	inv := inventoryForTests()
	p := Policy{Model: LifecycleModel, Rules: map[string]Decision{"phoenix": {Reference: "https://example.test/pull/1"}}}
	first := buildTestReport(t, inv, p, nil, "", BuildOptions{})
	second := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	f := finding(t, Inventory{Findings: second.Findings}, "phoenix")
	if second.Model.ring(f) != "Needs decision" || f.Decision.Status == "approved" || f.Proposal != nil || !f.ReviewRequired || f.MissingDecisionMaterial {
		t.Fatal("reference granted lifecycle authority or was ignored")
	}
}

func TestRemovingLifecycleMaterialDoesNotClearChangedEvidence(t *testing.T) {
	inv := inventoryForTests()
	first := buildTestReport(t, inv, lifecyclePolicy("docs/adr-1.md"), nil, "", BuildOptions{})
	inv.Findings[0].Evidence[0].Detail = "new dependency version"
	changed := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	missing := buildTestReport(t, inv, lifecyclePolicy(""), &changed, "", BuildOptions{})
	f := finding(t, Inventory{Findings: missing.Findings}, "phoenix")
	if !f.EvidenceChanged || !f.MissingDecisionMaterial || !f.ReviewRequired {
		t.Fatal("removing decision material silently reviewed changed evidence")
	}
	reviewed := buildTestReport(t, inv, lifecyclePolicy("docs/adr-2.md"), &missing, "", BuildOptions{})
	f = finding(t, Inventory{Findings: reviewed.Findings}, "phoenix")
	if f.EvidenceChanged || f.MissingDecisionMaterial || f.ReviewRequired {
		t.Fatal("supplied review did not resolve independent gaps")
	}
}

func TestLifecycleChartPlacesAllStatesInTheirCanonicalBands(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	cases := []struct {
		id, lifecycle string
		inner, outer  float64
	}{
		{"phoenix", "Adopt", 0, 215},
		{"ash", "Experiment", 215, 295},
		{"ecto", "Hold", 295, 365},
		{"oban", "Retire", 365, 430},
	}
	p := Policy{Model: LifecycleModel, Rules: map[string]Decision{}}
	for _, tt := range cases {
		p.Rules[tt.id] = Decision{Lifecycle: tt.lifecycle, Scope: "application", Rationale: "Supplied team decision"}
	}
	r := buildTestReport(t, inv, p, nil, "", BuildOptions{View: RadarView})
	for _, compact := range []bool{false, true} {
		data := renderTestExport(t, r, "html", compact)
		for _, tt := range cases {
			f := finding(t, Inventory{Findings: r.Findings}, tt.id)
			pattern := fmt.Sprintf(`<a href="#entry-%d"[^>]*><circle cx="([0-9.]+)" cy="([0-9.]+)"`, f.Number)
			m := regexp.MustCompile(pattern).FindSubmatch(data)
			if len(m) != 3 {
				t.Fatalf("%s is missing from lifecycle chart", tt.id)
			}
			x, _ := strconv.ParseFloat(string(m[1]), 64)
			y, _ := strconv.ParseFloat(string(m[2]), 64)
			radius := math.Hypot(x-500, y-500)
			if radius-15 <= tt.inner || radius+15 >= tt.outer {
				t.Fatalf("%s radius %f outside %s", tt.id, radius, tt.lifecycle)
			}
			if !strings.Contains(chartTooltip(r.Model, f), tt.lifecycle) {
				t.Fatal("tooltip omitted lifecycle placement")
			}
		}
	}
}

func TestLifecycleReportRejectsForgedReviewAndGroupedApproval(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	d := Decision{Lifecycle: "Adopt", Scope: "UI", Rationale: "Team decision", Owner: "UI team", Status: "approved", Reviewed: "2026-11-03"}
	first := buildTestReport(t, inv, Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}, nil, "", BuildOptions{})
	second := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	for i := range second.Findings {
		second.Findings[i].MissingDecisionMaterial = false
	}
	data, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(data); err == nil {
		t.Fatal("forged report cleared missing-material review")
	}
	d.Scope = "Dialogs only"
	first.Decisions["npm:@radix-ui/react-dialog"] = d
	for i := range first.Findings {
		if first.Findings[i].ID == "npm:@radix-ui/react-dialog" {
			first.Findings[i].Decision = d
		}
	}
	data, err = json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(data); err == nil {
		t.Fatal("forged group granted a parent decision to a differently scoped member")
	}
}
