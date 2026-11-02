package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tech-radar/internal/radar"
)

func TestConciseExportKeepsFullEvidenceAndPortableDecisions(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "mix.exs", "defp deps do\n[\n{:phoenix, \"~> 1.8\"}\n]\nend\n")
	for i := range 45 {
		write(t, repo, filepath.Join("lib", fmt.Sprintf("endpoint%02d.ex", i)), "use Phoenix.Endpoint, otp_app: :demo\n")
	}
	dest := filepath.Join(work, "TECHNOLOGY_RADAR.html")
	call(t, 0, "generate", "--repo", repo, "--name", "Demo", "--format", "html", "--out", dest)
	doc, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `href="#entry-`) {
		t.Fatal("radar has no plotted blips")
	}
	if strings.Contains(string(doc), "endpoint44.ex") {
		t.Fatal("full evidence still embedded in human report")
	}
	companions, err := filepath.Glob(filepath.Join(work, "TECHNOLOGY_RADAR.evidence.*.json"))
	if err != nil || len(companions) != 1 {
		t.Fatalf("missing separate evidence: %v, %v", companions, err)
	}
	evidence, err := os.ReadFile(companions[0])
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ Findings []radar.Finding }
	if err := json.Unmarshal(evidence, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Findings) != 2 || state.Findings[1].ID != "phoenix" || len(state.Findings[1].Evidence) != 46 {
		t.Fatal("companion lost full occurrences")
	}
	call(t, 0, "generate", "--repo", repo, "--previous", dest, "--format", "html", "--out", dest, "--force")
	again, _ := os.ReadFile(dest)
	if !bytes.Equal(doc, again) {
		t.Fatal("concise replay changed unchanged output")
	}
	// Decisions and review flags survive without the evidence file.
	if err := os.Remove(companions[0]); err != nil {
		t.Fatal(err)
	}
	call(t, 0, "generate", "--repo", repo, "--previous", dest, "--format", "html", "--out", dest, "--force")
	again, _ = os.ReadFile(dest)
	if !bytes.Equal(doc, again) {
		t.Fatal("missing companion broke portable replay")
	}
}

func TestEvidenceConflictDoesNotPublishOrOverwriteReport(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
	dest := filepath.Join(work, "radar.md")
	call(t, 0, "generate", "--repo", repo, "--out", dest)
	companions, _ := filepath.Glob(filepath.Join(work, "radar.evidence.*.json"))
	if len(companions) != 1 {
		t.Fatal("missing companion")
	}
	before, _ := os.ReadFile(dest)
	write(t, work, filepath.Base(companions[0]), "human evidence notes")
	call(t, 1, "generate", "--repo", repo, "--out", dest, "--force")
	after, _ := os.ReadFile(dest)
	if !bytes.Equal(before, after) {
		t.Fatal("published report despite evidence conflict")
	}
	kept, _ := os.ReadFile(companions[0])
	if string(kept) != "human evidence notes" {
		t.Fatal("overwrote non-generated companion")
	}
}

func TestCompactReplayPreservesApprovalAndRemovedEvidence(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	manifest := write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
	policy := write(t, work, "policy.json", `{"schema_version":1,"rules":{"react":{"ring":"Caution","scope":"Existing UI","rationale":"Team review","status":"approved","owner":"Team","reference":"PR #7","reviewed":"2026-10-30"}}}`)
	dest := filepath.Join(work, "radar.md")
	call(t, 0, "generate", "--repo", repo, "--policy", policy, "--out", dest)
	doc, _ := os.ReadFile(dest)
	compact, err := radar.ReadReport(doc)
	if err != nil {
		t.Fatal(err)
	}
	if compact.Findings[0].Decision.Status != "approved" || len(compact.Findings[0].Evidence) != 0 {
		t.Fatal("compact decision state is wrong")
	}
	inv, err := radar.Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := radar.Build(inv, radar.Policy{}, &compact, "")
	if err != nil || unchanged.Findings[0].ReviewRequired {
		t.Fatalf("compact replay falsely invalidated approval: %v", err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	// The evidence file restores locations for findings no longer in the repo.
	text := call(t, 0, "scan", "--repo", repo, "--previous", dest)
	r, err := radar.ReadReport([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if r.Findings[0].Presence != "not detected" || len(r.Findings[0].Evidence) != 1 || r.Decisions["react"].Ring != "Caution" {
		t.Fatal("removed evidence or decision was discarded")
	}
	companions, _ := filepath.Glob(filepath.Join(work, "radar.evidence.*.json"))
	write(t, work, filepath.Base(companions[0]), "{}")
	call(t, 1, "scan", "--repo", repo, "--previous", dest)
}

func TestLegacySelfContainedExportStillAvailable(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
	dest := filepath.Join(work, "legacy.html")
	call(t, 0, "generate", "--repo", repo, "--format", "html", "--self-contained", "--out", dest)
	b, _ := os.ReadFile(dest)
	r, err := radar.ReadReport(b)
	if err != nil || len(r.Findings[0].Evidence) != 1 || r.EvidenceFile != "" {
		t.Fatalf("legacy self-contained capability lost: %v", err)
	}
}

func write(t *testing.T, root, name, content string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
func call(t *testing.T, want int, args ...string) string {
	t.Helper()
	var out, err bytes.Buffer
	code := run(args, &out, &err)
	if code != want {
		t.Fatalf("exit %d, want %d: %s", code, want, err.String())
	}
	return out.String()
}

func TestRunSpecificDetectionRequiresExplicitRulesInput(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "lib/connectors.ex", "Example.Connectors.CRM.call()\n")
	rules := write(t, work, "rules.json", `{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"\\bExample\\.Connectors\\.CRM\\b","detail":"HubSpot integration configuration/reference"},{"id":"example-scheduler","name":"Example scheduler","quadrant":"Platforms","pattern":"\\bExample\\.Schedule\\b","detail":"Scheduling integration reference"}]}`)
	write(t, repo, "lib/scheduler.ex", "Example.Schedule.call()\n")
	without := call(t, 0, "scan", "--repo", repo)
	if strings.Contains(without, `"id": "hubspot"`) || strings.Contains(without, `"id": "example-scheduler"`) {
		t.Fatal("run-specific detector was active without input")
	}
	with := call(t, 0, "scan", "--repo", repo, "--rules", rules)
	r, err := radar.ReadReport([]byte(with))
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, f := range r.Findings {
		if f.ID == "hubspot" || f.ID == "example-scheduler" {
			found++
			if len(f.Evidence) != 1 || f.Decision.Status == "approved" {
				t.Fatal("input detection lost evidence or invented approval")
			}
		}
	}
	if found != 2 {
		t.Fatal("runtime mappings did not detect both known and custom integrations")
	}
	var state struct {
		Source struct {
			RulesFingerprint string `json:"rules_fingerprint"`
		}
	}
	if err := json.Unmarshal([]byte(with), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Source.RulesFingerprint) != 64 {
		t.Fatal("missing deterministic rules provenance")
	}
	previous := write(t, work, "previous.json", with)
	call(t, 1, "scan", "--repo", repo, "--previous", previous)
	again := call(t, 0, "scan", "--repo", repo, "--rules", rules, "--previous", previous)
	if again != with {
		t.Fatal("unchanged rules and evidence did not replay deterministically")
	}
	document := filepath.Join(work, "radar.html")
	call(t, 0, "generate", "--repo", repo, "--rules", rules, "--format", "html", "--out", document)
	fromDocument := call(t, 0, "scan", "--repo", repo, "--rules", rules, "--previous", document)
	if fromDocument != with {
		t.Fatal("concise document/companion replay lost runtime detection state")
	}
}

func TestRuntimeRuleFileDoesNotBecomeEvidenceOrGetOverwritten(t *testing.T) {
	repo := t.TempDir()
	write(t, repo, "lib/app.ex", "Example.CRM.call()\n")
	rules := write(t, repo, "infrastructure/rules.json", `{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"Example.CRM","detail":"Example.CRM reference"}]}`)
	before, _ := os.ReadFile(rules)
	text := call(t, 0, "scan", "--repo", repo, "--rules", rules)
	r, err := radar.ReadReport([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.Files != 1 {
		t.Fatal("rules input entered its own source corpus")
	}
	for _, f := range r.Findings {
		if f.ID == "hubspot" && (len(f.Evidence) != 1 || f.Evidence[0].Path != "lib/app.ex") {
			t.Fatal("rule text was mistaken for application evidence")
		}
	}
	call(t, 1, "generate", "--repo", repo, "--rules", rules, "--out", rules, "--format", "json", "--force")
	after, _ := os.ReadFile(rules)
	if !bytes.Equal(before, after) {
		t.Fatal("overwrote run rules")
	}
}

func TestInvalidDetectionRulesFailBeforeRepositoryScan(t *testing.T) {
	work := t.TempDir()
	for i, body := range []string{
		`{"schema_version":1}`,
		`{"schema_version":2,"patterns":[]}`,
		`{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"[","detail":"Invalid expression"}]}`,
		`{"schema_version":1,"patterns":[{"id":"hubspot","pattern":".*","detail":"Matches empty lines"}]}`,
		`{"schema_version":1,"patterns":[{"id":"unknown-integration","pattern":"Example","detail":"Missing classification"}]}`,
		`{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"Example","detail":"Reference","ring":"Adopt"}]}`,
		`{"schema_version":1,"schema_version":1,"patterns":[]}`,
	} {
		rules := write(t, work, fmt.Sprintf("bad%d.json", i), body)
		var out, stderr bytes.Buffer
		if run([]string{"scan", "--repo", filepath.Join(work, "missing-repo"), "--rules", rules}, &out, &stderr) != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "rules:") {
			t.Fatalf("did not reject invalid rules first: %s", stderr.String())
		}
	}
}

// Existing source files and reports must survive failed output writes.
func TestGenerateDeterministicAndSafeWrites(t *testing.T) {
	repo, out := t.TempDir(), t.TempDir()
	source := write(t, repo, "package.json", `{"dependencies":{"react":"^19"}}`)
	destination := filepath.Join(out, "TECHNOLOGY_RADAR.md")
	call(t, 0, "generate", "--repo", repo, "--name", "Demo", "--out", destination)
	first, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	call(t, 1, "generate", "--repo", repo, "--name", "Demo", "--out", destination)
	call(t, 0, "generate", "--repo", repo, "--name", "Demo", "--previous", destination, "--out", destination, "--force")
	second, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("unchanged repository and prior decisions produced a different document")
	}
	call(t, 1, "generate", "--repo", repo, "--format", "json", "--out", source, "--force")
	b, _ := os.ReadFile(source)
	if string(b) != `{"dependencies":{"react":"^19"}}` {
		t.Fatal("overwrote repository source")
	}
	human := write(t, out, "human.md", "Human-authored decisions\n")
	call(t, 1, "generate", "--repo", repo, "--out", human, "--force")
	b, _ = os.ReadFile(human)
	if string(b) != "Human-authored decisions\n" {
		t.Fatal("overwrote human prose")
	}
}

func TestCLIReplaysExclusionsAndMissingEvidence(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"^19"}}`)
	write(t, repo, "bin/check.py", "print('hello')\n")
	policy := write(t, work, "policy.json", `{"schema_version":1,"repository":"Demo","rules":{"python":{"exclude":true,"rationale":"Team exclusion"}}}`)
	previous := filepath.Join(work, "previous.html")
	call(t, 0, "generate", "--repo", repo, "--policy", policy, "--format", "html", "--out", previous)
	write(t, repo, "package.json", `{"dependencies":{}}`)
	text := call(t, 0, "scan", "--repo", repo, "--previous", previous)
	r, err := radar.ReadReport([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.ID == "python" {
			t.Fatal("forgot prior exclusion")
		}
		if f.ID == "react" && f.Presence != "not detected" {
			t.Fatal("forgot missing dependency")
		}
	}
}

func TestCLIRejectsSymlinkAndInvalidArguments(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "go.mod", "module demo\n\ngo 1.25\n")
	target := write(t, work, "target.md", "Do not change\n")
	link := filepath.Join(work, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	call(t, 1, "generate", "--repo", repo, "--out", link, "--force")
	linkDir := filepath.Join(work, "linked-dir")
	if err := os.Symlink(work, linkDir); err != nil {
		t.Fatal(err)
	}
	call(t, 1, "generate", "--repo", repo, "--out", filepath.Join(linkDir, "new.md"))
	call(t, 1, "generate", "--repo", repo, "--format", "xml")
	call(t, 1, "generate", "--repo", repo, "unexpected")
	call(t, 1, "generate")
	call(t, 0, "generate", "--help")
	if text := call(t, 0, "catalog"); !strings.Contains(text, `"phoenix"`) {
		t.Fatal("catalog command missing classification options")
	}
}
