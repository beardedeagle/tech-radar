package radar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"tech-radar/internal/jsonutil"
)

const snapshotStart = "\n<!-- tech-radar:snapshot\n"
const snapshotEnd = "\ntech-radar:snapshot -->\n"

type snapshot struct {
	BodySHA256 string `json:"body_sha256"`
	Report     Report `json:"report"`
}

func Render(report Report, format string) ([]byte, error) {
	if err := validateReport(report); err != nil {
		return nil, err
	}
	report.View = report.selectedView()
	if format == "json" {
		b, err := json.MarshalIndent(report, "", "  ")
		return append(b, '\n'), err
	}
	var body []byte
	var err error
	switch format {
	case "markdown":
		body = renderMarkdown(report)
	case "html":
		body, err = renderHTML(report)
	default:
		return nil, fmt.Errorf("unknown output format %q", format)
	}
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	state, err := json.MarshalIndent(snapshot{hex.EncodeToString(digest[:]), report}, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, []byte(snapshotStart)...)
	body = append(body, state...)
	body = append(body, []byte(snapshotEnd)...)
	return body, nil
}

func ReadReport(data []byte) (Report, error) {
	var report Report
	if index := bytes.Index(data, []byte(snapshotStart)); index >= 0 {
		remaining := data[index+len(snapshotStart):]
		end := bytes.Index(remaining, []byte(snapshotEnd))
		if end < 0 || len(bytes.TrimSpace(remaining[end+len(snapshotEnd):])) > 0 {
			return report, fmt.Errorf("incomplete snapshot or additional content after snapshot")
		}
		var state snapshot
		if err := jsonutil.DecodeStrict(remaining[:end], &state); err != nil {
			return report, fmt.Errorf("snapshot: %w", err)
		}
		digest := sha256.Sum256(data[:index])
		if hex.EncodeToString(digest[:]) != state.BodySHA256 {
			return report, fmt.Errorf("generated document body was edited; preserve those edits and express decisions in policy before regenerating")
		}
		report = state.Report
	} else if err := jsonutil.DecodeStrict(data, &report); err != nil {
		return report, fmt.Errorf("not a tech-radar JSON report or self-contained export: %w", err)
	}
	return report, validateReport(report)
}

func validateReport(r Report) error {
	if (r.SchemaVersion != 1 && r.SchemaVersion != 2) || r.GeneratorVersion == "" {
		return fmt.Errorf("unsupported or missing report schema/version")
	}
	if len(r.Source.Fingerprint) != 64 {
		return fmt.Errorf("invalid source fingerprint")
	}
	if _, err := hex.DecodeString(r.Source.Fingerprint); err != nil {
		return fmt.Errorf("invalid source fingerprint")
	}
	if r.Source.Files < 0 {
		return fmt.Errorf("invalid source file count")
	}
	if r.Source.ContextFingerprint != "" && (!validDigest(r.Source.ContextFingerprint) || !supportedPortfolioVersion(r.Source.CorpusVersion)) {
		return fmt.Errorf("invalid portfolio context provenance")
	}
	if r.Source.RulesFingerprint != "" {
		if b, err := hex.DecodeString(r.Source.RulesFingerprint); err != nil || len(b) != 32 {
			return fmt.Errorf("invalid run rules fingerprint")
		}
	}
	if (r.Source.GroupingFingerprint != "" && !validDigest(r.Source.GroupingFingerprint)) || (r.Source.GroupingCatalog != "" && !validDigest(r.Source.GroupingCatalog)) {
		return fmt.Errorf("invalid grouping provenance")
	}
	if r.EvidenceFile != "" || r.EvidenceSHA256 != "" {
		if !fs.ValidPath(r.EvidenceFile) || path.Base(r.EvidenceFile) != r.EvidenceFile || path.Ext(r.EvidenceFile) != ".json" {
			return fmt.Errorf("invalid evidence companion filename")
		}
		if b, err := hex.DecodeString(r.EvidenceSHA256); err != nil || len(b) != 32 {
			return fmt.Errorf("invalid evidence companion digest")
		}
	}
	if err := validateModel(r.Model); err != nil {
		return err
	}
	if err := validateView(r.View); err != nil {
		return err
	}
	if r.Baseline && r.Model != LifecycleModel {
		return fmt.Errorf("baseline marker requires lifecycle model")
	}
	if err := validateSavedDecisions(r); err != nil {
		return err
	}
	ids := map[string]bool{}
	raw := map[string]Finding{}
	numbers := map[int]bool{}
	for _, f := range r.Findings {
		if !validID.MatchString(f.ID) || ids[f.ID] {
			return fmt.Errorf("invalid or duplicate finding id %q", f.ID)
		}
		ids[f.ID] = true
		raw[f.ID] = f
		if f.Number < 1 || numbers[f.Number] {
			return fmt.Errorf("invalid or duplicate display number %d", f.Number)
		}
		numbers[f.Number] = true
		if f.Quadrant != "" && !contains(Quadrants, f.Quadrant) {
			return fmt.Errorf("%s: invalid finding quadrant", f.ID)
		}
		if f.Presence != "detected" && f.Presence != "not detected" {
			return fmt.Errorf("%s: invalid presence", f.ID)
		}
		if r.Model == LifecycleModel && (f.MissingDecisionMaterial != (!r.Baseline && strings.TrimSpace(f.Decision.Reference) == "") || (f.MissingDecisionMaterial && !f.ReviewRequired)) {
			return fmt.Errorf("%s: inconsistent missing decision material review state", f.ID)
		}
		if f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("%s: finding and policy decisions disagree", f.ID)
		}
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		if f.EvidenceCount < len(f.Evidence) {
			return fmt.Errorf("%s: invalid evidence count", f.ID)
		}
		for _, e := range f.Evidence {
			if !fs.ValidPath(e.Path) || e.Path == "." || e.Line < 1 {
				return fmt.Errorf("%s: invalid relative evidence location", f.ID)
			}
		}
	}
	for _, c := range r.Components {
		if !fs.ValidPath(c.Path) || !knownProfile(c.Profile) {
			return fmt.Errorf("invalid component profile/path")
		}
	}
	for _, f := range r.ExcludedFindings {
		if ids[f.ID] || !validID.MatchString(f.ID) || !contains(r.Excluded, f.ID) || !r.Decisions[f.ID].Exclude || f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("invalid excluded evidence %s", f.ID)
		}
		ids[f.ID] = true
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		for _, e := range f.Evidence {
			if !fs.ValidPath(e.Path) || e.Path == "." || e.Line < 1 {
				return fmt.Errorf("invalid excluded evidence path")
			}
		}
	}
	grouped := map[string]bool{}
	familyIDs := map[string]bool{}
	for _, f := range r.Highlights {
		if f.FamilyID != "" {
			key := familyIdentity(f)
			if familyIDs[key] {
				return fmt.Errorf("duplicate family/component highlight %s", f.FamilyID)
			}
			familyIDs[key] = true
		}
		original, ok := raw[f.ID]
		if !ok || f.Number != original.Number || f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("highlight does not match source finding %s", f.ID)
		}
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		if r.Model == LifecycleModel && (f.MissingDecisionMaterial != original.MissingDecisionMaterial || (f.MissingDecisionMaterial && !f.ReviewRequired)) {
			return fmt.Errorf("%s: highlight lost missing decision material review state", f.ID)
		}
		if !contains(f.Members, f.ID) {
			return fmt.Errorf("highlight omits its root %s", f.ID)
		}
		for _, id := range f.Members {
			if _, exists := raw[id]; !exists || grouped[id] {
				return fmt.Errorf("invalid/duplicate grouped member %s", id)
			}
			if id != f.ID && !groupableDecisions(r.Model, f.Decision, r.Decisions[id]) {
				return fmt.Errorf("group hides explicit decision %s", id)
			}
			grouped[id] = true
		}
	}
	return nil
}

func validateInference(f Finding, model Model) error {
	if f.MissingDecisionMaterial && model != LifecycleModel {
		return fmt.Errorf("%s: material gap requires lifecycle model", f.ID)
	}
	if model == LifecycleModel && f.Proposal != nil {
		return fmt.Errorf("%s: lifecycle placement requires a supplied decision", f.ID)
	}
	if (f.FamilyID != "" && (!validID.MatchString(f.FamilyID) || len(f.FamilyID) > 128 || !fs.ValidPath(f.GroupComponent))) || (f.FamilyID == "" && f.GroupComponent != "") {
		return fmt.Errorf("%s: invalid family identity or component", f.ID)
	}
	if c := f.PlacementChange; c != nil {
		if !contains([]string{"new", "unchanged", "moved", "moved-in", "moved-out"}, c.Kind) || (c.PreviousRing != "" && !contains(append(append([]string{}, model.rings()...), model.pending()), c.PreviousRing)) || (c.PreviousQuadrant != "" && !contains(Quadrants, c.PreviousQuadrant)) {
			return fmt.Errorf("%s: invalid placement comparison", f.ID)
		}
		if (c.Kind == "new" && (c.PreviousRing != "" || c.PreviousQuadrant != "")) || (c.Kind != "new" && c.PreviousRing == "") {
			return fmt.Errorf("%s: incomplete placement comparison", f.ID)
		}
	}
	if f.Assessment != nil {
		if !contains([]string{"unknown", "system_defining", "workflow_enabling", "implementation_support"}, f.Assessment.Significance) || !contains([]string{"unknown", "documentation", "reference", "declaration", "configuration", "static wiring"}, f.Assessment.Confidence) {
			return fmt.Errorf("%s: invalid assessment", f.ID)
		}
	}
	if f.Proposal != nil {
		if f.Proposal.Status != "proposed" || f.Proposal.Exclude || f.Proposal.Owner != "" || f.Proposal.Reference != "" || f.Proposal.Reviewed != "" {
			return fmt.Errorf("%s: inference cannot claim approval", f.ID)
		}
		if err := validateRules(map[string]Decision{f.ID: *f.Proposal}); err != nil {
			return err
		}
	}
	return nil
}

func ring(f Finding) string {
	if f.Decision.Model == LifecycleModel || f.Decision.Ring == "" {
		if f.Proposal != nil {
			return f.Proposal.Ring
		}
		return "Needs review"
	}
	return f.Decision.Ring
}
func quadrant(f Finding) string {
	if f.Quadrant == "" {
		return "Unclassified"
	}
	return f.Quadrant
}
func (m Model) status(f Finding) string {
	label := m.displayDecision(f).Status
	if label == "" {
		label = "unreviewed"
		if m != LifecycleModel && f.Proposal != nil {
			label = "suggested"
		}
	}
	if f.MissingDecisionMaterial {
		label += " - missing decision material requires review"
	}
	if f.Presence != "detected" {
		return label + "; not detected - review required"
	}
	if f.EvidenceChanged {
		return label + "; evidence changed - review required"
	}
	if f.ReviewRequired && !f.MissingDecisionMaterial {
		return label + "; review required"
	}
	return label
}

const coverage = "Reads manifests, direct dependency locks, tool pins, language counts and selected source, configuration and documentation references. Unknown dependencies stay unclassified. Dynamic code and ambiguous references need review. Runtime behavior, deployed versions and vendor accounts aren't checked."
const boundaries = "Skips dependency/build/generated directories, tests, credentials, symlinks, binary files and files over 2 MiB. Warnings list size and binary skips. Counts cover scanned files. Repo scripts, builds and hooks aren't run. Git HEAD is read when available."
