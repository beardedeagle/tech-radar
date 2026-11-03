package radar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"tech-radar/internal/jsonutil"
)

// PortfolioVersion identifies the capture format and profile rules.
const PortfolioVersion = "portfolio-3"

const legacyPortfolioVersion = "portfolio-1"

var regexpCommit = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// FileHash records a source file's relative path and SHA-256.
type FileHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// RepositorySnapshot saves a repo's findings, revision and file hashes.
// It excludes absolute local paths and raw source/configuration values.
type RepositorySnapshot struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Origin    string     `json:"origin"`
	Revision  string     `json:"revision,omitempty"`
	Archived  bool       `json:"archived"`
	Fork      bool       `json:"fork"`
	Status    string     `json:"status"`
	Inventory Inventory  `json:"inventory"`
	Files     []FileHash `json:"files"`
}

// ContextProfile lists review topics for a language and application type.
type ContextProfile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Ecosystem    string   `json:"ecosystem"`
	Role         string   `json:"role"`
	Concerns     []string `json:"concerns"`
	Boundaries   []string `json:"boundaries"`
	Repositories []string `json:"repositories"`
}

type contextTechnology struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Quadrant      string   `json:"quadrant,omitempty"`
	Significance  string   `json:"significance"`
	Confidence    string   `json:"confidence"`
	Repositories  []string `json:"repositories"`
	Profiles      []string `json:"profiles,omitempty"`
	EvidenceCount int      `json:"evidence_count"`
}

type contextPack struct {
	Kind             string               `json:"kind"`
	SchemaVersion    int                  `json:"schema_version"`
	GeneratorVersion string               `json:"generator_version"`
	CatalogVersion   string               `json:"catalog_version"`
	Snapshots        []RepositorySnapshot `json:"snapshots"`
	Technologies     []contextTechnology  `json:"technologies"`
	Profiles         []ContextProfile     `json:"profiles"`
	Limitations      []string             `json:"limitations"`
	Fingerprint      string               `json:"fingerprint"`
}

// Context holds saved repo findings and their shared checklist.
// BuildContext and ReadContext validate it. The zero value has no context.
type Context struct {
	pack        contextPack
	fingerprint string
	sourceHash  [32]byte
	catalog     []Technology
	profiles    map[string]bool
}

// MaxContextBytes is the size limit for reading and writing context files.
const MaxContextBytes = 128 << 20

// BuildContext builds a repeatable checklist from saved repo findings.
// It rejects duplicate repos, invalid source metadata and embedded decisions.
// Importance comes from each technology's role, not how many repos use it.
func BuildContext(snapshots []RepositorySnapshot) (Context, error) {
	return buildContext(snapshots, PortfolioVersion, Version)
}

func buildContext(snapshots []RepositorySnapshot, version, generatorVersion string) (Context, error) {
	if len(snapshots) == 0 {
		return Context{}, fmt.Errorf("context requires at least one repository")
	}
	// Copy before sorting so we don't change the caller's snapshots.
	b, err := json.Marshal(snapshots)
	if err != nil {
		return Context{}, err
	}
	if len(b) > MaxContextBytes {
		return Context{}, fmt.Errorf("context exceeds 128 MiB")
	}
	b = nil
	copies := copySnapshots(snapshots)
	sort.Slice(copies, func(i, j int) bool { return copies[i].ID < copies[j].ID })
	p := contextPack{Kind: "tech-radar-context", SchemaVersion: 1, GeneratorVersion: generatorVersion, CatalogVersion: version, Snapshots: copies, Technologies: []contextTechnology{}, Profiles: []ContextProfile{}, Limitations: []string{
		"Static bounded capture only; repository code, builds and hooks are never executed. Membership reflects accessible repositories at collection, not an atomic organization snapshot.",
		"Dependency/generated trees, tests, credentials, symlinks, binary content and eligible files larger than 2 MiB are excluded; repository warnings record additional gaps.",
		"Literal manifest readers do not evaluate dynamic syntax or resolve dependency includes. Unknown observations remain unclassified; absence is not proof of non-use.",
		"Profiles are provisional native checklists, not implementation, deployment, ownership or approval proof. Repository prevalence never sets significance or radar rings.",
		"Hashes bind frozen normalized observations for reproducible compilation; they do not attest source authenticity. Keep packs private when repository metadata is private.",
	}}
	byID := map[string]*contextTechnology{}
	custom := map[string]Technology{}
	profileRepos := map[string]map[string]bool{}
	known := map[string]Technology{}
	for _, t := range Catalog() {
		known[t.ID] = t
	}
	for i := range p.Snapshots {
		s := &p.Snapshots[i]
		if i > 0 && s.ID == p.Snapshots[i-1].ID {
			return Context{}, fmt.Errorf("duplicate repository identity %q", s.ID)
		}
		if err := normalizeSnapshot(s, known); err != nil {
			return Context{}, fmt.Errorf("%s: %w", s.ID, err)
		}
		for _, c := range s.Inventory.Components {
			if profileRepos[c.Profile] == nil {
				profileRepos[c.Profile] = map[string]bool{}
			}
			profileRepos[c.Profile][s.ID] = true
		}
		for _, f := range s.Inventory.Findings {
			if t, ok := known[f.ID]; ok && (t.Name != f.Name || t.Quadrant != f.Quadrant) {
				return Context{}, fmt.Errorf("%s conflicts with maintained catalog", f.ID)
			}
			a := assessPortfolioVersion(f, s.Inventory.Components, version)
			t := byID[f.ID]
			if t == nil {
				t = &contextTechnology{ID: f.ID, Name: f.Name, Quadrant: f.Quadrant, Significance: "unknown", Confidence: "unknown", Repositories: []string{}}
				byID[f.ID] = t
			}
			if t.Name != f.Name || t.Quadrant != f.Quadrant {
				return Context{}, fmt.Errorf("conflicting observed metadata for %s; use consistent detection rules", f.ID)
			}
			t.Repositories = append(t.Repositories, s.ID)
			t.EvidenceCount += f.EvidenceCount
			if significanceRank(a.Significance) > significanceRank(t.Significance) {
				t.Significance = a.Significance
			}
			if confidenceRank(a.Confidence) > confidenceRank(t.Confidence) {
				t.Confidence = a.Confidence
			}
			t.Profiles = uniqueSorted(append(t.Profiles, a.Profiles...))
			if _, ok := known[f.ID]; !ok && f.Quadrant != "" {
				custom[f.ID] = Technology{ID: f.ID, Name: f.Name, Quadrant: f.Quadrant}
			}
		}
	}
	for _, id := range sortedKeys(byID) {
		p.Technologies = append(p.Technologies, *byID[id])
	}
	profiles := map[string]bool{}
	for _, template := range profileCatalog() {
		if repos := profileRepos[template.ID]; len(repos) > 0 {
			template.Repositories = sortedKeys(repos)
			p.Profiles = append(p.Profiles, template)
			profiles[template.ID] = true
		}
	}
	sort.Slice(p.Profiles, func(i, j int) bool { return p.Profiles[i].ID < p.Profiles[j].ID })
	b, err = json.Marshal(p)
	if err != nil {
		return Context{}, err
	}
	digest := sha256.Sum256(b)
	p.Fingerprint = hex.EncodeToString(digest[:])
	c := Context{pack: p, fingerprint: p.Fingerprint, profiles: profiles}
	for _, id := range sortedKeys(custom) {
		c.catalog = append(c.catalog, custom[id])
	}
	if _, err := c.Bytes(); err != nil {
		return Context{}, err
	}
	return c, nil
}

// Bytes returns stable JSON, subject to MaxContextBytes.
func (c Context) Bytes() ([]byte, error) {
	if c.fingerprint == "" {
		return nil, fmt.Errorf("context is not validated")
	}
	b, err := json.MarshalIndent(c.pack, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b)+1 > MaxContextBytes {
		return nil, fmt.Errorf("context exceeds 128 MiB")
	}
	return append(b, '\n'), nil
}

// ReadContext checks a saved context against a rebuild from its findings.
// Supported older generator metadata is retained when rebuilding its fingerprint.
// Edited checklists, unsupported versions and embedded decisions return an error.
func ReadContext(data []byte) (Context, error) {
	if len(data) > MaxContextBytes {
		return Context{}, fmt.Errorf("context exceeds 128 MiB")
	}
	var p contextPack
	if err := jsonutil.DecodeStrict(data, &p); err != nil {
		return Context{}, fmt.Errorf("context: %w", err)
	}
	knownGenerator := p.GeneratorVersion == Version || p.GeneratorVersion == "0.2.1" || p.GeneratorVersion == "0.2.0"
	if p.Kind != "tech-radar-context" || p.SchemaVersion != 1 || !supportedPortfolioVersion(p.CatalogVersion) || !knownGenerator {
		return Context{}, fmt.Errorf("unsupported context kind/schema/catalog/generator version")
	}
	c, err := buildContext(p.Snapshots, p.CatalogVersion, p.GeneratorVersion)
	if err != nil {
		return Context{}, err
	}
	expected, _ := json.Marshal(c.pack)
	actual, _ := json.Marshal(p)
	if !bytes.Equal(expected, actual) {
		return Context{}, fmt.Errorf("saved context doesn't match its rebuilt checklist; keep the file and collect again or rebuild with the original tool version")
	}
	c.sourceHash = sha256.Sum256(data)
	return c, nil
}

func supportedPortfolioVersion(version string) bool {
	return version == PortfolioVersion || version == "portfolio-2" || version == legacyPortfolioVersion
}

func validDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && value == strings.ToLower(value)
}

func uniqueSorted(values []string) []string {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return sortedKeys(m)
}

func significanceRank(value string) int {
	return map[string]int{"unknown": 0, "implementation_support": 1, "workflow_enabling": 2, "system_defining": 3}[value]
}

func normalizeSnapshot(s *RepositorySnapshot, known map[string]Technology) error {
	if s.Name == "" || strings.ContainsAny(s.Name, "\\\x00\r\n") || strings.HasPrefix(s.Name, "/") {
		return fmt.Errorf("invalid repository name")
	}
	if s.Origin != "local" && s.Origin != "github" {
		return fmt.Errorf("invalid repository origin")
	}
	if s.Origin == "local" {
		if strings.Contains(s.Name, "/") || s.Name == "." || s.Name == ".." || s.ID != "local:"+s.Name {
			return fmt.Errorf("invalid local identity; supply a portable display name")
		}
	} else if len(strings.Split(s.Name, "/")) != 2 || !validID.MatchString(s.Name) || s.ID != "github:"+strings.ToLower(s.Name) {
		return fmt.Errorf("invalid GitHub identity")
	}
	if s.Status != "captured" && s.Status != "empty" {
		return fmt.Errorf("invalid capture status")
	}
	inv := &s.Inventory
	if !supportedPortfolioVersion(inv.Source.CorpusVersion) || inv.Source.ContextFingerprint != "" || inv.Source.GroupingCatalog != "" || inv.Source.GroupingFingerprint != "" {
		return fmt.Errorf("unsupported capture corpus or nested context")
	}
	if s.Revision != inv.Source.GitHead {
		return fmt.Errorf("revision does not match captured head")
	}
	if s.Revision != "" && !regexpCommit.MatchString(s.Revision) {
		return fmt.Errorf("invalid revision")
	}
	if s.Origin == "github" && s.Status == "captured" && s.Revision == "" {
		return fmt.Errorf("remote capture requires pinned revision")
	}
	if inv.Source.RulesFingerprint != "" && !validDigest(inv.Source.RulesFingerprint) {
		return fmt.Errorf("invalid rules fingerprint")
	}
	if s.Files == nil {
		s.Files = []FileHash{}
	}
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	paths := map[string]bool{}
	h := sha256.New()
	for _, f := range s.Files {
		if !PortfolioPathEligible(f.Path) || !validDigest(f.SHA256) || paths[f.Path] {
			return fmt.Errorf("invalid or duplicate source hash path %q", f.Path)
		}
		paths[f.Path] = true
		fmt.Fprintf(h, "%s\x00%s\n", f.Path, f.SHA256)
	}
	if inv.Source.Files != len(s.Files) || inv.Source.Fingerprint != hex.EncodeToString(h.Sum(nil)) {
		return fmt.Errorf("source corpus fingerprint/count mismatch")
	}
	if s.Status == "empty" && (len(s.Files) > 0 || len(inv.Findings) > 0 || s.Revision != "") {
		return fmt.Errorf("empty repository contains captured facts")
	}
	ids := map[string]bool{}
	for i := range inv.Findings {
		f := &inv.Findings[i]
		if !validID.MatchString(f.ID) || ids[f.ID] || f.Name == "" || (f.Quadrant != "" && !contains(Quadrants, f.Quadrant)) {
			return fmt.Errorf("invalid or duplicate finding %q", f.ID)
		}
		ids[f.ID] = true
		if f.Decision != (Decision{}) || f.Proposal != nil || f.Assessment != nil || f.PlacementChange != nil || f.FamilyID != "" || f.GroupComponent != "" || len(f.Members) > 0 || f.Number != 0 || f.EvidenceChanged || !f.ReviewRequired || f.Presence != "detected" {
			return fmt.Errorf("capture must contain observations without decisions or derived assessments")
		}
		seen := map[Evidence]bool{}
		for _, e := range f.Evidence {
			if !paths[e.Path] || e.Line < 1 || kindOrder(e.Kind) > 5 || seen[e] {
				return fmt.Errorf("invalid or unbound evidence for %s", f.ID)
			}
			seen[e] = true
		}
		b, _ := json.Marshal(f.Evidence)
		digest := sha256.Sum256(b)
		if f.EvidenceCount == 0 || f.EvidenceCount != len(f.Evidence) || f.EvidenceFingerprint != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("incomplete evidence for %s", f.ID)
		}
	}
	sort.Slice(inv.Findings, func(i, j int) bool { return inv.Findings[i].ID < inv.Findings[j].ID })
	inv.Warnings = uniqueSorted(inv.Warnings)
	if inv.Findings == nil {
		inv.Findings = []Finding{}
	}
	if inv.Languages == nil {
		inv.Languages = []LanguageStat{}
	}
	sort.Slice(inv.Languages, func(i, j int) bool { return inv.Languages[i].Language < inv.Languages[j].Language })
	components := portfolioComponents(inv.Findings, nil)
	if !reflect.DeepEqual(inv.Components, components) {
		return fmt.Errorf("component profiles differ from captured evidence")
	}
	// Check saved definitions against the built-in catalog or observed custom IDs.
	// Saved metadata can't add package aliases.
	definitions := map[string]Technology{}
	for _, t := range inv.Technologies {
		if expected, ok := known[t.ID]; ok {
			if !reflect.DeepEqual(t, expected) {
				return fmt.Errorf("altered catalog definition %s", t.ID)
			}
		} else if !ids[t.ID] {
			continue
		} else if !validID.MatchString(t.ID) || t.Name == "" || !contains(Quadrants, t.Quadrant) || len(t.Hex) > 0 || len(t.NPM) > 0 {
			return fmt.Errorf("invalid custom definition %s", t.ID)
		}
		if old, ok := definitions[t.ID]; ok && !reflect.DeepEqual(old, t) {
			return fmt.Errorf("conflicting catalog definition %s", t.ID)
		}
		definitions[t.ID] = t
	}
	inv.Technologies = nil
	for _, id := range sortedKeys(definitions) {
		inv.Technologies = append(inv.Technologies, definitions[id])
	}
	return nil
}

func copySnapshots(snapshots []RepositorySnapshot) []RepositorySnapshot {
	copies := append([]RepositorySnapshot(nil), snapshots...)
	for i := range copies {
		s := &copies[i]
		s.Files = append([]FileHash(nil), s.Files...)
		inv := &s.Inventory
		inv.Warnings = append([]string(nil), inv.Warnings...)
		inv.Languages = append([]LanguageStat(nil), inv.Languages...)
		inv.Components = append([]Component(nil), inv.Components...)
		inv.Findings = append([]Finding(nil), inv.Findings...)
		for j := range inv.Findings {
			f := &inv.Findings[j]
			f.Evidence = append([]Evidence(nil), f.Evidence...)
			f.Members = append([]string(nil), f.Members...)
		}
		inv.Technologies = append([]Technology(nil), inv.Technologies...)
		for j := range inv.Technologies {
			t := &inv.Technologies[j]
			t.Hex = append([]string(nil), t.Hex...)
			t.NPM = append([]string(nil), t.NPM...)
		}
	}
	return copies
}

// Repositories returns an independent copy of the validated frozen observations.
// It does not refresh repository contents or organization membership.
func (c Context) Repositories() []RepositorySnapshot { return copySnapshots(c.pack.Snapshots) }
