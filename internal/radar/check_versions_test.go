package radar

import "testing"

func TestCheckResolvedVersionsStayInsideTheirComponent(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow","versions":">=19.0.0 <20.0.0"}]`, `{"dependencies":{"react":"^19"}}`)
	f := snapshot.Inventory.Findings[0]
	f.Evidence = append(f.Evidence, Evidence{Path: "other/package-lock.json", Line: 1, Kind: "lock resolution", Detail: "npm package react: 19.1.0"})
	snapshot.Inventory.Findings = []Finding{f}
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 3 {
		t.Fatal("unrelated lockfile authorized unresolved declaration")
	}
	f.Evidence[1].Path = "package-lock.json"
	snapshot.Inventory.Findings = []Finding{f}
	result, err = Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil || result.ExitCode() != 0 {
		t.Fatalf("same-component lock not used: %v %+v", err, result)
	}
}

func TestCheckRuntimePinConstraintsAndVersionDrift(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, ".tool-versions", "elixir 1.18.4\n")
	inv, files, err := CaptureRepository(root, DetectionRules{})
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := ReadCheckPolicy([]byte(`{"kind":"tech-radar-policy","schema_version":1,"id":"runtime","version":"1","default":"deny","approval":{"status":"approved","owner":"Engineering","reference":"DEC-1","reviewed":"2026-11-01"},"rules":[{"id":"elixir","technologies":["elixir"],"effect":"allow","versions":">=1.18.0 <1.19.0"}]}`))
	snapshot := RepositorySnapshot{ID: "local:example", Name: "example", Origin: "local", Status: "captured", Inventory: inv, Files: files}
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil || result.ExitCode() != 0 {
		t.Fatalf("runtime pin not checked: %v %+v", err, result)
	}
	prior := snapshot
	prior.Inventory.Findings = append([]Finding(nil), inv.Findings...)
	prior.Inventory.Findings[0].Evidence = append([]Evidence(nil), inv.Findings[0].Evidence...)
	prior.Inventory.Findings[0].Evidence[0].Detail = "elixir pin 1.17.0"
	result, err = Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{prior}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repositories[0].Drift) != 1 || result.Repositories[0].Drift[0].Kind != "version_changed" {
		t.Fatal("runtime version drift omitted")
	}
}

func TestCheckLockResolutionMustSatisfyTheDeclaration(t *testing.T) {
	for _, tt := range []struct {
		ecosystem, name, declaration, pin string
		exit                              int
	}{
		{"npm", "react", "^20.0.0", "19.1.0", 3},
		{"npm", "react", "[non-version source reference omitted]", "19.1.0", 3},
		{"npm", "react", "^19.0.0", "19.1.0", 0},
		{"hex", "phoenix", "~> 1.8", "1.9.0", 0},
		{"hex", "phoenix", "~> 1.8.0", "1.9.0", 3},
	} {
		policy, snapshot := checkFixture(t, `[{"id":"framework","technologies":["react","phoenix"],"effect":"allow","versions":">=1.0.0 <21.0.0"}]`, `{"dependencies":{"react":"19.1.0"}}`)
		f := snapshot.Inventory.Findings[0]
		f.ID = tt.name
		f.Evidence = []Evidence{{Path: "package.json", Line: 1, Kind: "declaration", Detail: tt.ecosystem + " package " + tt.name + ": " + tt.declaration + " [runtime]"}, {Path: "package-lock.json", Line: 1, Kind: "lock resolution", Detail: tt.ecosystem + " package " + tt.name + ": " + tt.pin}, {Path: "src/app", Line: 1, Kind: "reference", Detail: "Framework use"}}
		snapshot.Inventory.Findings = []Finding{f}
		result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode() != tt.exit {
			t.Fatalf("%s %s resolved as %s: got %d want %d", tt.ecosystem, tt.declaration, tt.pin, result.ExitCode(), tt.exit)
		}
	}
}
