package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Successful fixture setup uses the canonical APIs with all options visible.
// Tests that expect validation or filesystem errors call those APIs directly.
func scanTestRepository(t *testing.T, root string, options ScanOptions) Inventory {
	t.Helper()
	inv, err := ScanWithOptions(root, options)
	if err != nil {
		t.Fatalf("scan fixture %q: %v", root, err)
	}
	return inv
}

func buildTestReport(t *testing.T, inv Inventory, policy Policy, previous *Report, name string, options BuildOptions) Report {
	t.Helper()
	r, err := BuildWithOptions(inv, policy, previous, name, options)
	if err != nil {
		t.Fatalf("build report %q (model=%q, view=%q): %v", name, options.Model, options.View, err)
	}
	return r
}

// renderTestExport exercises the full or portable document pipeline without
// rendering an unused full document before a compact one.
func renderTestExport(t *testing.T, r Report, format string, compact bool) []byte {
	t.Helper()
	var data []byte
	var err error
	if compact {
		data, err = RenderDocument(r, format, "evidence.json", strings.Repeat("a", 64))
	} else {
		data, err = Render(r, format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readTestReport(t *testing.T, data []byte) Report {
	t.Helper()
	r, err := ReadReport(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReportWordingKeepsSavedStateAndTeamText(t *testing.T) {
	s := capturedFixture(t, "web", "mix.exs", "defp deps do\n[\n{:phoenix, \"1.8\"}\n]\nend\n")
	teamText := "Investigate this declared/referenced architectural choice and confirm its scope, ownership and current use before recommending commitment."
	r := buildTestReport(t, s.Inventory, Policy{Rules: map[string]Decision{"phoenix": {Ring: "Assess", Scope: "web app", Rationale: teamText}}}, nil, "example", BuildOptions{})
	module := capturedFixture(t, "module", "go.mod", "module example.org/module\n\ngo 1.26\n")
	goReport := buildTestReport(t, module.Inventory, Policy{Rules: map[string]Decision{"go": {Ring: "Assess", Scope: "module", Rationale: teamText}}}, nil, "example", BuildOptions{})
	for _, r := range []Report{r, goReport} {
		before, err := Render(r, "json")
		if err != nil {
			t.Fatal(err)
		}
		for _, format := range []string{"markdown", "html"} {
			for _, concise := range []bool{false, true} {
				data := renderTestExport(t, r, format, concise)
				body := html.UnescapeString(strings.SplitN(string(data), snapshotStart, 2)[0])
				if strings.Contains(body, "native execution/domain/data boundary") || strings.Contains(body, "service role unverified") || strings.Contains(body, "beam_web_data") || strings.Contains(body, "go_service") {
					t.Fatal("internal profile metadata leaked into the displayed report")
				}
				if !strings.Contains(body, teamText) {
					t.Fatal("rendering rewrote the team's rationale")
				}
			}
		}
		after, err := Render(r, "json")
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("display wording changed saved report state")
		}
	}
}

func TestTrialRingFitsArchitecturalFindingsWithoutOverlappingMarkers(t *testing.T) {
	policy := Policy{SchemaVersion: 1, Rules: map[string]Decision{}}
	for i := range 15 {
		id := fmt.Sprintf("platform-%02d", i)
		policy.Rules[id] = Decision{Name: id, Quadrant: "Platforms", Ring: "Trial", Scope: "Demo", Rationale: "Review candidate"}
	}
	r := buildTestReport(t, Inventory{Source: Source{Fingerprint: strings.Repeat("a", 64)}}, policy, nil, "Dense radar", BuildOptions{})
	b, err := Render(r, "html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), `<a href="#entry-`) != 15 {
		t.Fatal("architectural findings overflow a ring with space for two marker rows")
	}
	matches := regexp.MustCompile(`cx="([0-9.]+)" cy="([0-9.]+)" r="15"`).FindAllStringSubmatch(string(b), -1)
	if len(matches) != 15 {
		t.Fatalf("expected 15 marker coordinates, got %d", len(matches))
	}
	for i, a := range matches {
		x, _ := strconv.ParseFloat(a[1], 64)
		y, _ := strconv.ParseFloat(a[2], 64)
		for _, c := range matches[:i] {
			cx, _ := strconv.ParseFloat(c[1], 64)
			cy, _ := strconv.ParseFloat(c[2], 64)
			if math.Hypot(x-cx, y-cy) < 32 {
				t.Fatal("radar markers overlap")
			}
		}
	}
}

func TestEstablishedFrameworkPlotsInsideAdopt(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	f := finding(t, Inventory{Findings: r.Findings}, "phoenix")
	b, err := RenderDocument(r, "html", "evidence.json", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	pattern := fmt.Sprintf(`<a href="#entry-%d"[^>]*><circle cx="([0-9.]+)" cy="([0-9.]+)"`, f.Number)
	m := regexp.MustCompile(pattern).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("established framework is missing from the radar")
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	y, _ := strconv.ParseFloat(m[2], 64)
	if x <= 500 || y <= 500 || math.Hypot(x-500, y-500)+15 >= 215 {
		t.Fatalf("framework marker (%f, %f) is outside Adopt in Languages & Frameworks", x, y)
	}
}

func TestReportRejectsForgedProposalAndEscapingEvidenceReference(t *testing.T) {
	inv := scanTestRepository(t, beamFixture(t), ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	b, _ := Render(r, "json")
	if _, err := RenderDocument(r, "html", "", ""); err == nil {
		t.Fatal("concise document accepted a missing evidence reference")
	}
	for _, mutation := range []func(map[string]any){
		func(m map[string]any) {
			m["evidence_file"] = "../private.json"
			m["evidence_sha256"] = strings.Repeat("a", 64)
		},
		func(m map[string]any) { m["evidence_file"] = "evidence.json"; m["evidence_sha256"] = "bad" },
		func(m map[string]any) {
			m["highlights"].([]any)[0].(map[string]any)["proposal"] = map[string]any{"ring": "Adopt", "scope": "Demo", "rationale": "Forged approval", "status": "approved", "owner": "Team"}
		},
	} {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		mutation(m)
		bad, _ := json.Marshal(m)
		if _, err := ReadReport(bad); err == nil {
			t.Fatal("accepted forged proposal or unsafe evidence reference")
		}
	}
}

func TestExportRoundTripsDecisionsAndEvidence(t *testing.T) {
	p := Policy{SchemaVersion: 1, Repository: "ExampleApp", Rules: map[string]Decision{"python": {Exclude: true, Rationale: "Team excludes scripting languages"}, "phoenix": {Ring: "Adopt", Scope: "ExampleApp web", Rationale: "Team proposal"}}}
	r := buildTestReport(t, inventoryForTests(), p, nil, "", BuildOptions{})
	for _, format := range []string{"markdown", "html", "json"} {
		t.Run(format, func(t *testing.T) {
			output := renderTestExport(t, r, format, false)
			recovered := readTestReport(t, output)
			if !reflect.DeepEqual(r, recovered) {
				t.Fatalf("round trip lost report state:\n%#v\n%#v", r, recovered)
			}
			again := renderTestExport(t, r, format, false)
			if !bytes.Equal(output, again) {
				t.Fatal("output is not deterministic")
			}
			next := buildTestReport(t, inventoryForTests(), Policy{}, &recovered, "", BuildOptions{})
			if len(next.Findings) != 1 || next.Findings[0].Decision.Ring != "Adopt" {
				t.Fatal("reimported policy was not preserved")
			}
		})
	}
}

func TestRejectsEditedOrIncompleteGeneratedDocument(t *testing.T) {
	r := buildTestReport(t, inventoryForTests(), Policy{}, nil, "ExampleApp", BuildOptions{})
	for _, format := range []string{"markdown", "html"} {
		b, err := Render(r, format)
		if err != nil {
			t.Fatal(err)
		}
		edited := bytes.Replace(b, []byte("ExampleApp"), []byte("Edited repository"), 1)
		if _, err := ReadReport(edited); err == nil {
			t.Fatal("accepted manually edited body; next export could discard it")
		}
		if _, err := ReadReport(b[:len(b)/2]); err == nil {
			t.Fatal("accepted truncated export")
		}
	}
}

func TestExportEscapesPolicyMarkup(t *testing.T) {
	r := buildTestReport(t, inventoryForTests(), Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": {Name: "Framework | <script>alert(1)</script>", Ring: "Assess", Scope: "Test", Rationale: "Review </script> markup"}}}, nil, "ExampleApp", BuildOptions{})
	b, err := Render(r, "html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<script>alert") || strings.Contains(string(b), "Review </script>") {
		t.Fatal("policy markup escaped into the document")
	}
	if _, err := ReadReport(b); err != nil {
		t.Fatalf("escaping broke reimport: %v", err)
	}
}
