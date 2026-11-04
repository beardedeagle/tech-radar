package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCarriedDecisionProvenanceAndVisibleHistory(t *testing.T) {
	inv := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19.0.0"}}`).Inventory
	for _, tt := range []struct {
		name     string
		model    Model
		decision Decision
	}{
		{"note only", RecommendationModel, Decision{Lifecycle: "retire"}},
		{"scoped note", RecommendationModel, Decision{Lifecycle: "Retire", Scope: "old recommendation scope", Rationale: "old recommendation rationale"}},
		{"historical approval", RecommendationModel, Decision{Ring: "Adopt", Scope: "approved UI scope", Rationale: "old approval rationale", Status: "approved", Owner: "original UI owner", Reference: "DEC-original", Reviewed: "2026-11-01"}},
		{"retirement proposal", LifecycleModel, Decision{Lifecycle: "Retire", Scope: "old lifecycle scope", Rationale: "supplied retirement rationale", Status: "proposed", Owner: "lifecycle owner", Reference: "DEC-retire", Reviewed: "2026-11-02"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := buildTestReport(t, inv, Policy{Model: tt.model, Rules: map[string]Decision{"react": tt.decision}}, nil, "Example", BuildOptions{})
			// Reports from before provenance existed retain their saved report meaning.
			if tt.model == RecommendationModel {
				original.Model = ""
			}
			target := LifecycleModel
			if tt.model == LifecycleModel {
				target = RecommendationModel
			}
			r := buildTestReport(t, inv, Policy{}, &original, "", BuildOptions{Model: target})
			want := original.Decisions["react"]
			want.Model = tt.model
			f := finding(t, Inventory{Findings: r.Findings}, "react")
			if f.Decision != want || !f.ReviewRequired || f.EvidenceChanged || f.PlacementChange != nil {
				t.Fatalf("carried decision lost provenance or review state: %#v", f)
			}
			if target == LifecycleModel {
				if target.ring(f) != "Needs decision" || target.displayDecision(f) != (Decision{}) || !strings.HasPrefix(target.status(f), "unreviewed") || f.MissingDecisionMaterial != (want.Reference == "") {
					t.Fatalf("history acquired lifecycle authority: %#v", f)
				}
			} else if f.Proposal == nil || target.ring(f) != "Assess" || target.displayDecision(f).Status != "suggested" || !strings.HasPrefix(target.status(f), "suggested") {
				t.Fatalf("inferred placement borrowed lifecycle authority: %#v", f)
			}
			for _, view := range []View{RadarView, TableView} {
				r.View = view
				for _, format := range []string{"markdown", "html"} {
					for _, compact := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/compact=%t", view, format, compact), func(t *testing.T) {
							data := renderTestExport(t, r, format, compact)
							body := strings.SplitN(string(data), snapshotStart, 2)[0]
							for _, field := range []decisionField{
								{"Historical decision model", string(tt.model)}, {"Historical status", want.Status},
								{"Historical ring", want.Ring}, {"Historical scope", want.Scope},
								{"Historical rationale", want.Rationale}, {"Historical lifecycle note", want.Lifecycle},
								{"Historical owner", want.Owner}, {"Historical decision reference", want.Reference},
								{"Historical reviewed", want.Reviewed},
							} {
								if field.Value != "" && (!strings.Contains(body, field.Label) || !strings.Contains(body, field.Value)) {
									t.Fatalf("visible history omitted %s = %q", field.Label, field.Value)
								}
							}
							if target == RecommendationModel && (!strings.Contains(body, "Suggested rationale") || !strings.Contains(body, displayReason(f.Proposal.Rationale))) {
								t.Fatal("visible inferred rationale missing its suggested label")
							}
							prior := readTestReport(t, data)
							if prior.Decisions["react"] != want || prior.Source.Fingerprint != r.Source.Fingerprint {
								t.Fatal("snapshot changed saved provenance or evidence fingerprint")
							}
							replayed := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{})
							if replayed.Decisions["react"] != want || target.ring(finding(t, Inventory{Findings: replayed.Findings}, "react")) != target.ring(f) {
								t.Fatal("same-model compact recovery changed decision meaning")
							}
						})
					}
				}
			}
			prior := readTestReport(t, renderTestExport(t, r, "json", false))
			if want.Status != "approved" {
				back := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{Model: tt.model})
				again := buildTestReport(t, inv, Policy{}, &back, "", BuildOptions{Model: target})
				if back.Decisions["react"] != want || again.Decisions["react"] != want {
					t.Fatal("repeated switches rewrote original decision provenance")
				}
			}
		})
	}
}

func TestLegacyCanonicalNotesStayHistoricalAndApprovalSwitchGatesRemain(t *testing.T) {
	inv := inventoryForTests()
	for _, note := range []string{"retire", "Adopt", "eXpErImEnT", "Hold", "legacy note"} {
		t.Run(note, func(t *testing.T) {
			original := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"phoenix": {Lifecycle: note}}}, nil, "", BuildOptions{})
			r := buildTestReport(t, inv, Policy{}, &original, "", BuildOptions{Model: LifecycleModel})
			f := finding(t, Inventory{Findings: r.Findings}, "phoenix")
			if r.Model.ring(f) != "Needs decision" || f.Decision.Lifecycle != note || f.Decision.Model != RecommendationModel {
				t.Fatal("canonical-looking note acquired lifecycle placement")
			}
			readTestReport(t, renderTestExport(t, r, "json", false))
			approved := Decision{Ring: "Adopt", Lifecycle: note, Scope: "web", Rationale: "reviewed old recommendation", Status: "approved", Owner: "Team", Reference: "DEC-1", Reviewed: "2026-11-01"}
			original = buildTestReport(t, inv, Policy{Rules: map[string]Decision{"phoenix": approved}}, nil, "", BuildOptions{})
			_, err := BuildWithOptions(inv, Policy{}, &original, "", BuildOptions{Model: LifecycleModel})
			if (canonicalLifecycle(note) != "") != (err != nil) {
				t.Fatalf("existing canonical-note approval gate changed: %v", err)
			}
			replacement := Decision{Lifecycle: "Hold", Scope: "new scope", Rationale: "new target decision"}
			r = buildTestReport(t, inv, Policy{Model: LifecycleModel, Rules: map[string]Decision{"phoenix": replacement}}, &original, "", BuildOptions{})
			replacement.Status = "proposed"
			if r.Decisions["phoenix"] != replacement || r.Model.ring(finding(t, Inventory{Findings: r.Findings}, "phoenix")) != "Hold" {
				t.Fatal("target replacement retained historical approval metadata")
			}
		})
	}
}

func TestDecisionProvenanceValidationAndLegacyReadCompatibility(t *testing.T) {
	inv := inventoryForTests()
	for _, origin := range []Model{"unknown", LifecycleModel} {
		policy := fmt.Sprintf(`{"schema_version":1,"rules":{"phoenix":{"model":%q,"lifecycle":"Retire","scope":"web","rationale":"supplied"}}}`, origin)
		if _, err := ReadPolicyForModel([]byte(policy), RecommendationModel); err == nil {
			t.Fatalf("policy accepted mismatched or unknown provenance %q", origin)
		}
	}
	legacy := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"phoenix": {Ring: "Trial", Lifecycle: "legacy phase", Scope: "web", Rationale: "original recommendation"}}}, nil, "", BuildOptions{})
	// Old lifecycle exports carried these recommendation records without metadata.
	legacy.Model = LifecycleModel
	legacy.Baseline = true
	for i := range legacy.Findings {
		legacy.Findings[i].Proposal = nil
	}
	for i := range legacy.Highlights {
		legacy.Highlights[i].Proposal = nil
	}
	for _, format := range []string{"json", "markdown", "html"} {
		data := renderTestExport(t, legacy, format, false)
		prior := readTestReport(t, data)
		if prior.Decisions["phoenix"].Model != "" || prior.Decisions["phoenix"] != legacy.Decisions["phoenix"] {
			t.Fatal("legacy read rewrote saved decision metadata")
		}
		if !bytes.Equal(data, renderTestExport(t, prior, format, false)) {
			t.Fatal("reading legacy export rewrote its saved hash or state")
		}
	}
	for _, origin := range []Model{"unknown", LifecycleModel} {
		data := renderTestExport(t, legacy, "json", false)
		var saved map[string]any
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		saved["decisions"].(map[string]any)["phoenix"].(map[string]any)["model"] = origin
		data, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReadReport(data); err == nil {
			t.Fatalf("saved report accepted unknown or incompatible provenance %q", origin)
		}
	}
	if note := modelReviewNote(Report{Model: LifecycleModel}); !strings.Contains(note, "missing a decision reference") || strings.Contains(note, "every included technology") {
		t.Fatalf("subsequent-generation note overstates reference gap: %s", note)
	}
}

func TestInferredRecommendationKeepsSameModelRecordedMaterialVisible(t *testing.T) {
	inv := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19.0.0"}}`).Inventory
	d := Decision{Scope: "recorded scope", Rationale: "recorded rationale", Lifecycle: "maintain", Owner: "recorded owner", Reference: "DEC-recorded", Reviewed: "2026-11-01", Status: "proposed"}
	r := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"react": d}}, nil, "", BuildOptions{})
	f := finding(t, Inventory{Findings: r.Findings}, "react")
	if active := r.Model.displayDecision(f); active.Status != "suggested" || active.Owner != "" || active.Reference != "" || active.Rationale == d.Rationale {
		t.Fatalf("inference merged recorded authority: %#v", active)
	}
	for _, view := range []View{RadarView, TableView} {
		r.View = view
		for _, format := range []string{"markdown", "html"} {
			for _, compact := range []bool{false, true} {
				body := strings.SplitN(string(renderTestExport(t, r, format, compact)), snapshotStart, 2)[0]
				for _, field := range r.Model.historicalDecision(f) {
					if field.Value != "" && (!strings.Contains(body, field.Label) || !strings.Contains(body, field.Value)) {
						t.Fatalf("%s/%s/compact=%t omitted recorded %s", view, format, compact, field.Label)
					}
				}
			}
		}
	}
}

func TestLifecycleGroupingKeepsDistinctDecisionOrigins(t *testing.T) {
	inv := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19.0.0","@radix-ui/react-dialog":"1.0.0"}}`).Inventory
	d := Decision{Lifecycle: "Hold", Scope: "UI component", Rationale: "Team review", Status: "proposed"}
	old := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}, nil, "", BuildOptions{})
	d.Model = LifecycleModel
	r := buildTestReport(t, inv, Policy{Model: LifecycleModel, Rules: map[string]Decision{"npm:@radix-ui/react-dialog": d}}, &old, "", BuildOptions{})
	parent := finding(t, Inventory{Findings: r.Highlights}, "react")
	child := finding(t, Inventory{Findings: r.Highlights}, "npm:@radix-ui/react-dialog")
	if contains(parent.Members, child.ID) || r.Model.ring(parent) != "Needs decision" || r.Model.ring(child) != "Hold" {
		t.Fatal("grouping merged historical recommendation and active lifecycle authority")
	}
	readTestReport(t, renderTestExport(t, r, "json", false))
}

func TestSuppliedLifecycleRingOnlyDecisionKeepsOrigin(t *testing.T) {
	inv := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19.0.0"}}`).Inventory
	policy, err := ReadPolicyForModel([]byte(`{"schema_version":1,"model":"lifecycle","rules":{"react":{"ring":"Trial","scope":"UI","rationale":"Lifecycle policy supplied record"}}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	old := buildTestReport(t, inv, Policy{Rules: map[string]Decision{"react": {Ring: "Adopt", Lifecycle: "Retire", Scope: "old scope", Rationale: "old approval", Status: "approved", Owner: "old owner", Reference: "DEC-old", Reviewed: "2026-11-01"}}}, nil, "", BuildOptions{})
	for _, previous := range []*Report{nil, &old} {
		t.Run(fmt.Sprintf("replacement=%t", previous != nil), func(t *testing.T) {
			r := buildTestReport(t, inv, policy, previous, "", BuildOptions{})
			want := policy.Rules["react"]
			want.Model, want.Status = LifecycleModel, "proposed"
			f := finding(t, Inventory{Findings: r.Findings}, "react")
			if f.Decision != want || r.Model.ring(f) != "Needs decision" || len(r.Model.historicalDecision(f)) != 0 || r.Baseline != (previous == nil) {
				t.Fatalf("fresh policy lost selected lifecycle origin: %#v", f)
			}
			prior := readTestReport(t, renderTestExport(t, r, "json", false))
			next := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{Model: RecommendationModel})
			f = finding(t, Inventory{Findings: next.Findings}, "react")
			if f.Decision != want || f.Proposal == nil || next.Model.ring(f) != "Assess" || next.Model.displayDecision(f).Status != "suggested" {
				t.Fatalf("lifecycle ring supplied active recommendation authority: %#v", f)
			}
			body := strings.SplitN(string(renderTestExport(t, next, "markdown", false)), snapshotStart, 2)[0]
			for _, text := range []string{"**Ring:** Assess", "**Decision:** suggested", "Suggested rationale", "Historical decision model:** lifecycle", "Historical ring:** Trial", "Historical rationale:** Lifecycle policy supplied record"} {
				if !strings.Contains(body, text) {
					t.Fatalf("switched document omitted %q", text)
				}
			}
		})
	}
}
