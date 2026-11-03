package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func TestCLILifecycleSwitchPreservesMetadataOnlyDecision(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
	policy := write(t, work, "policy.json", `{"schema_version":1,"rules":{"react":{"lifecycle":"maintain"}}}`)
	original := call(t, 0, "scan", "--repo", repo, "--policy", policy)
	previous := write(t, work, "previous.json", original)
	for _, format := range []string{"json", "markdown", "html"} {
		text := call(t, 0, "generate", "--repo", repo, "--previous", previous, "--model", "lifecycle", "--format", format)
		r, err := radar.ReadReport([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		f := r.Findings[0]
		if r.Model != radar.LifecycleModel || r.Baseline || f.Decision != (radar.Decision{Lifecycle: "maintain"}) || f.Proposal != nil || !f.MissingDecisionMaterial || !f.ReviewRequired {
			t.Fatalf("metadata-only switch lost historical state: %#v", f)
		}
		if format != "json" && !strings.Contains(strings.SplitN(text, "\n<!-- tech-radar:snapshot\n", 2)[0], "Needs decision") {
			t.Fatal("historical metadata supplied lifecycle placement")
		}
		saved := write(t, work, "switched.json", call(t, 0, "scan", "--repo", repo, "--previous", previous, "--model", "lifecycle"))
		call(t, 0, "scan", "--repo", repo, "--previous", saved)
	}
	call(t, 1, "scan", "--repo", repo, "--model", "lifecycle", "--policy", policy)
}

func TestCLIExcludedHistoryOnModelSwitch(t *testing.T) {
	for _, tt := range []struct {
		source, target                radar.Model
		ring, lifecycle, previousRing string
	}{
		{radar.RecommendationModel, radar.LifecycleModel, "Trial", "", "Trial"},
		{radar.LifecycleModel, radar.RecommendationModel, "", "Hold", "Hold"},
	} {
		t.Run(string(tt.source), func(t *testing.T) {
			repo, work := t.TempDir(), t.TempDir()
			manifest := write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
			p := radar.Policy{SchemaVersion: 1, Model: tt.source, Rules: map[string]radar.Decision{"react": {Ring: tt.ring, Lifecycle: tt.lifecycle, Scope: "UI", Rationale: "Team proposal"}}}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			policy := write(t, work, "policy.json", string(data))
			first := write(t, work, "first.json", call(t, 0, "scan", "--repo", repo, "--policy", policy))
			write(t, repo, "package.json", `{"dependencies":{"react":"20"}}`)
			second := call(t, 0, "scan", "--repo", repo, "--previous", first)
			prior, err := radar.ReadReport([]byte(second))
			if err != nil {
				t.Fatal(err)
			}
			old := prior.Findings[0]
			if old.PlacementChange == nil || old.PlacementChange.PreviousRing != tt.previousRing {
				t.Fatalf("fixture lacks non-shared previous ring: %#v", old.PlacementChange)
			}
			previous := write(t, work, "second.json", second)
			if err := os.Remove(manifest); err != nil {
				t.Fatal(err)
			}
			exclusion := write(t, work, "exclusion.json", `{"schema_version":1,"rules":{"react":{"exclude":true,"rationale":"Team excludes removed UI"}}}`)
			for _, model := range []radar.Model{tt.source, tt.target} {
				text := call(t, 0, "scan", "--repo", repo, "--previous", previous, "--model", string(model), "--policy", exclusion)
				r, err := radar.ReadReport([]byte(text))
				if err != nil {
					t.Fatal(err)
				}
				if len(r.Findings) != 0 || len(r.ExcludedFindings) != 1 {
					t.Fatal("excluded technology was lost or included")
				}
				f := r.ExcludedFindings[0]
				if f.Presence != "not detected" || !f.Decision.Exclude || f.Decision.Rationale != "Team excludes removed UI" || !reflect.DeepEqual(f.Evidence, old.Evidence) || f.EvidenceFingerprint != old.EvidenceFingerprint || f.EvidenceCount != old.EvidenceCount {
					t.Fatal("exclusion lost its evidence or decision")
				}
				if model == tt.target && f.PlacementChange != nil {
					t.Fatal("model switch retained incomparable excluded history")
				}
				if model == tt.source && !reflect.DeepEqual(f.PlacementChange, old.PlacementChange) {
					t.Fatal("same-model exclusion lost placement history")
				}
			}
		})
	}
}

func TestCLILifecycleHTMLShowsReviewDateWithoutReference(t *testing.T) {
	for _, standalone := range []bool{false, true} {
		t.Run(fmt.Sprint(standalone), func(t *testing.T) {
			repo, work := t.TempDir(), t.TempDir()
			write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
			policy := write(t, work, "policy.json", `{"schema_version":1,"model":"lifecycle","rules":{"react":{"lifecycle":"Adopt","status":"approved","scope":"UI","rationale":"Team review","owner":"UI team","reviewed":"2026-11-03"}}}`)
			dest := filepath.Join(work, "radar.html")
			args := []string{"generate", "--repo", repo, "--policy", policy, "--format", "html", "--out", dest}
			if standalone {
				args = append(args, "--self-contained")
			}
			call(t, 0, args...)
			data, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.SplitN(string(data), "\n<!-- tech-radar:snapshot\n", 2)[0]
			if !strings.Contains(body, "<strong>Reviewed:</strong> 2026-11-03") {
				t.Fatal("visible HTML omitted reference-free approval date")
			}
		})
	}
}
