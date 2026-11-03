package radar

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"tech-radar/internal/jsonutil"
)

// ReadPolicy validates policy using its explicit model or recommendation.
func ReadPolicy(data []byte) (Policy, error) {
	return ReadPolicyForModel(data, RecommendationModel)
}

// ReadPolicyForModel validates policy using its explicit model or the supplied
// fallback. A zero fallback uses recommendation semantics.
func ReadPolicyForModel(data []byte, fallback Model) (Policy, error) {
	var policy Policy
	if err := jsonutil.DecodeStrict(data, &policy); err != nil {
		return policy, fmt.Errorf("policy: %w", err)
	}
	if policy.SchemaVersion != 1 {
		return policy, fmt.Errorf("policy: unsupported schema_version %d (want 1)", policy.SchemaVersion)
	}
	model := policy.Model
	if model == "" {
		model = fallback
	}
	if err := validateModel(model); err != nil {
		return policy, err
	}
	return policy, validateModelRules(policy.Rules, model)
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_@][a-zA-Z0-9_@./:\-]*$`)

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateRules(rules map[string]Decision) error {
	return validateModelRules(rules, RecommendationModel)
}

func validateModelRules(rules map[string]Decision, model Model) error {
	for _, id := range sortedKeys(rules) {
		d := rules[id]
		if !validID.MatchString(id) {
			return fmt.Errorf("invalid technology id %q", id)
		}
		if d.Quadrant != "" && !contains(Quadrants, d.Quadrant) {
			return fmt.Errorf("%s: invalid quadrant %q", id, d.Quadrant)
		}
		if d.Ring != "" && !contains(Rings, d.Ring) {
			return fmt.Errorf("%s: invalid ring %q; lifecycle is a separate field", id, d.Ring)
		}
		if d.Status != "" && d.Status != "proposed" && d.Status != "approved" {
			return fmt.Errorf("%s: status must be proposed or approved", id)
		}
		lifecycle := model == LifecycleModel && canonicalLifecycle(d.Lifecycle) != ""
		if model == LifecycleModel && d.Lifecycle != "" && !lifecycle {
			return fmt.Errorf("%s: invalid lifecycle %q", id, d.Lifecycle)
		}
		if d.Exclude {
			if d.Ring != "" || d.Status != "" || lifecycle {
				return fmt.Errorf("%s: exclusion cannot also recommend or approve", id)
			}
			if strings.TrimSpace(d.Rationale) == "" {
				return fmt.Errorf("%s: exclusion needs a rationale", id)
			}
		}
		if (d.Ring != "" || lifecycle) && (strings.TrimSpace(d.Scope) == "" || strings.TrimSpace(d.Rationale) == "") {
			return fmt.Errorf("%s: a recommendation needs scope and rationale", id)
		}
		if d.Reviewed != "" {
			if _, err := time.Parse("2006-01-02", d.Reviewed); err != nil {
				return fmt.Errorf("%s: reviewed must be a valid YYYY-MM-DD date", id)
			}
		}
		if model == LifecycleModel && d.Status == "approved" && !lifecycle {
			return fmt.Errorf("%s: lifecycle approval needs a lifecycle decision", id)
		}
		if d.Status == "approved" && lifecycle && (strings.TrimSpace(d.Owner) == "" || strings.TrimSpace(d.Scope) == "" || strings.TrimSpace(d.Rationale) == "" || d.Reviewed == "") {
			return fmt.Errorf("%s: lifecycle approval needs lifecycle, scope, rationale, owner and reviewed date", id)
		}
		if d.Status == "approved" && !lifecycle && (d.Ring == "" || strings.TrimSpace(d.Owner) == "" || strings.TrimSpace(d.Reference) == "" || d.Reviewed == "") {
			return fmt.Errorf("%s: approval needs ring, scope, rationale, owner, reference and reviewed date", id)
		}
	}
	return nil
}

func Build(inv Inventory, policy Policy, previous *Report, name string) (Report, error) {
	return BuildWithGrouping(inv, policy, previous, name, GroupingRules{})
}

// BuildWithGrouping applies explicit presentation rules while retaining raw
// finding identities, evidence and policy. Invalid catalog inputs return errors.
func BuildWithGrouping(inv Inventory, policy Policy, previous *Report, name string, grouping GroupingRules) (Report, error) {
	return BuildWithOptions(inv, policy, previous, name, BuildOptions{Grouping: grouping})
}

// BuildOptions selects a report model, view and presentation grouping. The zero
// value inherits the model and same-model view, and uses built-in grouping.
type BuildOptions struct {
	Model    Model
	View     View
	Grouping GroupingRules
}

// BuildWithOptions builds a report without translating decisions between models.
// An explicit model must agree with an explicit policy model.
func BuildWithOptions(inv Inventory, policy Policy, previous *Report, name string, options BuildOptions) (Report, error) {
	model, err := selectModel(options.Model, policy.Model, previous)
	if err != nil {
		return Report{}, err
	}
	view, err := selectView(options.View, model, previous)
	if err != nil {
		return Report{}, err
	}
	grouping := options.Grouping
	if grouping.families == nil {
		var err error
		grouping, err = defaultGrouping()
		if err != nil {
			return Report{}, err
		}
	}
	inv.Source.GroupingFingerprint = grouping.fingerprint
	inv.Source.GroupingCatalog = grouping.catalogDigest
	if policy.SchemaVersion != 0 && policy.SchemaVersion != 1 {
		return Report{}, fmt.Errorf("unsupported policy schema_version %d", policy.SchemaVersion)
	}
	if err := validateModelRules(policy.Rules, model); err != nil {
		return Report{}, err
	}
	report := Report{Model: model, View: view, Baseline: model == LifecycleModel && previous == nil, SchemaVersion: 2, GeneratorVersion: Version, Repository: "Repository", Source: inv.Source, Findings: []Finding{}, Decisions: map[string]Decision{}, Excluded: []string{}, Warnings: append([]string{}, inv.Warnings...)}
	report.Languages = append([]LanguageStat{}, inv.Languages...)
	report.Components = append([]Component(nil), inv.Components...)
	rulesChanged := previous != nil && previous.Source.RulesFingerprint != inv.Source.RulesFingerprint
	contextChanged := previous != nil && previous.Source.ContextFingerprint != inv.Source.ContextFingerprint
	groupingChanged := previous != nil && (previous.Source.GroupingFingerprint != inv.Source.GroupingFingerprint || previous.Source.GroupingCatalog != inv.Source.GroupingCatalog)
	classificationChanged := previous != nil && previous.RuleVersion != ruleVersion
	prior := map[string]Finding{}
	if previous != nil {
		if previous.SchemaVersion != 1 && previous.SchemaVersion != 2 {
			return Report{}, fmt.Errorf("unsupported previous schema_version %d", previous.SchemaVersion)
		}
		if err := validateModel(previous.Model); err != nil {
			return Report{}, err
		}
		if err := validateSavedDecisions(*previous); err != nil {
			return Report{}, fmt.Errorf("previous decisions: %w", err)
		}
		report.Repository = previous.Repository
		for id, d := range previous.Decisions {
			if model != previous.Model.effective() && d.Status == "approved" {
				_, replaced := policy.Rules[id]
				if !replaced && (model == RecommendationModel || canonicalLifecycle(d.Lifecycle) != "") {
					return Report{}, fmt.Errorf("%s: model switch requires an explicit %s policy replacement for the saved approval", id, model)
				}
			}
			report.Decisions[id] = d
		}
		for _, f := range previous.Findings {
			if _, ok := prior[f.ID]; ok {
				return Report{}, fmt.Errorf("duplicate prior finding %s", f.ID)
			}
			prior[f.ID] = f
		}
		for _, f := range previous.ExcludedFindings {
			if _, exists := prior[f.ID]; exists {
				return Report{}, fmt.Errorf("duplicate prior finding %s", f.ID)
			}
			prior[f.ID] = f
		}
	}
	if policy.Repository != "" {
		report.Repository = policy.Repository
	}
	if name != "" {
		report.Repository = name
	}
	for id, d := range policy.Rules {
		if model == LifecycleModel {
			if canonical := canonicalLifecycle(d.Lifecycle); canonical != "" {
				d.Lifecycle = canonical
			}
		}
		if (d.Ring != "" || (model == LifecycleModel && canonicalLifecycle(d.Lifecycle) != "")) && d.Status == "" {
			d.Status = "proposed"
		}
		report.Decisions[id] = d
	}
	current := map[string]Finding{}
	for _, f := range inv.Findings {
		if _, ok := current[f.ID]; ok {
			return Report{}, fmt.Errorf("duplicate detected finding %s", f.ID)
		}
		current[f.ID] = f
	}
	retainSavedPackageIDs(current, prior, report.Decisions)
	for id, f := range prior {
		if _, ok := current[id]; !ok {
			f.Presence = "not detected"
			current[id] = f
		}
	}
	known := map[string]Technology{}
	for _, tech := range Catalog() {
		known[tech.ID] = tech
	}
	for _, tech := range inv.Technologies {
		known[tech.ID] = tech
	}
	nextNumber := 1
	for _, f := range prior {
		if f.Number >= nextNumber {
			nextNumber = f.Number + 1
		}
	}
	for _, id := range sortedKeys(report.Decisions) {
		d := report.Decisions[id]
		if _, ok := current[id]; ok || d.Exclude {
			continue
		}
		tech, knownID := known[id]
		if !knownID && (d.Name == "" || d.Quadrant == "") {
			return Report{}, fmt.Errorf("%s: unknown rule id; a manual candidate needs name and quadrant", id)
		}
		if !knownID {
			tech = Technology{ID: id, Name: d.Name, Quadrant: d.Quadrant}
		}
		current[id] = Finding{ID: id, Name: tech.Name, Quadrant: tech.Quadrant, Presence: "not detected", Evidence: []Evidence{}}
	}
	for _, id := range sortedKeys(current) {
		f := current[id]
		d := report.Decisions[id]
		f.Evidence = append([]Evidence{}, f.Evidence...)
		f.Decision = d
		f.MissingDecisionMaterial = false
		if model == LifecycleModel {
			f.Proposal = nil
		}
		if d.Exclude {
			if previous != nil && model != previous.Model.effective() {
				f.PlacementChange = nil
			}
			report.Excluded = append(report.Excluded, id)
			report.ExcludedFindings = append(report.ExcludedFindings, f)
			continue
		}
		if old, ok := prior[id]; ok && old.Number > 0 {
			f.Number = old.Number
		} else {
			f.Number = nextNumber
			nextNumber++
		}
		if d.Name != "" {
			f.Name = d.Name
		}
		if d.Quadrant != "" {
			f.Quadrant = d.Quadrant
		}
		if old, ok := prior[id]; ok {
			changed := f.EvidenceFingerprint != old.EvidenceFingerprint || f.EvidenceCount != old.EvidenceCount
			// Renaming or reclassifying a technology needs another review.
			changed = changed || (rulesChanged && ((d.Name == "" && old.Name != f.Name) || (d.Quadrant == "" && old.Quadrant != f.Quadrant)))
			if contextChanged || groupingChanged || classificationChanged || inv.Source.ContextFingerprint != "" {
				assessment := assessPortfolio(f, inv.Components)
				changed = changed || (d.Name == "" && old.Name != f.Name) || (d.Quadrant == "" && old.Quadrant != f.Quadrant)
				if old.Assessment != nil {
					changed = changed || !reflect.DeepEqual(old.Assessment, &assessment)
				}
			}
			if f.EvidenceFingerprint == "" || old.EvidenceFingerprint == "" {
				changed = changed || !reflect.DeepEqual(f.Evidence, old.Evidence)
			}
			f.EvidenceChanged = old.EvidenceChanged || changed
			if replacement, explicit := policy.Rules[id]; explicit {
				reviewed := replacement.Reference != old.Decision.Reference || replacement.Reviewed != old.Decision.Reviewed
				if model == LifecycleModel {
					reviewed = (strings.TrimSpace(replacement.Reference) != "" && replacement.Reference != old.Decision.Reference) || (replacement.Reviewed != "" && replacement.Reviewed != old.Decision.Reviewed)
				}
				if reviewed {
					f.EvidenceChanged = false
				}
			}
		}
		f.MissingDecisionMaterial = model == LifecycleModel && previous != nil && strings.TrimSpace(d.Reference) == ""
		f.ReviewRequired = f.MissingDecisionMaterial || (model != LifecycleModel && d.Ring == "") || (model == LifecycleModel && canonicalLifecycle(d.Lifecycle) == "") || f.Presence != "detected" || f.EvidenceChanged || d.Status != "approved" || f.Quadrant == ""
		report.Findings = append(report.Findings, f)
	}
	sort.Strings(report.Excluded)
	sort.Strings(report.Warnings)
	synthesizeWithGrouping(&report, grouping)
	comparePlacements(&report, previous)
	return report, nil
}
