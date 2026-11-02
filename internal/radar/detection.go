package radar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"tech-radar/internal/jsonutil"
)

// DetectionRules holds compiled custom patterns. The zero value adds no rules.
// Use ReadDetectionRules to validate input before scanning.
type DetectionRules struct {
	patterns     []sourcePattern
	technologies []Technology
	fingerprint  string
	sourceHash   [32]byte
}

type detectionPattern struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Quadrant      string `json:"quadrant,omitempty"`
	Pattern       string `json:"pattern"`
	Detail        string `json:"detail"`
	Documentation bool   `json:"documentation,omitempty"`
}

// ReadDetectionRules validates schema-1 JSON and compiles its Go regular expressions.
// New IDs need a name and quadrant. Known IDs keep their catalog definitions.
// Invalid, conflicting, empty-matching or oversized patterns return an error.
func ReadDetectionRules(data []byte) (DetectionRules, error) {
	var config struct {
		SchemaVersion int                `json:"schema_version"`
		Patterns      []detectionPattern `json:"patterns"`
	}
	fail := func(err error) (DetectionRules, error) { return DetectionRules{}, fmt.Errorf("rules: %w", err) }
	if err := jsonutil.DecodeStrict(data, &config); err != nil {
		return fail(err)
	}
	if config.SchemaVersion != 1 {
		return fail(fmt.Errorf("unsupported schema_version %d (want 1)", config.SchemaVersion))
	}
	if config.Patterns == nil {
		return fail(fmt.Errorf("patterns must be an array; use [] for no added recognizers"))
	}
	if len(config.Patterns) > 256 {
		return fail(fmt.Errorf("at most 256 patterns are allowed"))
	}
	known := map[string]Technology{}
	for _, t := range Catalog() {
		known[t.ID] = t
	}
	custom := map[string]Technology{}
	seen := map[string]bool{}
	compiledPatterns := map[string]*regexp.Regexp{}
	for i := range config.Patterns {
		r := &config.Patterns[i]
		if !validID.MatchString(r.ID) || len(r.ID) > 128 {
			return fail(fmt.Errorf("invalid technology id"))
		}
		if len(r.Pattern) == 0 || len(r.Pattern) > 4096 || len(r.Detail) > 240 || strings.TrimSpace(r.Detail) == "" {
			return fail(fmt.Errorf("%s: pattern must be 1-4096 bytes and detail 1-240 bytes", r.ID))
		}
		if t, ok := known[r.ID]; ok {
			if (r.Name != "" && r.Name != t.Name) || (r.Quadrant != "" && r.Quadrant != t.Quadrant) {
				return fail(fmt.Errorf("%s: rules cannot override public catalog metadata; use decision policy", r.ID))
			}
			r.Name, r.Quadrant = "", ""
		} else {
			if strings.TrimSpace(r.Name) == "" || len(r.Name) > 120 || !contains(Quadrants, r.Quadrant) {
				return fail(fmt.Errorf("%s: a custom technology needs name (1-120 bytes) and quadrant", r.ID))
			}
			if t, exists := custom[r.ID]; exists && (t.Name != r.Name || t.Quadrant != r.Quadrant) {
				return fail(fmt.Errorf("%s: inconsistent technology metadata", r.ID))
			}
			custom[r.ID] = Technology{ID: r.ID, Name: r.Name, Quadrant: r.Quadrant}
		}
		key := fmt.Sprintf("%s\x00%t\x00%s", r.ID, r.Documentation, r.Pattern)
		if seen[key] {
			return fail(fmt.Errorf("%s: duplicate detection pattern", r.ID))
		}
		seen[key] = true
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fail(fmt.Errorf("%s: invalid pattern: %w", r.ID, err))
		}
		compiledPatterns[r.Pattern] = compiled
		if compiled.MatchString("") {
			return fail(fmt.Errorf("%s: patterns must not match empty lines", r.ID))
		}
	}
	// Hash the effective rules so formatting and input order don't change the result.
	sort.Slice(config.Patterns, func(i, j int) bool {
		a, b := config.Patterns[i], config.Patterns[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Documentation != b.Documentation {
			return !a.Documentation
		}
		if a.Pattern != b.Pattern {
			return a.Pattern < b.Pattern
		}
		return a.Detail < b.Detail
	})
	result := DetectionRules{sourceHash: sha256.Sum256(data)}
	for _, r := range config.Patterns {
		result.patterns = append(result.patterns, sourcePattern{id: r.ID, label: r.Detail, re: compiledPatterns[r.Pattern], doc: r.Documentation, custom: true})
	}
	for _, id := range sortedKeys(custom) {
		result.technologies = append(result.technologies, custom[id])
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return fail(err)
	}
	digest := sha256.Sum256(canonical)
	result.fingerprint = hex.EncodeToString(digest[:])
	return result, nil
}
