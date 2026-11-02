package radar

import (
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/Masterminds/semver/v3"
)

func dependencyEvidence(e Evidence) (id, version string) {
	if e.Kind == "tool pin" {
		name, pin, ok := strings.Cut(e.Detail, " pin ")
		if ok {
			return "tool:" + name, pin
		}
	}
	if strings.HasPrefix(e.Detail, "Node engine constraint ") {
		return "tool:nodejs", strings.TrimPrefix(e.Detail, "Node engine constraint ")
	}
	if e.Kind != "declaration" && e.Kind != "lock resolution" {
		return "", ""
	}
	ecosystem, tail, ok := strings.Cut(e.Detail, " package ")
	if !ok {
		return "", ""
	}
	name, value, ok := strings.Cut(tail, ": ")
	if !ok {
		return "", ""
	}
	if cut := strings.LastIndex(value, " ["); cut >= 0 && strings.HasSuffix(value, "]") {
		value = value[:cut]
	}
	return ecosystem + ":" + name, value
}

func checkVersion(rule CheckRule, f Finding, e Evidence) (allowed, unknown bool) {
	if rule.constraint == nil {
		return true, false
	}
	packageID, value := dependencyEvidence(e)
	var candidates []Evidence
	if packageID != "" {
		if _, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v")); err == nil {
			candidates = []Evidence{e}
		} else {
			declared, err := declarationConstraint(packageID, value)
			if err != nil {
				return false, true
			}
			for _, other := range f.Evidence {
				id, _ := dependencyEvidence(other)
				if (other.Kind == "lock resolution" || other.Kind == "tool pin") && id == packageID && path.Dir(other.Path) == path.Dir(e.Path) {
					_, pin := dependencyEvidence(other)
					resolved, err := semver.StrictNewVersion(strings.TrimPrefix(pin, "v"))
					if err != nil || !declared.Check(resolved) {
						return false, true
					}
					candidates = append(candidates, other)
				}
			}
		}
	} else {
		nearest := -1
		for _, other := range f.Evidence {
			id, _ := dependencyEvidence(other)
			dir := path.Dir(other.Path)
			if id == "" || (dir != "." && !strings.HasPrefix(e.Path, dir+"/")) {
				continue
			}
			depth := strings.Count(dir, "/") + 1
			if dir == "." {
				depth = 0
			}
			if depth > nearest {
				nearest = depth
				candidates = nil
			}
			if depth == nearest && (other.Kind == "declaration" || other.Kind == "tool pin") {
				candidates = append(candidates, other)
			}
		}
		if len(candidates) > 0 {
			for _, candidate := range candidates {
				ok, gap := checkVersion(rule, f, candidate)
				if !ok || gap {
					return false, gap
				}
			}
			return true, false
		}
	}
	if len(candidates) == 0 {
		return false, true
	}
	for _, candidate := range candidates {
		_, pin := dependencyEvidence(candidate)
		v, err := semver.StrictNewVersion(strings.TrimPrefix(pin, "v"))
		if err != nil {
			return false, true
		}
		if !rule.constraint.Check(v) {
			return false, false
		}
	}
	return true, false
}

// declarationConstraint checks whether a lock can resolve the declared source.
// npm and Hex use different range semantics. Unrecognized source/range syntax
// cannot borrow permission from an unrelated or stale lock resolution.
func declarationConstraint(id, value string) (*semver.Constraints, error) {
	if strings.HasPrefix(id, "npm:") || id == "tool:nodejs" {
		return semver.NewConstraint(value)
	}
	if !strings.HasPrefix(id, "hex:") {
		return nil, fmt.Errorf("unsupported declaration range")
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "~>") {
		lower := strings.TrimSpace(strings.TrimPrefix(value, "~>"))
		count := len(strings.Split(lower, "."))
		if count == 2 {
			lower += ".0"
		} else if count != 3 {
			return nil, fmt.Errorf("unsupported Hex requirement")
		}
		v, err := semver.StrictNewVersion(lower)
		if err != nil {
			return nil, err
		}
		var upper string
		if count == 2 {
			if v.Major() == math.MaxUint64 {
				return nil, fmt.Errorf("Hex requirement overflow")
			}
			upper = fmt.Sprintf("%d.0.0", v.Major()+1)
		} else {
			if v.Minor() == math.MaxUint64 {
				return nil, fmt.Errorf("Hex requirement overflow")
			}
			upper = fmt.Sprintf("%d.%d.0", v.Major(), v.Minor()+1)
		}
		return semver.NewConstraint(">=" + v.String() + " <" + upper)
	}
	for _, operator := range []string{">=", "<=", "==", "!=", ">", "<", "="} {
		if strings.HasPrefix(value, operator) {
			v, err := semver.StrictNewVersion(strings.TrimSpace(strings.TrimPrefix(value, operator)))
			if err != nil {
				return nil, err
			}
			if operator == "==" {
				operator = "="
			}
			return semver.NewConstraint(operator + v.String())
		}
	}
	return nil, fmt.Errorf("unsupported Hex requirement")
}
