package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tech-radar/internal/radar"
)

const commandPolicy = `{"kind":"tech-radar-policy","schema_version":1,"id":"example-standard","version":"1","default":"deny","approval":{"status":"approved","owner":"Engineering","reference":"DEC-1","reviewed":"2026-11-01"},"rules":[{"id":"react","technologies":["react"],"effect":"allow"}]}`

func TestCheckCommandCIAndProtectedReplay(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19.0.0"}}`)
	policy := write(t, repo, "check-policy.json", commandPolicy)
	raw := call(t, 0, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01")
	r, err := radar.ReadCheckResult([]byte(raw))
	if err != nil || len(r.Repositories) != 1 {
		t.Fatalf("invalid check output: %v", err)
	}
	if r.Repositories[0].Snapshot.Inventory.Source.Files != 1 {
		t.Fatal("check policy entered scan evidence")
	}
	out := filepath.Join(work, "check.json")
	call(t, 0, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01", "--out", out)
	call(t, 1, "check", "--repo", repo, "--policy", policy, "--out", out)
	call(t, 0, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01", "--baseline", out, "--out", out, "--force")
	write(t, repo, "package.json", `{"dependencies":{"react":"19.0.0","unapproved":"1"}}`)
	raw = call(t, 2, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01", "--baseline", out)
	if !strings.Contains(raw, `"kind": "added"`) || !strings.Contains(raw, `"technology": "npm:unapproved"`) {
		t.Fatal("new violation lacks drift/evidence")
	}
	call(t, 1, "check", "--repo", repo, "--policy", policy, "--out", policy, "--force")
}

func TestCheckCommandUnknownVersionAndOfflineOrgInput(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"^19"}}`)
	constrained := strings.Replace(commandPolicy, `"effect":"allow"`, `"effect":"allow","versions":">=19.0.0 <20.0.0"`, 1)
	policy := write(t, work, "policy.json", constrained)
	call(t, 3, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01")
	context := filepath.Join(work, "context.json")
	call(t, 0, "discover", "--repo", repo, "--out", context)
	raw := call(t, 3, "check", "--from", context, "--policy", policy, "--as-of", "2026-11-01")
	var r struct{ Repositories []json.RawMessage }
	if err := json.Unmarshal([]byte(raw), &r); err != nil || len(r.Repositories) != 1 {
		t.Fatal("offline checking failed")
	}
	call(t, 1, "check", "--repo", repo, "--from", context, "--policy", policy)
}

func TestGeneratedCheckInsideRepositoryDoesNotScanItself(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19.0.0"}}`)
	policy := write(t, work, "policy.json", commandPolicy)
	out := filepath.Join(repo, ".github", "technology-check.json")
	call(t, 0, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01", "--out", out)
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	call(t, 0, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01", "--out", out, "--force")
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("generated check entered its own source fingerprint")
	}
}

func TestCheckStaleLockCannotTurnAForbiddenDeclarationIntoPermission(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"^20.0.0"}}`)
	write(t, repo, "package-lock.json", `{"lockfileVersion":3,"packages":{"node_modules/react":{"version":"19.1.0"}}}`)
	policy := write(t, work, "policy.json", strings.Replace(commandPolicy, `"effect":"allow"`, `"effect":"allow","versions":">=19.0.0 <20.0.0"`, 1))
	call(t, 3, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01")
}
