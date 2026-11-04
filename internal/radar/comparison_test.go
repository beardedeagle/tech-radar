package radar

import (
	"reflect"
	"strings"
	"testing"
)

func TestRadarSymbolsComparePlacementRatherThanEvidence(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	for _, tt := range []struct {
		name, from, to, change string
	}{
		{"moves inward", "Assess", "Adopt", "moved-in"},
		{"moves outward", "Adopt", "Caution", "moved-out"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policy := Policy{Rules: map[string]Decision{"phoenix": {Ring: tt.from, Scope: "Web app", Rationale: "Team choice"}}}
			previous := buildTestReport(t, inv, policy, nil, "Demo", BuildOptions{})
			policy.Rules["phoenix"] = Decision{Ring: tt.to, Scope: "Web app", Rationale: "Updated team choice"}
			r := buildTestReport(t, inv, policy, &previous, "Demo", BuildOptions{})
			f := finding(t, Inventory{Findings: r.Findings}, "phoenix")
			if f.PlacementChange == nil || f.PlacementChange.Kind != tt.change || f.PlacementChange.PreviousRing != tt.from {
				t.Fatalf("incorrect placement comparison: %+v", f.PlacementChange)
			}
			if f.EvidenceChanged {
				t.Fatal("policy movement incorrectly changed evidence")
			}
			data, err := RenderDocument(r, "html", "evidence.json", strings.Repeat("a", 64))
			if err != nil {
				t.Fatal(err)
			}
			saved, err := ReadReport(data)
			if err != nil {
				t.Fatal(err)
			}
			if got := finding(t, Inventory{Findings: saved.Findings}, "phoenix"); !reflect.DeepEqual(got.PlacementChange, f.PlacementChange) {
				t.Fatal("export lost the placement comparison")
			}
			replay := buildTestReport(t, inv, policy, &saved, "Demo", BuildOptions{})
			again, err := RenderDocument(replay, "html", "evidence.json", strings.Repeat("a", 64))
			if err != nil || string(data) != string(again) {
				t.Fatal("unchanged replay erased movement or changed the document")
			}
		})
	}
	previous := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	changed := inv
	changed.Findings = append([]Finding(nil), inv.Findings...)
	for i := range changed.Findings {
		if changed.Findings[i].ID == "phoenix" {
			changed.Findings[i].EvidenceFingerprint = strings.Repeat("b", 64)
		}
	}
	r := buildTestReport(t, changed, Policy{}, &previous, "Demo", BuildOptions{})
	f := finding(t, Inventory{Findings: r.Findings}, "phoenix")
	if !f.EvidenceChanged || f.PlacementChange == nil || f.PlacementChange.Kind != "unchanged" {
		t.Fatal("changed evidence was mistaken for movement")
	}
}

func TestFamilyRepresentativeChangeDoesNotInventANewTechnology(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "mix.exs", "defp deps do\n[\n{:ex_aws_s3, \"2\"},\n{:ex_aws_kms, \"2\"}\n]\nend\n")
	fixture(t, root, "lib/storage.ex", "ExAws.S3.get_object(bucket, key)\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	previous := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	fixture(t, root, ".github/workflows/deploy.yml", "jobs:\n  deploy:\n    steps:\n      - run: aws ecs describe-services\n")
	inv = scanTestRepository(t, root, ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, &previous, "Demo", BuildOptions{})
	for _, h := range r.Highlights {
		if h.Name == "Amazon Web Services" {
			if h.ID != "amazon-ecs" || h.PlacementChange == nil || h.PlacementChange.Kind != "unchanged" {
				t.Fatalf("changing the representative invented a new AWS entry: %+v", h.PlacementChange)
			}
			return
		}
	}
	t.Fatal("missing AWS family")
}

func TestRadarComparisonHandlesNewEntriesAndMissingHistory(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	first := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	for _, f := range first.Highlights {
		if f.PlacementChange == nil || f.PlacementChange.Kind != "new" || f.PlacementChange.PreviousRing != "" {
			t.Fatal("first report should mark entries as new to the radar without inventing prior placements")
		}
	}
	firstHTML, err := RenderDocument(first, "html", "evidence.json", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	chart := strings.SplitN(string(firstHTML), "</svg>", 2)[0]
	if strings.Count(chart, `r="18"`) != len(first.Highlights) {
		t.Fatal("first-run chart did not draw an outer ring for every entry")
	}
	baseline := first
	baseline.Findings = nil
	baseline.Highlights = nil
	newReport := buildTestReport(t, inv, Policy{}, &baseline, "Demo", BuildOptions{})
	for _, f := range newReport.Highlights {
		if f.PlacementChange == nil || f.PlacementChange.Kind != "new" {
			t.Fatal("new radar entry has no new-entry symbol")
		}
	}
	data, err := RenderDocument(first, "html", "evidence.json", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	compact, err := ReadReport(data)
	if err != nil {
		t.Fatal(err)
	}
	// Older exports omitted placement summaries along with their companion.
	compact.Highlights = nil
	inv.Source.GitHead = strings.Repeat("a", 40)
	next := buildTestReport(t, inv, Policy{}, &compact, "Demo", BuildOptions{})
	for _, f := range next.Highlights {
		if f.PlacementChange != nil {
			t.Fatal("missing companion was treated as known placement history")
		}
	}
}
