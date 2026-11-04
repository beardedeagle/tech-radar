package radar

import (
	"bytes"
	"reflect"
	"testing"
)

func TestLifecyclePlacementHistorySurvivesFamilyTransitions(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	d := Decision{Lifecycle: "Adopt", Scope: "UI", Rationale: "Team decision", Status: "approved", Owner: "UI team", Reference: "ADR-1", Reviewed: "2026-11-03"}
	child := d
	child.Lifecycle = "Retire"
	splitPolicy := Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": child}}
	split := buildTestReport(t, inv, splitPolicy, nil, "", BuildOptions{})

	t.Run("same ID wins after root removal", func(t *testing.T) {
		fixture(t, root, "package.json", `{"dependencies":{"@radix-ui/react-dialog":"1"}}`)
		removed := buildTestReport(t, scanTestRepository(t, root, ScanOptions{}), splitPolicy, &split, "", BuildOptions{})
		f := finding(t, Inventory{Findings: removed.Highlights}, "npm:@radix-ui/react-dialog")
		if c := f.PlacementChange; c == nil || c.Kind != "unchanged" || c.PreviousRing != "Retire" {
			t.Fatalf("remaining member reused the removed root's placement: %+v", c)
		}
		if f := finding(t, Inventory{Findings: removed.Findings}, "react"); f.Presence != "not detected" || f.Decision != d {
			t.Fatal("removed root lost its saved decision")
		}
	})

	mergedPolicy := Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}
	merged := buildTestReport(t, inv, mergedPolicy, &split, "", BuildOptions{})
	if len(merged.Highlights) != 1 || len(merged.Highlights[0].Members) != 2 {
		t.Fatal("identical current decisions no longer group")
	}
	if f := finding(t, Inventory{Findings: merged.Findings}, "npm:@radix-ui/react-dialog"); f.PlacementChange == nil || f.PlacementChange.Kind != "moved-in" || f.PlacementChange.PreviousRing != "Retire" {
		t.Fatalf("merge lost member movement: %+v", f.PlacementChange)
	}
	mergedData := renderTestExport(t, merged, "html", true)
	mergedSaved := readTestReport(t, mergedData)
	mergedReplay := buildTestReport(t, inv, Policy{}, &mergedSaved, "", BuildOptions{})
	if !bytes.Equal(mergedData, renderTestExport(t, mergedReplay, "html", true)) {
		t.Fatal("repeating the merged generation erased member movement or changed its bytes")
	}
	detached := buildTestReport(t, inv, splitPolicy, &merged, "", BuildOptions{})
	f := finding(t, Inventory{Findings: detached.Highlights}, "npm:@radix-ui/react-dialog")
	if c := f.PlacementChange; c == nil || c.Kind != "moved-out" || c.PreviousRing != "Adopt" {
		t.Fatalf("existing singleton became new after splitting: %+v", c)
	}
	data := renderTestExport(t, detached, "html", true)
	saved := readTestReport(t, data)
	replay := buildTestReport(t, inv, Policy{}, &saved, "", BuildOptions{View: RadarView})
	if got := finding(t, Inventory{Findings: replay.Highlights}, f.ID); !reflect.DeepEqual(got.PlacementChange, f.PlacementChange) {
		t.Fatal("view-only replay erased singleton movement")
	}
}

func TestAmbiguousFamilyHistoryDoesNotChooseASubgroup(t *testing.T) {
	a := Finding{ID: "old-a", FamilyID: "react", GroupComponent: ".", Quadrant: "Languages & Frameworks", Members: []string{"old-a"}, Decision: Decision{Lifecycle: "Adopt"}}
	b := a
	b.ID, b.Members, b.Decision.Lifecycle = "old-b", []string{"old-b"}, "Retire"
	next := a
	next.ID, next.Members = "replacement", []string{"replacement"}
	for _, old := range [][]Finding{{a, b}, {b, a}} {
		prior := Report{Model: LifecycleModel, Highlights: old}
		r := Report{Model: LifecycleModel, Highlights: []Finding{next}}
		comparePlacements(&r, &prior)
		if c := r.Highlights[0].PlacementChange; c != nil {
			t.Fatalf("ambiguous family invented a prior placement: %+v", c)
		}
	}
}

func TestLifecycleSplitMemberDoesNotReuseAnotherComponentPlacement(t *testing.T) {
	root := t.TempDir()
	manifest := `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`
	fixture(t, root, "app-a/package.json", manifest)
	d := Decision{Lifecycle: "Adopt", Scope: "UI", Rationale: "Team decision", Status: "approved", Owner: "UI team", Reference: "ADR-1", Reviewed: "2026-11-03"}
	p := Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}
	prior := buildTestReport(t, scanTestRepository(t, root, ScanOptions{}), p, nil, "", BuildOptions{})
	fixture(t, root, "app-a/package.json", `{}`)
	fixture(t, root, "app-b/package.json", manifest)
	d.Lifecycle = "Retire"
	p.Rules["npm:@radix-ui/react-dialog"] = d
	next := buildTestReport(t, scanTestRepository(t, root, ScanOptions{}), p, &prior, "", BuildOptions{})
	f := finding(t, Inventory{Findings: next.Highlights}, "npm:@radix-ui/react-dialog")
	if c := f.PlacementChange; c == nil || c.Kind != "new" || c.PreviousRing != "" {
		t.Fatalf("split member reused a different component's group placement: %+v", c)
	}
}

func TestBaselinePlacementHistorySurvivesViewAndReviewTransitions(t *testing.T) {
	for _, reference := range []string{"", "ADR-1"} {
		t.Run("reference="+reference, func(t *testing.T) {
			inv := inventoryForTests()
			first := buildTestReport(t, inv, lifecyclePolicy(reference), nil, "Example", BuildOptions{})
			prior := readTestReport(t, renderTestExport(t, first, "json", false))
			second := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{View: RadarView})
			if second.Baseline || !first.Baseline {
				t.Fatal("baseline did not transition to a subsequent report")
			}
			for _, rows := range []struct{ before, after []Finding }{{first.Findings, second.Findings}, {first.Highlights, second.Highlights}} {
				for i, f := range rows.after {
					if f.PlacementChange == nil || f.PlacementChange.Kind != "new" || !reflect.DeepEqual(f.PlacementChange, rows.before[i].PlacementChange) {
						t.Fatalf("review-only transition erased original placement: %s %+v", f.ID, f.PlacementChange)
					}
				}
			}
			f := finding(t, Inventory{Findings: second.Findings}, "phoenix")
			if f.MissingDecisionMaterial != (reference == "") || f.ReviewRequired != (reference == "") || f.EvidenceChanged {
				t.Fatalf("placement replay changed review semantics: %+v", f)
			}
			data := renderTestExport(t, second, "html", false)
			saved := readTestReport(t, data)
			third := buildTestReport(t, inv, Policy{}, &saved, "", BuildOptions{})
			if !bytes.Equal(data, renderTestExport(t, third, "html", false)) {
				t.Fatal("repeating the resulting generation changed its bytes")
			}
		})
	}
}
