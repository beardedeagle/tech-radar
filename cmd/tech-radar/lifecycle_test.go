package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tech-radar/internal/radar"
)

func TestCLILifecycleBaselineReplayAndPortableSnapshot(t *testing.T) {
	for _, format := range []string{"markdown", "html", "json"} {
		t.Run(format, func(t *testing.T) {
			repo, work := t.TempDir(), t.TempDir()
			write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
			policy := write(t, work, "policy.json", `{"schema_version":1,"rules":{"react":{"lifecycle":"eXpErImEnT","status":"approved","scope":"UI","rationale":"Team review","owner":"UI team","reviewed":"2026-11-03"}}}`)
			dest := filepath.Join(work, "baseline."+map[string]string{"markdown": "md", "html": "html", "json": "json"}[format])
			call(t, 0, "generate", "--repo", repo, "--model", "lifecycle", "--policy", policy, "--format", format, "--out", dest)
			first, err := readPrevious(dest)
			if err != nil {
				t.Fatal(err)
			}
			if first.Model != radar.LifecycleModel || !first.Baseline || first.Findings[0].ReviewRequired || first.Findings[0].MissingDecisionMaterial {
				t.Fatal("CLI baseline lost supplied lifecycle approval")
			}
			for _, command := range []string{"scan", "generate"} {
				text := call(t, 0, command, "--repo", repo, "--previous", dest, "--format", "json")
				r, err := radar.ReadReport([]byte(text))
				if err != nil {
					t.Fatal(err)
				}
				if r.Model != radar.LifecycleModel || r.Baseline || !r.Findings[0].MissingDecisionMaterial || !r.Findings[0].ReviewRequired || r.Findings[0].EvidenceChanged {
					t.Fatal("CLI inherited baseline without material gap")
				}
			}
			call(t, 0, "generate", "--repo", repo, "--previous", dest, "--format", format, "--out", dest, "--force")
			second, _ := os.ReadFile(dest)
			companions, _ := filepath.Glob(filepath.Join(work, "baseline.evidence.*.json"))
			for _, companion := range companions {
				if err := os.Remove(companion); err != nil {
					t.Fatal(err)
				}
			}
			call(t, 0, "generate", "--repo", repo, "--previous", dest, "--format", format, "--out", dest, "--force")
			third, _ := os.ReadFile(dest)
			if !bytes.Equal(second, third) {
				t.Fatal("portable later rerun changed output")
			}
		})
	}
}

func TestCLIModelSelectionAndInvalidCombinations(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
	policy := write(t, work, "policy.json", `{"schema_version":1,"model":"lifecycle","rules":{}}`)
	for _, args := range [][]string{
		{"scan", "--repo", repo, "--model", "unknown"},
		{"generate", "--repo", repo, "--model", "lifecycle", "--format", "lifecycle"},
		{"scan", "--repo", repo, "--model", "recommendation", "--policy", policy},
	} {
		call(t, 1, args...)
	}
	inherited := call(t, 0, "scan", "--repo", repo, "--policy", policy)
	r, err := radar.ReadReport([]byte(inherited))
	if err != nil || r.Model != radar.LifecycleModel {
		t.Fatalf("policy model inheritance failed: %v", err)
	}
	defaultReport := call(t, 0, "scan", "--repo", repo)
	r, err = radar.ReadReport([]byte(defaultReport))
	if err != nil || r.Model != radar.RecommendationModel {
		t.Fatalf("default model changed: %v", err)
	}
	dest := write(t, work, "old.json", defaultReport)
	switched := call(t, 0, "scan", "--repo", repo, "--previous", dest, "--model", "lifecycle")
	r, err = radar.ReadReport([]byte(switched))
	if err != nil || r.Baseline || !r.Findings[0].MissingDecisionMaterial || r.Findings[0].Proposal != nil {
		t.Fatalf("old-model previous missed review gap: %v", err)
	}
	if !strings.Contains(call(t, 0, "--help"), "--model recommendation|lifecycle") {
		t.Fatal("model help missing")
	}
}
