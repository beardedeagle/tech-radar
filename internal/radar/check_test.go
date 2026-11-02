package radar

import (
	"strings"
	"testing"
)

const approvedCheckPolicy = `{"kind":"tech-radar-policy","schema_version":1,"id":"example-standard","version":"1","default":"deny","approval":{"status":"approved","owner":"Engineering","reference":"DEC-1","reviewed":"2026-11-01"},"rules":%s}`

func checkFixture(t *testing.T, rules, manifest string) (CheckPolicy, RepositorySnapshot) {
	t.Helper()
	policy, err := ReadCheckPolicy([]byte(strings.Replace(approvedCheckPolicy, "%s", rules, 1)))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	fixture(t, root, "package.json", manifest)
	inv, files, err := CaptureRepository(root, DetectionRules{})
	if err != nil {
		t.Fatal(err)
	}
	return policy, RepositorySnapshot{ID: "local:example", Name: "example", Origin: "local", Status: "captured", Inventory: inv, Files: files}
}

func TestCheckPermissionScopeAndDenial(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react-web","technologies":["react"],"effect":"allow","paths":["web"]},{"id":"deny-react","technologies":["react"],"effect":"deny","paths":["web/legacy"]}]`, `{"dependencies":{"react":"19.0.0"}}`)
	f := snapshot.Inventory.Findings[0]
	f.Evidence = []Evidence{{Path: "web/package.json", Line: 1, Kind: "declaration", Detail: "npm package react: 19.0.0 [dependencies]"}, {Path: "web/legacy/package.json", Line: 1, Kind: "declaration", Detail: "npm package react: 19.0.0 [dependencies]"}, {Path: "web-other/package.json", Line: 1, Kind: "declaration", Detail: "npm package react: 19.0.0 [dependencies]"}}
	snapshot.Inventory.Findings = []Finding{f}
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 || len(result.Repositories[0].Violations) != 2 {
		t.Fatalf("scope or deny bypassed: %+v", result.Repositories)
	}
}

func TestCheckVersionConstraintsDoNotGuessDependencyRanges(t *testing.T) {
	for _, tt := range []struct {
		version string
		code    int
	}{{"19.1.0", 0}, {"18.3.0", 2}, {"^19", 3}, {"19.0.0-beta.1", 2}} {
		policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow","versions":">=19.0.0 <20.0.0"}]`, `{"dependencies":{"react":"`+tt.version+`"}}`)
		result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode() != tt.code {
			t.Fatalf("version %s: got %d want %d", tt.version, result.ExitCode(), tt.code)
		}
	}
}

func TestCheckExistingOnlyNeedsComparableBaseline(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"existing-only"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 3 {
		t.Fatal("missing baseline became permission")
	}
	result, err = Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{snapshot}})
	if err != nil || result.ExitCode() != 0 {
		t.Fatalf("existing use rejected: %v %+v", err, result)
	}
	prior := snapshot
	prior.Inventory = snapshot.Inventory
	prior.Inventory.Findings = nil
	result, err = Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{prior}})
	if err != nil || result.ExitCode() != 2 {
		t.Fatal("new adoption allowed")
	}
}

func TestCheckGapsAndDriftStaySeparateFromViolations(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	prior := snapshot
	prior.Inventory = snapshot.Inventory
	prior.Inventory.Findings = nil
	snapshot.Inventory.Warnings = []string{"Skipped large eligible file"}
	r, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{prior}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode() != 3 || len(r.Repositories[0].Violations) != 0 || len(r.Repositories[0].Drift) != 1 || r.Repositories[0].Drift[0].Kind != "added" {
		t.Fatalf("drift/gaps conflated: %+v", r)
	}
}

func TestCheckPolicyRejectsUnapprovedOrAmbiguousPermission(t *testing.T) {
	valid := strings.Replace(approvedCheckPolicy, "%s", `[{"id":"react","technologies":["react"],"effect":"allow"}]`, 1)
	for _, input := range []string{strings.Replace(valid, `"approved"`, `"proposed"`, 1), strings.Replace(valid, `"default":"deny"`, `"default":"typo"`, 1), strings.Replace(valid, `"effect":"allow"`, `"effect":"allow","ring":"Adopt"`, 1), strings.Replace(valid, `"technologies":["react"]`, `"technologies":["react"],"paths":["../web"]`, 1)} {
		if _, err := ReadCheckPolicy([]byte(input)); err == nil {
			t.Fatal("accepted invalid enforcement policy")
		}
	}
}

func TestCheckIncompleteCaptureDoesNotClaimTechnologyRemoval(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	old := snapshot
	snapshot.Inventory.Findings = nil
	snapshot.Inventory.Warnings = []string{"Skipped package.json: eligible file exceeds 2 MiB."}
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{old}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repositories[0].Drift) != 0 || result.ExitCode() != 3 {
		t.Fatal("incomplete capture claimed removal")
	}
}

func TestCheckRawPackagePermissionDoesNotApproveItsOtherAliases(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react-core","technologies":["npm:react"],"effect":"allow"}]`, `{"dependencies":{"react":"19.0.0","react-dom":"19.0.0"}}`)
	result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 || len(result.Repositories[0].Violations) != 1 || !strings.Contains(result.Repositories[0].Violations[0].Evidence[0].Detail, "react-dom") {
		t.Fatal("package permission leaked to its canonical sibling")
	}
}

func TestCheckLocationDriftDoesNotClaimVersionChanged(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	old := snapshot
	old.Inventory.Findings = append([]Finding(nil), snapshot.Inventory.Findings...)
	old.Inventory.Findings[0].Evidence = append([]Evidence(nil), snapshot.Inventory.Findings[0].Evidence...)
	old.Inventory.Findings[0].Evidence[0].Path = "old/package.json"
	old.Inventory.Findings[0].EvidenceFingerprint = "different"
	r, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{old}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Repositories[0].Drift) != 1 || r.Repositories[0].Drift[0].Kind != "evidence_changed" {
		t.Fatal("moving an unchanged pin was called a version change")
	}
}

func TestCheckZeroFileBaselineRemainsIncomplete(t *testing.T) {
	for _, effect := range []string{"allow", "existing-only"} {
		policy, current := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"`+effect+`"}]`, `{"dependencies":{"react":"19.0.0"}}`)
		root := t.TempDir()
		inv, files, err := CaptureRepository(root, DetectionRules{})
		if err != nil {
			t.Fatal(err)
		}
		prior := current
		prior.Inventory = inv
		prior.Files = files
		r, err := Check([]RepositorySnapshot{current}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{prior}})
		if err != nil {
			t.Fatal(err)
		}
		if r.ExitCode() != 3 || r.Complete || len(r.Repositories[0].Drift) != 0 {
			t.Fatalf("%s treated unknown history as complete: %+v", effect, r)
		}
		prior.Status = "empty"
		r, err = Check([]RepositorySnapshot{current}, policy, CheckOptions{AsOf: "2026-11-01", Baseline: []RepositorySnapshot{prior}})
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if effect == "existing-only" {
			want = 2
		}
		if r.ExitCode() != want || !r.Complete {
			t.Fatal("known empty repository lost its distinct semantics")
		}
	}
}
