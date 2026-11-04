package radar

import (
	"reflect"
	"slices"
)

// comparePlacements uses the previous displayed family placement when present.
// Compact reports without their evidence companion may lack inferred placements;
// that missing history must not be described as an unchanged placement.
func comparePlacements(r *Report, previous *Report, grouping GroupingRules) {
	prior, highlights := map[string]Finding{}, map[string]Finding{}
	families := map[string][]Finding{}
	if previous != nil {
		for _, f := range previous.Findings {
			prior[f.ID] = f
		}
		for _, f := range previous.Highlights {
			highlights[f.ID] = f
			if f.FamilyID != "" {
				key := familyIdentity(f)
				families[key] = append(families[key], f)
			}
		}
	}
	if previous != nil && r.Model.effective() != previous.Model.effective() {
		for i := range r.Findings {
			f := &r.Findings[i]
			f.PlacementChange = nil
			if _, exists := prior[f.ID]; !exists {
				f.PlacementChange = &PlacementChange{Kind: "new"}
			}
		}
		for i := range r.Highlights {
			f := &r.Highlights[i]
			f.PlacementChange = nil
			known := false
			for _, id := range f.Members {
				if _, exists := prior[id]; exists {
					known = true
					break
				}
			}
			if !known {
				f.PlacementChange = &PlacementChange{Kind: "new"}
			}
		}
		return
	}
	replay := previous != nil && samePlacementInputs(*r, *previous, prior, highlights)
	// Split lifecycle members have no family metadata of their own. Component
	// families still require matching evidence ownership, including root components.
	memberComponents := map[string]string{}
	familyScopes := map[string]string{}
	if r.Model == LifecycleModel {
		memberComponents = componentOwners(findingsByID(*r))
		for _, family := range grouping.families {
			familyScopes[family.ID] = family.Scope
		}
	}
	for i := range r.Findings {
		f := &r.Findings[i]
		f.PlacementChange = nil
		if replay {
			f.PlacementChange = copyPlacementChange(prior[f.ID].PlacementChange)
		} else if previous == nil {
			f.PlacementChange = &PlacementChange{Kind: "new"}
		} else {
			old, exists := prior[f.ID]
			f.PlacementChange = placementChange(r.Model, *f, old, exists)
		}
	}
	for i := range r.Highlights {
		f := &r.Highlights[i]
		f.PlacementChange = nil
		if previous == nil {
			f.PlacementChange = &PlacementChange{Kind: "new"}
			continue
		}
		if replay {
			f.PlacementChange = copyPlacementChange(highlights[f.ID].PlacementChange)
		} else if old, exists := highlights[f.ID]; exists && compatibleFamily(*f, old) {
			f.PlacementChange = placementChange(r.Model, *f, old, true)
		} else if family := families[familyIdentity(*f)]; f.FamilyID != "" && len(family) == 1 {
			f.PlacementChange = placementChange(r.Model, *f, family[0], true)
		} else if len(previous.Highlights) > 0 {
			var matches []Finding
			for _, old := range previous.Highlights {
				if !compatibleFamily(*f, old) {
					continue
				}
				if r.Model == LifecycleModel && f.FamilyID == "" && len(f.Members) == 1 &&
					familyScopes[old.FamilyID] != "repository" && old.GroupComponent != "" && memberComponents[f.ID] != old.GroupComponent {
					continue
				}
				for _, member := range f.Members {
					if contains(old.Members, member) {
						matches = append(matches, old)
						break
					}
				}
			}
			if len(matches) == 1 && (len(f.Members) > 1 || r.Model == LifecycleModel) {
				// A family can change representatives or split an existing member.
				f.PlacementChange = placementChange(r.Model, *f, matches[0], true)
			} else if (len(matches) == 0 && len(families[familyIdentity(*f)]) == 0) || (len(f.Members) == 1 && r.Model != LifecycleModel) {
				f.PlacementChange = &PlacementChange{Kind: "new"}
			}
		} else if old, exists := prior[f.ID]; !exists {
			f.PlacementChange = &PlacementChange{Kind: "new"}
		} else if len(previous.Highlights) == 0 && len(f.Members) == 1 {
			f.PlacementChange = placementChange(r.Model, *f, old, true)
		}
	}
}

// Repeating placement inputs keeps the original comparison, including badges.
// Baseline and review-only transitions do not change placement history.
func samePlacementInputs(current, previous Report, prior, highlights map[string]Finding) bool {
	if current.Model.effective() != previous.Model.effective() || current.Source != previous.Source || current.Repository != previous.Repository ||
		current.GeneratorVersion != previous.GeneratorVersion || current.RuleVersion != previous.RuleVersion ||
		len(current.Findings) != len(previous.Findings) || !reflect.DeepEqual(current.Decisions, previous.Decisions) ||
		!reflect.DeepEqual(current.Warnings, previous.Warnings) {
		return false
	}
	for _, f := range current.Findings {
		old, exists := prior[f.ID]
		if !exists || f.Name != old.Name || f.Quadrant != old.Quadrant || f.Presence != old.Presence ||
			f.EvidenceFingerprint != old.EvidenceFingerprint || f.EvidenceCount != old.EvidenceCount ||
			f.EvidenceChanged != old.EvidenceChanged ||
			(old.Proposal != nil && current.Model.ring(f) != current.Model.ring(old)) ||
			(old.Assessment != nil && !reflect.DeepEqual(f.Assessment, old.Assessment)) {
			return false
		}
	}
	if len(highlights) > 0 {
		if len(highlights) != len(current.Highlights) {
			return false
		}
		for _, f := range current.Highlights {
			old, exists := highlights[f.ID]
			if !exists || f.Name != old.Name || f.FamilyID != old.FamilyID || f.GroupComponent != old.GroupComponent || f.Quadrant != old.Quadrant || current.Model.ring(f) != current.Model.ring(old) || !reflect.DeepEqual(f.Members, old.Members) {
				return false
			}
		}
	}
	return true
}

func copyPlacementChange(change *PlacementChange) *PlacementChange {
	if change == nil {
		return nil
	}
	copy := *change
	return &copy
}

func placementChange(model Model, current, previous Finding, exists bool) *PlacementChange {
	if !exists {
		return &PlacementChange{Kind: "new"}
	}
	if model != LifecycleModel && previous.Decision.Ring == "" && previous.Proposal == nil {
		return nil
	}
	before, after := model.ring(previous), model.ring(current)
	change := &PlacementChange{Kind: "unchanged", PreviousRing: before, PreviousQuadrant: previous.Quadrant}
	if before == after && previous.Quadrant == current.Quadrant {
		return change
	}
	change.Kind = "moved"
	from, to := slices.Index(model.rings(), before), slices.Index(model.rings(), after)
	if from >= 0 && to >= 0 {
		if to < from {
			change.Kind = "moved-in"
		} else if to > from {
			change.Kind = "moved-out"
		}
	}
	return change
}

func familyIdentity(f Finding) string { return f.FamilyID + "\x00" + f.GroupComponent }

// Old reports have no family metadata. Their shared members remain the fallback.
func compatibleFamily(a, b Finding) bool {
	return a.FamilyID == "" || b.FamilyID == "" || familyIdentity(a) == familyIdentity(b)
}
