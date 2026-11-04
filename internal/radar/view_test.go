package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"strings"
	"testing"
)

func TestViewSelectionAndLegacyDefaults(t *testing.T) {
	inv := inventoryForTests()
	for _, model := range []Model{RecommendationModel, LifecycleModel} {
		want := RadarView
		if model == LifecycleModel {
			want = TableView
		}
		first, err := BuildWithOptions(inv, Policy{}, nil, "", BuildOptions{Model: model})
		if err != nil || first.View != want {
			t.Fatalf("%s default view: %q, %v", model, first.View, err)
		}
		for _, view := range []View{RadarView, TableView} {
			prior := first
			prior.View = view
			inherited, err := BuildWithOptions(inv, Policy{}, &prior, "", BuildOptions{})
			if err != nil || inherited.View != view {
				t.Fatalf("view inheritance: %q, %v", inherited.View, err)
			}
			explicit, err := BuildWithOptions(inv, Policy{}, &prior, "", BuildOptions{View: want})
			if err != nil || explicit.View != want {
				t.Fatalf("explicit view: %q, %v", explicit.View, err)
			}
		}
		first.View = ""
		data, _ := json.Marshal(first)
		legacy, err := ReadReport(data)
		if err != nil || legacy.selectedView() != want {
			t.Fatalf("legacy view: %q, %v", legacy.selectedView(), err)
		}
		other := RecommendationModel
		if model == RecommendationModel {
			other = LifecycleModel
		}
		first.View = want
		switched, err := BuildWithOptions(inv, Policy{}, &first, "", BuildOptions{Model: other})
		if err != nil || switched.View == want {
			t.Fatalf("cross-model view inherited: %q, %v", switched.View, err)
		}
	}
	if _, err := BuildWithOptions(inv, Policy{}, nil, "", BuildOptions{View: "grid"}); err == nil {
		t.Fatal("invalid view accepted")
	}
}

func TestViewRoundTripsDoNotChangeDecisionOrReviewState(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	first := buildTestReport(t, inv, lifecyclePolicy(""), nil, "Example", BuildOptions{})
	prior := buildTestReport(t, inv, Policy{}, &first, "", BuildOptions{})
	for _, view := range []View{RadarView, TableView} {
		r := buildTestReport(t, inv, Policy{}, &prior, "", BuildOptions{View: view})
		unchanged := r
		unchanged.View = prior.View
		if !reflect.DeepEqual(prior, unchanged) {
			t.Fatal("view change altered model, movement, evidence or decisions")
		}
		for _, format := range []string{"json", "markdown", "html"} {
			for _, compact := range []bool{false, true} {
				if compact && format == "json" {
					continue
				}
				data := renderTestExport(t, r, format, compact)
				recovered, err := ReadReport(data)
				if err != nil || recovered.View != view {
					t.Fatalf("%s compact=%v view lost: %v", format, compact, err)
				}
				next, err := BuildWithOptions(inv, Policy{}, &recovered, "", BuildOptions{})
				if err != nil || !reflect.DeepEqual(r, next) {
					t.Fatalf("%s compact=%v replay changed state: %v", format, compact, err)
				}
				again, _ := Render(next, format)
				if !compact && !bytes.Equal(data, again) {
					t.Fatal("identical replay changed output")
				}
			}
		}
	}
	prior.View = "grid"
	data, _ := json.Marshal(prior)
	if _, err := ReadReport(data); err == nil {
		t.Fatal("saved invalid view accepted")
	}
}

func TestDecisionTableGroupsMembersAndEscapesSuppliedText(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1","unrecognized-helper":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	d := Decision{Lifecycle: "Hold", Scope: "UI | <scope>", Rationale: "Review [link](javascript:alert(1))", Owner: "UI <team>", Status: "approved", Reference: "docs/<script>decision</script>|record", Reviewed: "2026-11-03"}
	r := buildTestReport(t, inv, Policy{Model: LifecycleModel, Rules: map[string]Decision{"react": d, "npm:@radix-ui/react-dialog": d}}, nil, "Example", BuildOptions{})
	groups := decisionGroups(r, false)
	count := 0
	for _, group := range groups {
		for _, f := range group.Findings {
			for _, member := range tableMembers(findingsByID(r), f) {
				count++
				if member.ID == "" {
					t.Fatal("missing member")
				}
			}
		}
	}
	if count != len(r.Findings) {
		t.Fatalf("table omitted or duplicated members: %d of %d", count, len(r.Findings))
	}
	for _, format := range []string{"markdown", "html"} {
		for _, compact := range []bool{false, true} {
			data := renderTestExport(t, r, format, compact)
			body := strings.SplitN(string(data), snapshotStart, 2)[0]
			for _, value := range []string{"Hold", "2026-11-03", "npm:@radix-ui/react-dialog", "Needs decision"} {
				if !strings.Contains(body, value) {
					t.Fatalf("%s compact=%v omitted %s", format, compact, value)
				}
			}
			if strings.Contains(body, "<script>decision</script>") || strings.Contains(body, `href="javascript:`) || strings.Contains(body, "## Register") || strings.Contains(body, `<div class="register">`) {
				t.Fatal("unsafe markup or redundant register in table")
			}
			if compact && !strings.Contains(body, "evidence.json") {
				t.Fatal("compact table omitted companion link")
			}
			if !compact && !strings.Contains(body, "package.json") {
				t.Fatal("full table omitted member evidence")
			}
			if format == "html" {
				if !strings.Contains(body, `class="state-badge state-hold"`) ||
					!strings.Contains(body, `<details>`) ||
					!strings.Contains(body, `<summary>Rationale, members and evidence</summary>`) ||
					!strings.Contains(body, `scope="col">Reviewed`) ||
					!strings.Contains(body, `role="region" tabindex="0"`) ||
					strings.Contains(body, `const tooltip = document.getElementById`) {
					t.Fatal("table missing accessible controls or retained radar script")
				}
			} else if strings.Contains(body, "[link](javascript:") || strings.Contains(body, "UI |") {
				t.Fatal("Markdown table accepted user link syntax or column delimiter")
			}
		}
	}
}

func TestCompactTableKeepsExistingRecommendationHighlights(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19","unrecognized-helper":"1"}}`)
	inv := scanTestRepository(t, root, ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "", BuildOptions{View: TableView})
	for _, concise := range []bool{false, true} {
		count := 0
		for _, group := range decisionGroups(r, concise) {
			count += len(group.Findings)
		}
		if concise && count != len(r.Highlights) {
			t.Fatal("compact table changed prioritized highlights")
		}
		if !concise && count <= len(r.Highlights) {
			t.Fatal("full table omitted supporting observation")
		}
	}
}

func TestLifecycleTablePreservesCarriedLegacyNotes(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	for _, note := range []string{"maintain", "legacy <script>phase</script>|note"} {
		policy := Policy{Rules: map[string]Decision{"phoenix": {Lifecycle: note}}}
		original := buildTestReport(t, inv, policy, nil, "Example", BuildOptions{})
		r := buildTestReport(t, inv, Policy{}, &original, "", BuildOptions{Model: LifecycleModel})
		f := finding(t, Inventory{Findings: r.Findings}, "phoenix")
		if r.View != TableView || r.Model.ring(f) != "Needs decision" || f.Decision.Status != "" || f.Decision.Lifecycle != note {
			t.Fatal("carried legacy note changed decision meaning")
		}
		for _, format := range []string{"html", "markdown"} {
			for _, compact := range []bool{false, true} {
				t.Run(note+"/"+format+"/"+fmt.Sprint(compact), func(t *testing.T) {
					data := renderTestExport(t, r, format, compact)
					body := strings.SplitN(string(data), snapshotStart, 2)[0]
					want := "<br>Lifecycle: " + tableText(note)
					if format == "html" {
						want = "<p>Lifecycle: " + html.EscapeString(note) + "</p>"
					}
					if !strings.Contains(body, want) || !strings.Contains(body, "Needs decision") {
						t.Fatalf("visible body omitted exact legacy note %q", want)
					}
				})
			}
		}
	}
}
