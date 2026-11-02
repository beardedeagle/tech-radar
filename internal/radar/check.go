package radar

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// CheckOptions supplies the evaluation date, explicit exceptions and an optional
// baseline. A nil baseline means no history; dates use YYYY-MM-DD.
type CheckOptions struct {
	AsOf       string
	Exceptions CheckExceptions
	Baseline   []RepositorySnapshot
}

// Check evaluates every detected observation against explicit permission. Deny
// rules take priority; approved active exceptions can override them. It never
// uses radar rings, grouping or model proposals to grant permission.
// Invalid inputs return an error; undecidable checks are retained as gaps.
func Check(snapshots []RepositorySnapshot, policy CheckPolicy, options CheckOptions) (CheckResult, error) {
	if policy.fingerprint == "" || len(snapshots) == 0 {
		return CheckResult{}, fmt.Errorf("check requires a validated policy and at least one repository")
	}
	if _, err := time.Parse(time.DateOnly, options.AsOf); err != nil {
		return CheckResult{}, fmt.Errorf("as-of must be YYYY-MM-DD")
	}
	if policy.data.Approval.Reviewed > options.AsOf {
		return CheckResult{}, fmt.Errorf("policy approval is later than as-of")
	}
	if options.Exceptions.fingerprint != "" && options.Exceptions.data.PolicyFingerprint != policy.fingerprint {
		return CheckResult{}, fmt.Errorf("exceptions belong to another policy")
	}
	r := CheckResult{Kind: "tech-radar-check", SchemaVersion: 1, GeneratorVersion: Version, PolicyID: policy.data.ID, PolicyVersion: policy.data.Version, PolicyFingerprint: policy.fingerprint, ExceptionsFingerprint: options.Exceptions.fingerprint, AsOf: options.AsOf, Complete: true, Repositories: []CheckedRepository{}}
	prior := map[string]RepositorySnapshot{}
	for _, s := range options.Baseline {
		if _, exists := prior[s.ID]; exists {
			return CheckResult{}, fmt.Errorf("duplicate baseline repository %q", s.ID)
		}
		prior[s.ID] = s
	}
	current := copySnapshots(snapshots)
	sort.Slice(current, func(i, j int) bool { return current[i].ID < current[j].ID })
	seen := map[string]bool{}
	for _, snapshot := range current {
		if snapshot.ID == "" || seen[snapshot.ID] {
			return CheckResult{}, fmt.Errorf("missing/duplicate repository identity")
		}
		seen[snapshot.ID] = true
		checked := CheckedRepository{Snapshot: snapshot, Violations: []CheckIssue{}, Gaps: []CheckIssue{}, Drift: []CheckDrift{}}
		for _, warning := range snapshot.Inventory.Warnings {
			checked.Gaps = append(checked.Gaps, CheckIssue{Code: "scan-warning", Message: warning})
		}
		if snapshot.Status != "captured" && snapshot.Status != "empty" {
			checked.Gaps = append(checked.Gaps, CheckIssue{Code: "uncaptured-repository", Message: "Repository has no complete capture."})
		}
		if snapshot.Status != "empty" && snapshot.Inventory.Source.Files == 0 {
			checked.Gaps = append(checked.Gaps, CheckIssue{Code: "no-eligible-files", Message: "No eligible files were captured; coverage is unknown."})
		}
		old, hasOld := prior[snapshot.ID]
		comparable := hasOld && old.Inventory.Source.RulesFingerprint == snapshot.Inventory.Source.RulesFingerprint && old.Inventory.Source.CorpusVersion == snapshot.Inventory.Source.CorpusVersion && completeCapture(old)
		if hasOld && !comparable {
			checked.Gaps = append(checked.Gaps, CheckIssue{Code: "baseline-incomparable", Message: "Baseline detection inputs, capture version or coverage differ; repeat collection with matching inputs."})
		}
		if options.Baseline != nil && (!hasOld || comparable) {
			checked.Drift = checkDrift(snapshot.Inventory, old.Inventory, len(snapshot.Inventory.Warnings) == 0 && snapshot.Status == "captured" && snapshot.Inventory.Source.Files > 0)
		}
		uses := baselineUses(old.Inventory)
		for _, f := range snapshot.Inventory.Findings {
			if f.Presence != "detected" {
				continue
			}
			if len(f.Evidence) == 0 {
				checked.Gaps = append(checked.Gaps, CheckIssue{Technology: f.ID, Code: "missing-evidence", Message: "Detected finding has no evidence locations."})
				continue
			}
			for _, e := range f.Evidence {
				var issue *CheckIssue
				var exception string
				for _, override := range options.Exceptions.data.Exceptions {
					if override.Approval.Reviewed <= options.AsOf && options.AsOf <= override.Expires && checkRuleMatches(override.CheckRule, snapshot.ID, f.ID, e) {
						allowed, unknown := checkVersion(override.CheckRule, f, e)
						if allowed && !unknown {
							exception = override.ID
							break
						}
					}
				}
				if exception != "" {
					checked.Exceptions = append(checked.Exceptions, exception)
					continue
				}
				matched := false
				allowed := false
				var uncertain, failed *CheckIssue
				for _, rule := range policy.data.Rules {
					if !checkRuleMatches(rule, snapshot.ID, f.ID, e) {
						continue
					}
					matched = true
					if rule.Effect == "deny" {
						issue = &CheckIssue{Technology: f.ID, RuleID: rule.ID, Code: "denied", Message: "Detected use is denied by policy.", Evidence: []Evidence{e}}
						break
					}
					okay, unknown := checkVersion(rule, f, e)
					if unknown {
						uncertain = &CheckIssue{Technology: f.ID, RuleID: rule.ID, Code: "version-unknown", Message: "A concrete SemVer pin could not be associated with this use.", Evidence: []Evidence{e}}
						continue
					}
					if !okay {
						failed = &CheckIssue{Technology: f.ID, RuleID: rule.ID, Code: "version-not-permitted", Message: "Observed pin does not satisfy " + rule.Versions + ".", Evidence: []Evidence{e}}
						continue
					}
					if rule.Effect == "existing-only" {
						if options.Baseline == nil || (hasOld && !comparable) {
							uncertain = &CheckIssue{Technology: f.ID, RuleID: rule.ID, Code: "baseline-required", Message: "Existing-only permission requires a comparable baseline.", Evidence: []Evidence{e}}
							continue
						}
						if !hasOld || !uses[checkUse(f.ID, e)] {
							failed = &CheckIssue{Technology: f.ID, RuleID: rule.ID, Code: "new-use", Message: "Existing-only permission does not allow a new observation location.", Evidence: []Evidence{e}}
							continue
						}
					}
					allowed = true
				}
				if issue != nil {
					checked.Violations = append(checked.Violations, *issue)
				} else if allowed {
					continue
				} else if uncertain != nil {
					checked.Gaps = append(checked.Gaps, *uncertain)
				} else if failed != nil {
					checked.Violations = append(checked.Violations, *failed)
				} else if matched || policy.data.Default == "deny" {
					checked.Violations = append(checked.Violations, CheckIssue{Technology: f.ID, Code: "not-permitted", Message: "No applicable permission allows this observed use.", Evidence: []Evidence{e}})
				}
			}
		}
		checked.Exceptions = uniqueSorted(checked.Exceptions)
		checked.Violations = mergeCheckIssues(checked.Violations)
		checked.Gaps = mergeCheckIssues(checked.Gaps)
		if len(checked.Gaps) > 0 {
			r.Complete = false
		}
		r.Repositories = append(r.Repositories, checked)
	}
	for id := range prior {
		if !seen[id] {
			r.RemovedRepositories = append(r.RemovedRepositories, id)
		}
	}
	sort.Strings(r.RemovedRepositories)
	return r, nil
}

func checkRuleMatches(rule CheckRule, repository, id string, e Evidence) bool {
	packageID, _ := dependencyEvidence(e)
	if !contains(rule.Technologies, id) && !contains(rule.Technologies, canonicalFindingID(id)) && (packageID == "" || !contains(rule.Technologies, packageID)) {
		return false
	}
	if len(rule.Repositories) > 0 && !contains(rule.Repositories, repository) {
		return false
	}
	if len(rule.Paths) == 0 {
		return true
	}
	for _, prefix := range rule.Paths {
		if prefix == "." || e.Path == prefix || strings.HasPrefix(e.Path, prefix+"/") {
			return true
		}
	}
	return false
}

type observationKey struct{ technology, packageID, path, kind string }

func checkUse(id string, e Evidence) observationKey {
	packageID, _ := dependencyEvidence(e)
	return observationKey{canonicalFindingID(id), packageID, e.Path, e.Kind}
}

func baselineUses(inv Inventory) map[observationKey]bool {
	uses := map[observationKey]bool{}
	for _, f := range inv.Findings {
		if f.Presence == "detected" {
			for _, e := range f.Evidence {
				uses[checkUse(f.ID, e)] = true
			}
		}
	}
	return uses
}

func checkDrift(current, prior Inventory, allowRemovals bool) []CheckDrift {
	a, b := map[string]Finding{}, map[string]Finding{}
	for _, f := range current.Findings {
		if f.Presence == "detected" {
			a[f.ID] = f
		}
	}
	for _, f := range prior.Findings {
		if f.Presence == "detected" {
			b[f.ID] = f
		}
	}
	result := []CheckDrift{}
	for _, id := range sortedKeys(a) {
		f := a[id]
		old, exists := b[id]
		if !exists {
			result = append(result, CheckDrift{Technology: id, Kind: "added"})
			continue
		}
		before, after := checkVersions(old), checkVersions(f)
		if !reflect.DeepEqual(before, after) {
			result = append(result, CheckDrift{Technology: id, Kind: "version_changed", Before: before, After: after})
		} else if f.EvidenceFingerprint != old.EvidenceFingerprint {
			result = append(result, CheckDrift{Technology: id, Kind: "evidence_changed"})
		}
	}
	for _, id := range sortedKeys(b) {
		if _, exists := a[id]; !exists && allowRemovals {
			result = append(result, CheckDrift{Technology: id, Kind: "removed"})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Technology < result[j].Technology })
	return result
}

func checkVersions(f Finding) []string {
	var values []string
	for _, e := range f.Evidence {
		id, version := dependencyEvidence(e)
		if id != "" {
			values = append(values, id+" "+version)
		}
	}
	return uniqueSorted(values)
}

func mergeCheckIssues(issues []CheckIssue) []CheckIssue {
	merged := map[string]*CheckIssue{}
	for _, issue := range issues {
		key := issue.Technology + "\x00" + issue.RuleID + "\x00" + issue.Code + "\x00" + issue.Message
		if existing := merged[key]; existing != nil {
			existing.Evidence = append(existing.Evidence, issue.Evidence...)
		} else {
			copy := issue
			merged[key] = &copy
		}
	}
	result := []CheckIssue{}
	for _, key := range sortedKeys(merged) {
		issue := *merged[key]
		sort.Slice(issue.Evidence, func(i, j int) bool {
			a, b := issue.Evidence[i], issue.Evidence[j]
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
			return a.Detail < b.Detail
		})
		result = append(result, issue)
	}
	return result
}

func completeCapture(s RepositorySnapshot) bool {
	return len(s.Inventory.Warnings) == 0 && (s.Status == "empty" || s.Status == "captured" && s.Inventory.Source.Files > 0)
}
