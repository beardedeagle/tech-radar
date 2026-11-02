package radar

import (
	"reflect"
	"slices"
)

// comparePlacements uses the previous displayed family placement when present.
// Compact reports without their evidence companion may lack inferred placements;
// that missing history must not be described as an unchanged placement.
func comparePlacements(r *Report, previous *Report) {
	prior, highlights := map[string]Finding{}, map[string]Finding{}
	families := map[string]Finding{}
	if previous != nil {
		for _, f := range previous.Findings {
			prior[f.ID] = f
		}
		for _, f := range previous.Highlights {
			highlights[f.ID] = f
			if f.FamilyID != "" {
				families[familyIdentity(f)] = f
			}
		}
	}
	replay := previous != nil && samePlacementInputs(*r, *previous, prior, highlights)
	for i := range r.Findings {
		f := &r.Findings[i]
		f.PlacementChange = nil
		if replay {
			f.PlacementChange = copyPlacementChange(prior[f.ID].PlacementChange)
		} else if previous == nil {
			f.PlacementChange = &PlacementChange{Kind: "new"}
		} else {
			old, exists := prior[f.ID]
			f.PlacementChange = placementChange(*f, old, exists)
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
		} else if old, exists := families[familyIdentity(*f)]; f.FamilyID != "" && exists {
			f.PlacementChange = placementChange(*f, old, true)
		} else if old, exists := highlights[f.ID]; exists && compatibleFamily(*f, old) {
			f.PlacementChange = placementChange(*f, old, true)
		} else if len(previous.Highlights) > 0 {
			var matches []Finding
			for _, old := range previous.Highlights {
				if !compatibleFamily(*f, old) {
					continue
				}
				for _, member := range f.Members {
					if contains(old.Members, member) {
						matches = append(matches, old)
						break
					}
				}
			}
			if len(f.Members) > 1 && len(matches) == 1 {
				// Adding a service can change a family's representative ID.
				f.PlacementChange = placementChange(*f, matches[0], true)
			} else if len(matches) == 0 || len(f.Members) == 1 {
				f.PlacementChange = &PlacementChange{Kind: "new"}
			}
		} else if old, exists := prior[f.ID]; !exists {
			f.PlacementChange = &PlacementChange{Kind: "new"}
		} else if len(previous.Highlights) == 0 && len(f.Members) == 1 {
			f.PlacementChange = placementChange(*f, old, true)
		}
	}
}

// Repeating a report version keeps its original comparison, including badges.
// Otherwise an unchanged regeneration would create a diff or erase movement.
func samePlacementInputs(current, previous Report, prior, highlights map[string]Finding) bool {
	if current.Source != previous.Source || current.Repository != previous.Repository ||
		current.GeneratorVersion != previous.GeneratorVersion || current.RuleVersion != previous.RuleVersion ||
		len(current.Findings) != len(previous.Findings) || !reflect.DeepEqual(current.Decisions, previous.Decisions) ||
		!reflect.DeepEqual(current.Warnings, previous.Warnings) {
		return false
	}
	for _, f := range current.Findings {
		old, exists := prior[f.ID]
		if !exists || f.Name != old.Name || f.Quadrant != old.Quadrant || f.Presence != old.Presence ||
			f.EvidenceFingerprint != old.EvidenceFingerprint || f.EvidenceCount != old.EvidenceCount ||
			f.EvidenceChanged != old.EvidenceChanged || f.ReviewRequired != old.ReviewRequired ||
			(old.Proposal != nil && ring(f) != ring(old)) ||
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
			if !exists || f.Name != old.Name || f.FamilyID != old.FamilyID || f.GroupComponent != old.GroupComponent || f.Quadrant != old.Quadrant || ring(f) != ring(old) || !reflect.DeepEqual(f.Members, old.Members) {
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

func placementChange(current, previous Finding, exists bool) *PlacementChange {
	if !exists {
		return &PlacementChange{Kind: "new"}
	}
	if previous.Decision.Ring == "" && previous.Proposal == nil {
		return nil
	}
	before, after := ring(previous), ring(current)
	change := &PlacementChange{Kind: "unchanged", PreviousRing: before, PreviousQuadrant: previous.Quadrant}
	if before == after && previous.Quadrant == current.Quadrant {
		return change
	}
	change.Kind = "moved"
	from, to := slices.Index(Rings, before), slices.Index(Rings, after)
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
