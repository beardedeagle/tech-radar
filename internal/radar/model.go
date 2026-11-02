package radar

const Version = "0.2.1"

// CounterVersion identifies the pinned language-counting engine in reports.
const CounterVersion = "scc/v4 v4.1.0"

// MaxFileBytes is the eligible-file limit for local and archive scans.
const MaxFileBytes = 2 << 20

var Quadrants = []string{"Techniques", "Platforms", "Tools", "Languages & Frameworks"}
var Rings = []string{"Adopt", "Trial", "Assess", "Caution"}

type Evidence struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type Source struct {
	Fingerprint         string `json:"fingerprint"`
	Files               int    `json:"files"`
	GitHead             string `json:"git_head,omitempty"`
	Counter             string `json:"counter,omitempty"`
	RulesFingerprint    string `json:"rules_fingerprint,omitempty"`
	ContextFingerprint  string `json:"context_fingerprint,omitempty"`
	CorpusVersion       string `json:"corpus_version,omitempty"`
	GroupingFingerprint string `json:"grouping_fingerprint,omitempty"`
	GroupingCatalog     string `json:"grouping_catalog,omitempty"`
}

type LanguageStat struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
	Lines    int64  `json:"lines"`
	Code     int64  `json:"code"`
	Comments int64  `json:"comments"`
	Blank    int64  `json:"blank"`
}

type Decision struct {
	Name      string `json:"name,omitempty"`
	Quadrant  string `json:"quadrant,omitempty"`
	Ring      string `json:"ring,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Rationale string `json:"rationale,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Status    string `json:"status,omitempty"`
	Reference string `json:"reference,omitempty"`
	Reviewed  string `json:"reviewed,omitempty"`
	Lifecycle string `json:"lifecycle,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

type Finding struct {
	ID                  string           `json:"id"`
	Number              int              `json:"number"`
	Name                string           `json:"name"`
	Quadrant            string           `json:"quadrant,omitempty"`
	Evidence            []Evidence       `json:"evidence"`
	EvidenceCount       int              `json:"evidence_count"`
	EvidenceFingerprint string           `json:"evidence_fingerprint,omitempty"`
	Presence            string           `json:"presence"`
	EvidenceChanged     bool             `json:"evidence_changed"`
	ReviewRequired      bool             `json:"review_required"`
	Decision            Decision         `json:"decision"`
	Assessment          *Assessment      `json:"assessment,omitempty"`
	Proposal            *Decision        `json:"proposal,omitempty"`
	Members             []string         `json:"members,omitempty"`
	PlacementChange     *PlacementChange `json:"placement_change,omitempty"`
	FamilyID            string           `json:"family_id,omitempty"`
	GroupComponent      string           `json:"group_component,omitempty"`
}

// PlacementChange compares the displayed ring and quadrant with a known prior
// placement. It is separate from changes to the supporting evidence.
type PlacementChange struct {
	Kind             string `json:"kind"`
	PreviousRing     string `json:"previous_ring,omitempty"`
	PreviousQuadrant string `json:"previous_quadrant,omitempty"`
}

// Component describes an application found in the repo's manifests.
// Path is relative to the repo. A profile match doesn't establish deployment.
type Component struct {
	Path      string `json:"path"`
	Ecosystem string `json:"ecosystem"`
	Role      string `json:"role"`
	Profile   string `json:"profile"`
	Reason    string `json:"reason"`
}

// Assessment explains a finding's importance and the evidence supporting it.
type Assessment struct {
	Significance string   `json:"significance"`
	Confidence   string   `json:"confidence"`
	Role         string   `json:"role"`
	Scope        string   `json:"scope"`
	Profiles     []string `json:"profiles,omitempty"`
	Reason       string   `json:"reason"`
}

type Inventory struct {
	Source       Source         `json:"source"`
	Findings     []Finding      `json:"findings"`
	Warnings     []string       `json:"warnings"`
	Languages    []LanguageStat `json:"languages"`
	Components   []Component    `json:"components,omitempty"`
	Technologies []Technology   `json:"technologies,omitempty"`
}

type Policy struct {
	SchemaVersion int                 `json:"schema_version"`
	Repository    string              `json:"repository,omitempty"`
	Rules         map[string]Decision `json:"rules"`
}

type Report struct {
	SchemaVersion    int                 `json:"schema_version"`
	GeneratorVersion string              `json:"generator_version"`
	Repository       string              `json:"repository"`
	Source           Source              `json:"source"`
	Findings         []Finding           `json:"findings"`
	Decisions        map[string]Decision `json:"decisions"`
	Excluded         []string            `json:"excluded"`
	Warnings         []string            `json:"warnings"`
	Languages        []LanguageStat      `json:"languages"`
	Components       []Component         `json:"components,omitempty"`
	RuleVersion      string              `json:"rule_version,omitempty"`
	Highlights       []Finding           `json:"highlights,omitempty"`
	ExcludedFindings []Finding           `json:"excluded_findings,omitempty"`
	EvidenceFile     string              `json:"evidence_file,omitempty"`
	EvidenceSHA256   string              `json:"evidence_sha256,omitempty"`
}
