package radar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func capturedFixture(t *testing.T, name, manifest, body string) RepositorySnapshot {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, manifest), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	inv, files, err := CaptureRepository(root, DetectionRules{})
	if err != nil {
		t.Fatal(err)
	}
	return RepositorySnapshot{ID: "local:" + name, Name: name, Origin: "local", Status: "captured", Inventory: inv, Files: files}
}

func TestContextCanonicalOfflineAndUntrustedInputs(t *testing.T) {
	a := capturedFixture(t, "api", "pyproject.toml", "[project]\ndependencies = [\"fastapi\", \"langgraph\", \"unknown-package\"]\n")
	b := capturedFixture(t, "module", "go.mod", "module example.test/module\n\ngo 1.26.4\n")
	c, err := BuildContext([]RepositorySnapshot{a, b})
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := BuildContext([]RepositorySnapshot{b, a})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := reversed.Bytes()
	if !bytes.Equal(data, other) {
		t.Fatal("membership order changes context")
	}
	read, err := ReadContext(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := read.Bytes()
	if !bytes.Equal(data, again) {
		t.Fatal("offline compilation changed context")
	}
	if !bytes.Contains(data, []byte("pypi:unknown-package")) || !c.profiles["python_ai_api"] {
		t.Fatal("unknown or native checklist lost")
	}
	var pack contextPack
	if err := json.Unmarshal(data, &pack); err != nil {
		t.Fatal(err)
	}
	pack.Technologies[0].Significance = "system_defining"
	pack.Technologies[0].Name = "edited"
	tampered, _ := json.Marshal(pack)
	if _, err := ReadContext(tampered); err == nil {
		t.Fatal("accepted altered derived catalog")
	}
	if _, err := BuildContext([]RepositorySnapshot{a, a}); err == nil {
		t.Fatal("duplicate repository accepted")
	}
	a.Inventory.Findings[0].Decision = Decision{Status: "approved", Ring: "Adopt"}
	if _, err := BuildContext([]RepositorySnapshot{a}); err == nil {
		t.Fatal("capture can inject approval")
	}
}

func TestContextRejectsUnboundEvidenceAndHashes(t *testing.T) {
	for _, mutate := range []func(*RepositorySnapshot){
		func(s *RepositorySnapshot) { s.Files[0].Path = "../outside" },
		func(s *RepositorySnapshot) { s.Files[0].SHA256 = "bad" },
		func(s *RepositorySnapshot) { s.Inventory.Findings[0].Evidence[0].Path = "other.py" },
		func(s *RepositorySnapshot) { s.Inventory.Source.Fingerprint = "bad" },
	} {
		s := capturedFixture(t, "module", "go.mod", "module example.test/module\n\ngo 1.26.4\n")
		mutate(&s)
		if _, err := BuildContext([]RepositorySnapshot{s}); err == nil {
			t.Fatal("accepted unbound snapshot")
		}
	}
}

func TestContextOwnsItsSavedSnapshotData(t *testing.T) {
	snapshot := capturedFixture(t, "web", "mix.exs", "defp deps do\n[\n{:phoenix, \"1.8\"}\n]\nend\n")
	snapshot.Inventory.Warnings = []string{"original warning"}
	// Older snapshots can carry built-in aliases; the copy must own those too.
	for _, technology := range Catalog() {
		if technology.ID == "phoenix" {
			snapshot.Inventory.Technologies = []Technology{technology}
			break
		}
	}
	if len(snapshot.Inventory.Technologies) != 1 || len(snapshot.Inventory.Technologies[0].Hex) == 0 {
		t.Fatal("fixture has no package alias")
	}
	context, err := BuildContext([]RepositorySnapshot{snapshot})
	if err != nil {
		t.Fatal(err)
	}
	before, err := context.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files[0].SHA256 = "changed"
	snapshot.Inventory.Findings[0].Evidence[0].Detail = "changed"
	snapshot.Inventory.Warnings[0] = "changed"
	for i := range snapshot.Inventory.Technologies {
		if aliases := snapshot.Inventory.Technologies[i].Hex; len(aliases) > 0 {
			aliases[0] = "changed"
		}
	}
	after, err := context.Bytes()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("caller mutation changed a validated context")
	}
}

func TestPortfolioRuntimeScopesAndComponentOwnership(t *testing.T) {
	dev := capturedFixture(t, "dev", "pyproject.toml", "[project]\ndependencies = [\"fastapi\", \"langgraph\"]\n[project.optional-dependencies]\ndev = [\"pydantic\"]\n")
	optional := finding(t, dev.Inventory, "pydantic")
	if assessPortfolio(optional, dev.Inventory.Components).Significance == "system_defining" {
		t.Fatal("optional dependency promoted to primary runtime")
	}
	devOnly := capturedFixture(t, "devonly", "requirements-dev.txt", "fastapi\nlanggraph\n")
	if len(devOnly.Inventory.Components) > 0 {
		t.Fatal("dev-only packages establish runtime profile")
	}
	components := []Component{{Path: ".", Profile: "beam_web_data"}, {Path: "ui", Profile: "react_typescript_web"}}
	f := Finding{ID: "python", Evidence: []Evidence{{Path: "scripts/helper.py", Kind: "source reference"}, {Path: "ui/scripts/helper.py", Kind: "source reference"}}}
	a := assessPortfolio(f, components)
	if !contains(a.Profiles, "beam_web_data") || !contains(a.Profiles, "react_typescript_web") {
		t.Fatal("nearest component must be chosen independently for each occurrence")
	}
}

func TestContextKeepsExplicitRulesForPortfolioManifests(t *testing.T) {
	s := capturedFixture(t, "seed", "pyproject.toml", "[project]\ndependencies = [\"fastapi\", \"langgraph\"]\n")
	c, err := BuildContext([]RepositorySnapshot{s})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.example]\ntransport = \"custom_transport\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"example-transport","name":"Example transport","quadrant":"Platforms","pattern":"custom_transport","detail":"explicit transport reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ScanWithContext(root, rules, c)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range inv.Findings {
		found = found || f.ID == "example-transport"
	}
	if !found {
		t.Fatal("portfolio manifest reader suppressed explicit recognition")
	}
}

func TestContextLocalRoleChangesRequireApprovalReview(t *testing.T) {
	seed := capturedFixture(t, "seed", "requirements.txt", "fastapi\nlanggraph\n")
	c, err := BuildContext([]RepositorySnapshot{seed})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manifest := filepath.Join(root, "requirements.txt")
	if err := os.WriteFile(manifest, []byte("fastapi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inv, err := ScanWithContext(root, DetectionRules{}, c)
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{Rules: map[string]Decision{"python": {Status: "approved", Ring: "Adopt", Scope: "local API", Rationale: "team decision", Owner: "team", Reviewed: "2026-10-31", Reference: "review-1"}}}
	prior, err := Build(inv, policy, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("fastapi\nlanggraph\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inv, err = ScanWithContext(root, DetectionRules{}, c)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Build(inv, Policy{}, &prior, "")
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, Inventory{Findings: report.Findings}, "python")
	if !f.EvidenceChanged || !f.ReviewRequired || f.Decision != policy.Rules["python"] {
		t.Fatal("same-pack local role change silently reused approval")
	}
	again, err := Build(inv, Policy{}, &report, "")
	if err != nil {
		t.Fatal(err)
	}
	if !finding(t, Inventory{Findings: again.Findings}, "python").ReviewRequired {
		t.Fatal("replay cleared pending review")
	}
}

func TestPortfolioComposableProfilesPreserveFrameworkSignificance(t *testing.T) {
	s := capturedFixture(t, "events", "mix.exs", "defp deps do\n[\n{:phoenix, \"1.8\"},\n{:commanded, \"1\"}\n]\nend\n")
	f := finding(t, s.Inventory, "phoenix")
	if assessPortfolio(f, s.Inventory.Components).Significance != "system_defining" {
		t.Fatal("composable profile hid the web framework")
	}
}

func TestPortfolioUsesNearestComponentForShortPaths(t *testing.T) {
	for _, child := range []string{"a", "ab", "apps/ui"} {
		t.Run(child, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, "package.json", `{"dependencies":{"react":"19"}}`)
			fixture(t, root, child+"/go.mod", "module example.org/worker\n\ngo 1.26\n")
			fixture(t, root, child+"/helper.py", "print(1)\n")
			inv, err := Scan(root)
			if err != nil {
				t.Fatal(err)
			}
			f := finding(t, inv, "python")
			got := assessPortfolio(f, inv.Components)
			if len(got.Profiles) != 1 || got.Profiles[0] != "go_service" {
				t.Fatalf("child inherited another component's profiles: %v", got.Profiles)
			}
		})
	}
}

func TestPortfolio2ContextKeepsOriginalAssessment(t *testing.T) {
	data, err := os.ReadFile("testdata/portfolio-2-context.json")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ReadContext(data)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := saved.Bytes()
	if err != nil || !bytes.Equal(data, rebuilt) {
		t.Fatal("saved portfolio-2 context changed")
	}
	current, err := BuildContext(saved.Repositories())
	if err != nil {
		t.Fatal(err)
	}
	for _, technology := range current.pack.Technologies {
		if technology.ID == "python" {
			if len(technology.Profiles) != 1 || technology.Profiles[0] != "go_service" {
				t.Fatalf("current rules retained root profile: %v", technology.Profiles)
			}
			return
		}
	}
	t.Fatal("current context lost Python")
}

func TestCorrectedComponentProfileRequestsReview(t *testing.T) {
	data, err := os.ReadFile("testdata/portfolio-2-context.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ReadContext(data)
	if err != nil {
		t.Fatal(err)
	}
	inv := pack.Repositories()[0].Inventory
	policy := Policy{Rules: map[string]Decision{"go": {Status: "approved", Ring: "Adopt", Scope: "worker", Rationale: "team choice", Owner: "team", Reference: "review-1", Reviewed: "2026-11-01"}}}
	prior, err := Build(inv, policy, nil, "example")
	if err != nil {
		t.Fatal(err)
	}
	prior.RuleVersion = "context-3"
	for i := range prior.Findings {
		a := assessPortfolioVersion(prior.Findings[i], inv.Components, "portfolio-2")
		prior.Findings[i].Assessment = &a
	}
	updated, err := Build(inv, Policy{}, &prior, "")
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, Inventory{Findings: updated.Findings}, "go")
	if !f.EvidenceChanged || !f.ReviewRequired || f.Decision != policy.Rules["go"] {
		t.Fatal("corrected profile silently reused approval or lost the team's decision")
	}
}

func TestCaptureKeepsCustomDefinitionsWithoutBuiltInCatalog(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "client.go", "package example\nfunc call() { ExampleClient() }\n")
	rules, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"example-client","name":"Example client","quadrant":"Platforms","pattern":"ExampleClient","detail":"Example client reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv, files, err := CaptureRepository(root, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Technologies) != 1 || inv.Technologies[0].ID != "example-client" {
		t.Fatalf("capture lost custom metadata or repeated built-in definitions: %+v", inv.Technologies)
	}
	pack, err := BuildContext([]RepositorySnapshot{{ID: "local:example", Name: "example", Origin: "local", Status: "captured", Inventory: inv, Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := pack.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ReadContext(data)
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, saved.Repositories()[0].Inventory, "example-client")
	if f.Name != "Example client" || f.Quadrant != "Platforms" || len(f.Evidence) == 0 {
		t.Fatal("context lost custom classification or evidence")
	}
}
