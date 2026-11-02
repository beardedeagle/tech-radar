package radar

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"tech-radar/internal/jsonutil"
)

//go:embed data/grouping.json
var groupingData []byte

type groupingFamily struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Quadrant     string   `json:"quadrant,omitempty"`
	Significance string   `json:"significance,omitempty"`
	Relationship string   `json:"relationship,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	Members      []string `json:"members,omitempty"`
	Prefixes     []string `json:"prefixes,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
}

type groupingConfig struct {
	SchemaVersion  int              `json:"schema_version"`
	CatalogVersion string           `json:"catalog_version,omitempty"`
	Families       []groupingFamily `json:"families"`
}

// GroupingRules holds validated family definitions. Its zero value uses the
// built-in catalog. Grouping changes presentation and never grants approval.
type GroupingRules struct {
	families      []groupingFamily
	fingerprint   string
	catalogDigest string
	sourceHash    [32]byte
}

var defaultGrouping = sync.OnceValues(func() (GroupingRules, error) {
	var config groupingConfig
	if err := jsonutil.DecodeStrict(groupingData, &config); err != nil {
		return GroupingRules{}, fmt.Errorf("built-in grouping catalog: %w", err)
	}
	if config.CatalogVersion == "" {
		return GroupingRules{}, fmt.Errorf("built-in grouping catalog has no version")
	}
	if err := validateGrouping(&config, false); err != nil {
		return GroupingRules{}, err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return GroupingRules{}, err
	}
	digest := sha256.Sum256(data)
	return GroupingRules{families: config.Families, catalogDigest: hex.EncodeToString(digest[:])}, nil
})

// ReadGroupingRules validates schema-1 JSON and overlays families by ID on the
// built-in catalog. A disabled family removes that grouping. Unknown fields,
// overlapping memberships, invalid identifiers and oversized inputs fail.
// Families carry no executable code, radar rings or approval fields.
func ReadGroupingRules(data []byte) (GroupingRules, error) {
	if len(data) > MaxFileBytes {
		return GroupingRules{}, fmt.Errorf("grouping input exceeds 2 MiB")
	}
	var config groupingConfig
	if err := jsonutil.DecodeStrict(data, &config); err != nil {
		return GroupingRules{}, fmt.Errorf("grouping: %w", err)
	}
	if config.CatalogVersion != "" {
		return GroupingRules{}, fmt.Errorf("grouping: catalog_version belongs to the built-in catalog")
	}
	if err := validateGrouping(&config, true); err != nil {
		return GroupingRules{}, err
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return GroupingRules{}, err
	}
	digest := sha256.Sum256(canonical)
	base, err := defaultGrouping()
	if err != nil {
		return GroupingRules{}, err
	}
	byID := map[string]groupingFamily{}
	for _, family := range base.families {
		family.Members = slices.Clone(family.Members)
		family.Prefixes = slices.Clone(family.Prefixes)
		byID[family.ID] = family
	}
	for _, family := range config.Families {
		if family.Disabled {
			if _, exists := byID[family.ID]; !exists {
				return GroupingRules{}, fmt.Errorf("grouping: cannot disable unknown family %s", family.ID)
			}
			delete(byID, family.ID)
		} else {
			byID[family.ID] = family
		}
	}
	effective := groupingConfig{SchemaVersion: 1, CatalogVersion: "runtime", Families: []groupingFamily{}}
	for _, id := range sortedKeys(byID) {
		effective.Families = append(effective.Families, byID[id])
	}
	if err := validateGrouping(&effective, false); err != nil {
		return GroupingRules{}, err
	}
	encoded, err := json.Marshal(effective)
	if err != nil {
		return GroupingRules{}, err
	}
	catalog := sha256.Sum256(encoded)
	return GroupingRules{families: effective.Families, fingerprint: hex.EncodeToString(digest[:]), catalogDigest: hex.EncodeToString(catalog[:]), sourceHash: sha256.Sum256(data)}, nil
}

func validateGrouping(config *groupingConfig, overrides bool) error {
	if config.SchemaVersion != 1 || config.Families == nil || len(config.Families) > 256 {
		return fmt.Errorf("grouping: schema_version 1 and a families array of at most 256 entries are required")
	}
	ids, members := map[string]bool{}, map[string]string{}
	type prefixOwner struct{ prefix, owner string }
	var prefixes []prefixOwner
	for i := range config.Families {
		f := &config.Families[i]
		if !validID.MatchString(f.ID) || len(f.ID) > 128 || ids[f.ID] {
			return fmt.Errorf("grouping: invalid or duplicate family ID %q", f.ID)
		}
		ids[f.ID] = true
		if f.Disabled {
			if !overrides || f.Name != "" || f.Quadrant != "" || f.Significance != "" || f.Relationship != "" || f.Scope != "" || len(f.Members)+len(f.Prefixes) != 0 {
				return fmt.Errorf("grouping %s: disabled entries may contain only id and disabled", f.ID)
			}
			continue
		}
		if strings.TrimSpace(f.Name) == "" || len(f.Name) > 120 || !contains(Quadrants, f.Quadrant) || !contains([]string{"system_defining", "workflow_enabling", "implementation_support"}, f.Significance) || !contains([]string{"framework_extensions", "runtime_companions", "platform_services", "workflow_tools", "sdk_modules"}, f.Relationship) || !contains([]string{"component", "repository"}, f.Scope) {
			return fmt.Errorf("grouping %s: name, quadrant, significance, relationship and scope are required", f.ID)
		}
		if len(f.Members)+len(f.Prefixes) == 0 || len(f.Members) > 512 || len(f.Prefixes) > 128 {
			return fmt.Errorf("grouping %s: supply members or prefixes within the catalog limits", f.ID)
		}
		seen := map[string]bool{}
		for i, member := range f.Members {
			member = canonicalFindingID(member)
			f.Members[i] = member
			if !validID.MatchString(member) || len(member) > 256 || seen[member] || (members[member] != "" && members[member] != f.ID) {
				return fmt.Errorf("grouping %s: invalid, duplicate or overlapping member %q", f.ID, member)
			}
			seen[member], members[member] = true, f.ID
		}
		seen = map[string]bool{}
		for _, prefix := range f.Prefixes {
			colon := strings.IndexByte(prefix, ':')
			if !validID.MatchString(prefix) || len(prefix) > 256 || colon < 1 || colon == len(prefix)-1 || seen[prefix] {
				return fmt.Errorf("grouping %s: invalid or duplicate package prefix %q", f.ID, prefix)
			}
			seen[prefix] = true
			prefixes = append(prefixes, prefixOwner{prefix, f.ID})
		}
		sort.Strings(f.Prefixes)
	}
	if len(members) > 8192 || len(prefixes) > 2048 {
		return fmt.Errorf("grouping: catalog exceeds 8192 members or 2048 package prefixes")
	}
	sort.Slice(prefixes, func(i, j int) bool { return prefixes[i].prefix < prefixes[j].prefix })
	var parents []prefixOwner
	for _, a := range prefixes {
		for len(parents) > 0 && !strings.HasPrefix(a.prefix, parents[len(parents)-1].prefix) {
			parents = parents[:len(parents)-1]
		}
		if len(parents) > 0 && parents[len(parents)-1].owner != a.owner {
			return fmt.Errorf("grouping: overlapping prefixes in %s and %s", a.owner, parents[len(parents)-1].owner)
		}
		parents = append(parents, a)
		for member, owner := range members {
			if owner != a.owner && strings.HasPrefix(member, a.prefix) {
				return fmt.Errorf("grouping: member %s overlaps family %s", member, a.owner)
			}
		}
	}
	sort.Slice(config.Families, func(i, j int) bool { return config.Families[i].ID < config.Families[j].ID })
	return nil
}

func matchesFamily(id string, family groupingFamily) bool {
	id = canonicalFindingID(id)
	if contains(family.Members, id) {
		return true
	}
	for _, prefix := range family.Prefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// componentOwners uses manifest boundaries without treating a config directory
// as a separately deployed application. Ambiguous shared findings stay separate.
func componentOwners(findings map[string]Finding) map[string]string {
	dirs := map[string]bool{".": true}
	for _, f := range findings {
		for _, e := range f.Evidence {
			if e.Kind == "declaration" && componentManifest(e.Path) {
				dirs[path.Dir(e.Path)] = true
			}
		}
	}
	owners := map[string]string{}
	for id, f := range findings {
		paths := map[string]bool{}
		runtimeDirs := map[string]bool{}
		for _, e := range f.Evidence {
			if e.Kind == "declaration" && !restrictedDeclaration(e) {
				runtimeDirs[path.Dir(e.Path)] = true
			}
		}
		for _, e := range f.Evidence {
			if len(runtimeDirs) > 0 && (restrictedDeclaration(e) || (e.Kind == "lock resolution" && !runtimeDirs[path.Dir(e.Path)])) {
				continue
			}
			nearest := "."
			for dir := range dirs {
				if dir != "." && (e.Path == dir || strings.HasPrefix(e.Path, dir+"/")) && (nearest == "." || len(dir) > len(nearest)) {
					nearest = dir
				}
			}
			paths[nearest] = true
		}
		if len(paths) == 1 {
			owners[id] = sortedKeys(paths)[0]
		}
	}
	return owners
}

func componentManifest(name string) bool {
	switch path.Base(name) {
	case "package.json", "mix.exs", "go.mod", "pyproject.toml", "requirements.txt", "requirements-dev.txt", "Gemfile", "composer.json", "Cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts":
		return true
	}
	return contains([]string{".csproj", ".fsproj", ".vbproj"}, path.Ext(name))
}

func assignFamilies(findings map[string]Finding, grouping GroupingRules) (map[string]string, map[string]groupingFamily, map[string]string) {
	roots, families, scopes := map[string]string{}, map[string]groupingFamily{}, map[string]string{}
	owners := componentOwners(findings)
	for _, family := range grouping.families {
		buckets := map[string]map[string]bool{}
		for id, f := range findings {
			if f.Presence != "detected" || !matchesFamily(id, family) {
				continue
			}
			scope := "."
			if family.Scope == "component" {
				scope = owners[id]
				if scope == "" {
					continue
				}
			}
			if buckets[scope] == nil {
				buckets[scope] = map[string]bool{}
			}
			buckets[scope][id] = true
		}
		for scope, candidates := range buckets {
			root := ""
			for _, preferred := range family.Members {
				for _, id := range sortedKeys(candidates) {
					if id == preferred || canonicalFindingID(id) == preferred {
						root = id
						break
					}
				}
				if root != "" {
					break
				}
			}
			if root == "" {
				root = sortedKeys(candidates)[0]
			}
			for id := range candidates {
				roots[id], families[id], scopes[id] = root, family, scope
			}
		}
	}
	return roots, families, scopes
}
