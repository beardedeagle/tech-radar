package radar

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestGenericGroupingCatalogWorksAcrossEcosystems(t *testing.T) {
	for _, tt := range []struct {
		name, manifest, content, family string
		members                         []string
	}{
		{"Django", "requirements.txt", "django==5.2\ndjangorestframework==3.16\ndjango-filter==25.1\n", "Django", []string{"pypi:django", "pypi:djangorestframework", "pypi:django-filter"}},
		{"Rails", "Gemfile", "gem 'rails', '8'\ngem 'activerecord', '8'\ngem 'actionmailer', '8'\n", "Ruby on Rails", []string{"rails", "activerecord", "gem:actionmailer"}},
		{"Laravel", "composer.json", `{"require":{"laravel/framework":"^12","illuminate/support":"^12"}}`, "Laravel", []string{"laravel", "composer:illuminate/support"}},
		{"Go SDK", "go.mod", "module example.org/app\n\nrequire (\n github.com/aws/aws-sdk-go-v2/service/s3 v1.0.0\n github.com/aws/aws-sdk-go-v2/config v1.0.0\n)\n", "Amazon Web Services", []string{"go:github.com/aws/aws-sdk-go-v2/service/s3", "go:github.com/aws/aws-sdk-go-v2/config"}},
		{"Vue", "package.json", `{"dependencies":{"vue":"3","vue-router":"4","pinia":"3"}}`, "Vue", []string{"npm:vue", "npm:vue-router", "npm:pinia"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, tt.manifest, tt.content)
			inv := scanTestRepository(t, root, ScanOptions{})
			r := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
			var group *Finding
			for i := range r.Highlights {
				if r.Highlights[i].Name == tt.family {
					group = &r.Highlights[i]
				}
			}
			if group == nil {
				t.Fatalf("no %s family in %+v", tt.family, r.Highlights)
			}
			for _, id := range tt.members {
				if !contains(group.Members, id) || len(finding(t, inv, id).Evidence) == 0 {
					t.Fatalf("family lost member or evidence: %s", id)
				}
			}
			if group.Decision != (Decision{}) || len(r.Decisions) != 0 {
				t.Fatal("grouping created a team decision")
			}
		})
	}
}

func TestGroupingKeepsDifferentComponentsAndDecisionsSeparate(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19"}}`)
	fixture(t, root, "packages/widgets/package.json", `{"dependencies":{"@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
	for _, id := range []string{"react", "npm:@radix-ui/react-dialog"} {
		if len(finding(t, inv, id).Evidence) == 0 {
			t.Fatalf("component lost evidence: %s", id)
		}
	}
	parent := finding(t, Inventory{Findings: r.Highlights}, "react")
	if !contains(parent.Members, "react") || contains(parent.Members, "npm:@radix-ui/react-dialog") {
		t.Fatal("grouping lost the parent or merged separate components")
	}
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv = scanTestRepository(t, root, ScanOptions{})
	d := Decision{Status: "proposed", Name: "Dialog UI", Quadrant: "Languages & Frameworks", Ring: "Caution", Scope: "Dialogs", Rationale: "Migration required"}
	r = buildTestReport(t, inv, Policy{Rules: map[string]Decision{"npm:@radix-ui/react-dialog": d}}, nil, "Example", BuildOptions{})
	react := finding(t, Inventory{Findings: r.Highlights}, "react")
	dialog := finding(t, Inventory{Findings: r.Highlights}, "npm:@radix-ui/react-dialog")
	if contains(react.Members, dialog.ID) || dialog.Decision != d {
		t.Fatal("grouping hid an explicit child decision")
	}
}

func TestFamilyPlacementSurvivesRepresentativeReplacement(t *testing.T) {
	old := Finding{ID: "cargo:tokio", FamilyID: "rust-async", GroupComponent: "service", Quadrant: "Languages & Frameworks", Members: []string{"cargo:tokio"}, Proposal: &Decision{Ring: "Assess"}}
	next := Finding{ID: "cargo:tokio-stream", FamilyID: "rust-async", GroupComponent: "service", Quadrant: old.Quadrant, Members: []string{"cargo:tokio-stream"}, Proposal: &Decision{Ring: "Trial"}}
	prior := Report{Highlights: []Finding{old}}
	report := Report{Highlights: []Finding{next}}
	comparePlacements(&report, &prior, GroupingRules{})
	if got := report.Highlights[0].PlacementChange; got == nil || got.Kind != "moved-in" {
		t.Fatalf("family lost comparison: %+v", got)
	}
	report.Highlights[0].GroupComponent = "other"
	comparePlacements(&report, &prior, GroupingRules{})
	if got := report.Highlights[0].PlacementChange; got == nil || got.Kind != "new" {
		t.Fatalf("cross-component family matched: %+v", got)
	}
}

func TestGroupingValidationAndCanonicalInputs(t *testing.T) {
	a, err := ReadGroupingRules([]byte(`{"schema_version":1,"families":[{"id":"react","disabled":true},{"id":"django","disabled":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReadGroupingRules([]byte(`{ "families": [{"disabled":true,"id":"django"},{"disabled":true,"id":"react"}], "schema_version":1 }`))
	if err != nil || a.fingerprint != b.fingerprint || a.catalogDigest != b.catalogDigest {
		t.Fatalf("format/order changed meaning: %v", err)
	}
	for _, family := range a.families {
		if family.ID == "react" || family.ID == "django" {
			t.Fatal("disabled family retained")
		}
	}
	for _, data := range []string{
		`{"schema_version":1,"families":[{"id":"unknown","disabled":true}]}`,
		`{"schema_version":1,"families":[{"id":"react","disabled":true,"ring":"Adopt"}]}`,
		`{"schema_version":1,"families":[{"id":"react","disabled":true,"disabled":false}]}`,
		`{"schema_version":1,"families":[{"id":"other","name":"Other","quadrant":"Tools","significance":"workflow_enabling","relationship":"workflow_tools","scope":"repository","prefixes":["npm:@radix-ui/react-"]}]}`,
	} {
		if _, err := ReadGroupingRules([]byte(data)); err == nil {
			t.Fatalf("accepted invalid input: %s", data)
		}
	}
}

func TestGroupingUsesRuntimeMemberWhenRepresentativeIsDevelopmentOnly(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "pyproject.toml", "[project]\ndependencies = [\"djangorestframework>=3\"]\n[project.optional-dependencies]\ntest = [\"django>=5\"]\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	report := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
	for _, f := range report.Highlights {
		if f.FamilyID == "django" {
			return
		}
	}
	t.Fatal("development representative hid the runtime family")
}

func TestGroupingRulesAreSafeForConcurrentRuns(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	baseline := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
	for _, name := range []string{"First UI", "Second UI", "Third UI"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for range 4 {
				grouping, err := ReadGroupingRules([]byte(fmt.Sprintf(`{"schema_version":1,"families":[{"id":"react","name":%q,"quadrant":"Languages & Frameworks","significance":"system_defining","relationship":"framework_extensions","scope":"component","members":["react","npm:@radix-ui/react-dialog"]}]}`, name)))
				if err != nil {
					t.Fatal(err)
				}
				r := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{Grouping: grouping})
				f := finding(t, Inventory{Findings: r.Highlights}, "react")
				if f.Name != name || !contains(f.Members, "npm:@radix-ui/react-dialog") {
					t.Fatalf("grouping override leaked or lost members: %#v", f)
				}
				defaults := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
				if !reflect.DeepEqual(defaults, baseline) {
					t.Fatal("override mutated built-in grouping")
				}
			}
		})
	}
}

func TestNewAliasRecognitionPreservesSavedPackageIDs(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"prisma":"6","@prisma/client":"6","react-native":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	old := Report{SchemaVersion: 2, GeneratorVersion: Version, Source: inv.Source, Repository: "Example", Decisions: map[string]Decision{}, Findings: []Finding{
		{ID: "npm:prisma", Name: "npm:prisma", Presence: "detected", Number: 1},
		{ID: "npm:@prisma/client", Name: "npm:@prisma/client", Presence: "detected", Number: 2},
		{ID: "npm:react-native", Name: "npm:react-native", Presence: "detected", Number: 3},
	}}
	r := buildTestReport(t, inv, Policy{}, &old, "", BuildOptions{})
	for _, id := range []string{"npm:prisma", "npm:@prisma/client", "npm:react-native"} {
		found := false
		for _, f := range r.Findings {
			if f.ID == id {
				found = f.Presence == "detected" && len(f.Evidence) > 0
			}
		}
		if !found {
			t.Fatalf("saved ID lost its detection: %s", id)
		}
	}
	for _, f := range r.Findings {
		if f.ID == "prisma" || f.ID == "react-native" {
			t.Fatalf("duplicate canonical ID: %s", f.ID)
		}
	}
	next := buildTestReport(t, inv, Policy{}, &r, "", BuildOptions{})
	a, _ := Render(r, "json")
	b, _ := Render(next, "json")
	if string(a) != string(b) {
		t.Fatal("legacy ID replay changed")
	}
}

func TestLegacyAliasesStillDetectRemovedPackages(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"@prisma/client":"6"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	old := Report{SchemaVersion: 2, GeneratorVersion: Version, Source: inv.Source, Decisions: map[string]Decision{}, Findings: []Finding{{ID: "npm:prisma", Name: "npm:prisma", Presence: "detected", Number: 1}, {ID: "npm:@prisma/client", Name: "npm:@prisma/client", Presence: "detected", Number: 2}}}
	report := buildTestReport(t, inv, Policy{}, &old, "", BuildOptions{})
	if finding(t, Inventory{Findings: report.Findings}, "npm:prisma").Presence != "not detected" {
		t.Fatal("removed alias still detected through another alias")
	}
}

func TestAliasRecognitionKeepsExclusionsAndDecisions(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react-native":"1","prisma":"6"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	exclusion := Decision{Exclude: true, Rationale: "Reviewed separately"}
	decision := Decision{Ring: "Caution", Status: "proposed", Scope: "Mobile client", Rationale: "Migration planned"}
	old := Report{SchemaVersion: 2, GeneratorVersion: Version, Source: inv.Source, Decisions: map[string]Decision{"npm:prisma": exclusion, "npm:react-native": decision}, Findings: []Finding{{ID: "npm:react-native", Name: "npm:react-native", Presence: "detected", Number: 1, Decision: decision}}, Excluded: []string{"npm:prisma"}, ExcludedFindings: []Finding{{ID: "npm:prisma", Name: "npm:prisma", Presence: "detected", Decision: exclusion}}}
	r := buildTestReport(t, inv, Policy{}, &old, "", BuildOptions{})
	if !contains(r.Excluded, "npm:prisma") || len(r.ExcludedFindings) != 1 || r.ExcludedFindings[0].Presence != "detected" {
		t.Fatal("alias lost exclusion")
	}
	found := false
	for _, f := range r.Highlights {
		if f.ID == "npm:react-native" {
			found = f.Decision == decision && f.Number == 1
		}
	}
	if !found {
		t.Fatal("alias lost explicit decision")
	}
	if _, err := Render(r, "json"); err != nil {
		t.Fatal(err)
	}
}

func TestGroupingRejectsAliasOwnershipConflicts(t *testing.T) {
	_, err := ReadGroupingRules([]byte(`{"schema_version":1,"families":[{"id":"other","name":"Other","quadrant":"Tools","significance":"workflow_enabling","relationship":"workflow_tools","scope":"component","members":["npm:react"]}]}`))
	if err == nil {
		t.Fatal("alias assigned to two families")
	}
}

func TestLegacyAliasUpdateKeepsNewPackageEvidence(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"prisma":"6","@prisma/client":"6"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	old := Report{SchemaVersion: 2, GeneratorVersion: Version, Source: inv.Source, Decisions: map[string]Decision{}, Findings: []Finding{{ID: "npm:prisma", Name: "npm:prisma", Presence: "detected", Number: 1}}}
	r := buildTestReport(t, inv, Policy{}, &old, "", BuildOptions{})
	found := false
	for _, f := range r.Findings {
		for _, e := range f.Evidence {
			found = found || strings.HasPrefix(e.Detail, "npm package @prisma/client:")
		}
	}
	if !found {
		t.Fatal("new alias evidence disappeared during legacy ID preservation")
	}
}

func TestRawPackagePolicyWorksWithoutPreviousReport(t *testing.T) {
	for _, decision := range []Decision{{Exclude: true, Rationale: "Reviewed separately"}, {Ring: "Caution", Status: "proposed", Scope: "Mobile", Rationale: "Migration planned"}} {
		root := t.TempDir()
		fixture(t, root, "package.json", `{"dependencies":{"react-native":"1"}}`)
		inv := scanTestRepository(t, root, ScanOptions{})
		r := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"npm:react-native": decision}}, nil, "Example", BuildOptions{})
		if decision.Exclude {
			if !contains(r.Excluded, "npm:react-native") || len(r.Highlights) != 0 {
				t.Fatal("raw exclusion bypassed by canonical finding")
			}
		} else {
			found := false
			for _, f := range r.Highlights {
				found = found || (f.ID == "npm:react-native" && f.Decision == decision)
			}
			if !found {
				t.Fatal("raw policy lost its finding")
			}
		}
	}
}

func TestMovedComponentDoesNotMatchOldFamilyMembers(t *testing.T) {
	old := Finding{ID: "cargo:tokio", FamilyID: "rust-async", GroupComponent: "service-a", Quadrant: "Languages & Frameworks", Members: []string{"cargo:tokio", "cargo:tokio-stream"}, Proposal: &Decision{Ring: "Assess"}}
	next := old
	next.GroupComponent = "service-b"
	r := Report{Highlights: []Finding{next}}
	prior := Report{Highlights: []Finding{old}}
	comparePlacements(&r, &prior, GroupingRules{})
	if c := r.Highlights[0].PlacementChange; c == nil || c.Kind != "new" {
		t.Fatalf("component change ignored: %+v", c)
	}
}

func TestAllBuiltInFamiliesCanBeDisabled(t *testing.T) {
	defaults, err := defaultGrouping()
	if err != nil {
		t.Fatal(err)
	}
	config := groupingConfig{SchemaVersion: 1, Families: []groupingFamily{}}
	for _, family := range defaults.families {
		config.Families = append(config.Families, groupingFamily{ID: family.ID, Disabled: true})
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	grouping, err := ReadGroupingRules(data)
	if err != nil {
		t.Fatal(err)
	}
	if grouping.families == nil || len(grouping.families) != 0 {
		t.Fatal("explicit empty catalog not retained")
	}
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{Grouping: grouping})
	for _, f := range r.Highlights {
		if f.FamilyID != "" || len(f.Members) > 1 {
			t.Fatal("disabled catalog fell back to built-ins")
		}
	}
}
