package radar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

var gitHeadPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

const GeneratedMarker = "<!-- tech-radar:generated -->"

var toolVersionIDs = map[string]string{"elixir": "elixir", "erlang": "erlang-otp", "nodejs": "nodejs", "golang": "go", "python": "python", "ruby": "ruby"}

type inputFile struct {
	name string
	data []byte
}
type scanFile struct {
	name   string
	digest [32]byte
}

type scanner struct {
	known    map[string]Technology
	aliases  map[string]string
	found    map[string]*Finding
	declared map[string]map[string]bool
	seen     map[string]map[Evidence]bool
	patterns []sourcePattern
}

// ScanOptions supplies explicit inputs for a repository run. Zero-valued inputs
// use built-in detection and grouping without a shared context.
type ScanOptions struct {
	Rules    DetectionRules
	Context  Context
	Grouping GroupingRules
}

// ScanWithOptions scans without running repository code. Runtime input files are
// excluded by content hash; filesystem and parsing failures return an error.
func ScanWithOptions(root string, options ScanOptions) (Inventory, error) {
	var ignored [][32]byte
	if options.Grouping.fingerprint != "" {
		ignored = append(ignored, options.Grouping.sourceHash)
	}
	inv, _, err := scanRepository(root, options.Rules, false, options.Context, ignored...)
	return inv, err
}

func Scan(root string) (Inventory, error) {
	return ScanWithRules(root, DetectionRules{})
}

// ScanWithRules scans the repo with additional detection patterns.
// The rules file is excluded, even if it's inside the repo.
// Filesystem errors include the path that couldn't be read.
func ScanWithRules(root string, rules DetectionRules) (Inventory, error) {
	inv, _, err := scanRepository(root, rules, false, Context{})
	return inv, err
}

// CaptureRepository collects findings and file hashes for discovery.
// It reads eligible files inside the repo and omits absolute paths and raw values.
// ignored holds SHA-256 digests of explicit runtime inputs to omit from capture.
func CaptureRepository(root string, rules DetectionRules, ignored ...[32]byte) (Inventory, []FileHash, error) {
	return scanRepository(root, rules, true, Context{}, ignored...)
}

// ScanWithContext scans the repo using a validated shared checklist.
func ScanWithContext(root string, rules DetectionRules, pack Context) (Inventory, error) {
	if pack.fingerprint == "" {
		return Inventory{}, fmt.Errorf("context is not validated")
	}
	inv, _, err := scanRepository(root, rules, true, pack)
	return inv, err
}

func scanRepository(root string, rules DetectionRules, extended bool, pack Context, ignored ...[32]byte) (Inventory, []FileHash, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Inventory{}, nil, err
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return Inventory{}, nil, fmt.Errorf("open repository: %w", err)
	}
	defer confined.Close()
	var files []scanFile
	warnings := []string{}
	s := scanner{known: map[string]Technology{}, aliases: map[string]string{}, found: map[string]*Finding{}, declared: map[string]map[string]bool{}, seen: map[string]map[Evidence]bool{}}
	s.patterns = append(append([]sourcePattern(nil), patterns...), rules.patterns...)
	catalog := Catalog()
	for _, t := range catalog {
		s.known[t.ID] = t
		for _, a := range t.Hex {
			s.aliases["hex:"+a] = t.ID
		}
		for _, a := range t.NPM {
			s.aliases["npm:"+a] = t.ID
		}
	}
	for _, t := range pack.catalog {
		if _, exists := s.known[t.ID]; !exists {
			s.known[t.ID] = t
		}
	}
	for _, t := range rules.technologies {
		if known, exists := s.known[t.ID]; exists && (known.Name != t.Name || known.Quadrant != t.Quadrant) {
			return Inventory{}, nil, fmt.Errorf("rules conflict with context catalog for %s", t.ID)
		}
		s.known[t.ID] = t
	}
	languages := map[string]LanguageStat{}
	inspect := func(f inputFile) error {
		a := analyze(f)
		if a.stat.Language != "" {
			total := languages[a.stat.Language]
			total.Language = a.stat.Language
			total.Files++
			total.Lines += a.stat.Lines
			total.Code += a.stat.Code
			total.Comments += a.stat.Comments
			total.Blank += a.stat.Blank
			languages[a.stat.Language] = total
		}
		extra, inspectErr := s.inspectPortfolioManifest(f)
		warnings = append(warnings, extra...)
		if inspectErr != nil {
			return fmt.Errorf("%s: %w", f.name, inspectErr)
		}
		// A manifest reader must not bypass language observations or reference rules.
		if err := s.inspect(f, a); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		return nil
	}
	err = fs.WalkDir(confined.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("read repository path %s: %w", name, walkErr)
		}
		if d.IsDir() {
			if name != "." && skipDirectory(d.Name()) && d.Name() != ".claude-plugin" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !PortfolioPathEligible(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > MaxFileBytes {
			warnings = append(warnings, fmt.Sprintf("Skipped %s: eligible file exceeds 2 MiB.", name))
			return nil
		}
		f, err := confined.Open(name)
		if err != nil {
			return err
		}
		b, readErr := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(b) > MaxFileBytes {
			warnings = append(warnings, fmt.Sprintf("Skipped %s: eligible file exceeds 2 MiB.", name))
			return nil
		}
		digest := sha256.Sum256(b)
		if (rules.fingerprint != "" && digest == rules.sourceHash) || (pack.fingerprint != "" && digest == pack.sourceHash) {
			return nil
		}
		for _, inputHash := range ignored {
			if digest == inputHash {
				return nil
			}
		}
		if bytes.HasPrefix(bytes.TrimSpace(b), []byte(GeneratedMarker)) {
			return nil
		}
		if path.Ext(name) == ".json" && bytes.Contains(b, []byte(`"generator_version"`)) {
			if _, err := ReadReport(b); err == nil {
				return nil
			}
			if bytes.Contains(b, []byte(`"tech-radar-check"`)) {
				if _, err := ReadCheckResult(b); err == nil {
					return nil
				}
			}
			if bytes.Contains(b, []byte(`"tech-radar-context"`)) {
				if _, err := ReadContext(b); err == nil {
					return nil
				}
			}
		}
		if bytes.IndexByte(b, 0) >= 0 {
			warnings = append(warnings, "Skipped "+name+": binary content in an eligible path.")
			return nil
		}
		files = append(files, scanFile{name, digest})
		if base := path.Base(name); base == "package-lock.json" || base == "mix.lock" {
			return nil
		}
		return inspect(inputFile{name, b})
	})
	if err != nil {
		return Inventory{}, nil, err
	}
	// Lockfiles need declarations collected from the first pass. Re-read only
	// those files and verify their hashes instead of retaining all source bytes.
	for _, file := range files {
		base := path.Base(file.name)
		if base != "package-lock.json" && base != "mix.lock" {
			continue
		}
		opened, err := confined.Open(file.name)
		if err != nil {
			return Inventory{}, nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(opened, MaxFileBytes+1))
		closeErr := opened.Close()
		if readErr != nil {
			return Inventory{}, nil, readErr
		}
		if closeErr != nil {
			return Inventory{}, nil, closeErr
		}
		if sha256.Sum256(data) != file.digest {
			return Inventory{}, nil, fmt.Errorf("%s: lockfile changed during scan", file.name)
		}
		f := inputFile{file.name, data}
		if base == "package-lock.json" {
			if err := s.npmLock(f); err != nil {
				return Inventory{}, nil, fmt.Errorf("%s: %w", file.name, err)
			}
		} else {
			s.mixLock(f)
		}
	}
	// Walk order differs from lexical file order when a directory has a sibling
	// such as app.go. Capture validation uses the same canonical ordering.
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	hash := sha256.New()
	var hashes []FileHash
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%x\n", file.name, file.digest)
		if extended && pack.fingerprint == "" {
			hashes = append(hashes, FileHash{Path: file.name, SHA256: hex.EncodeToString(file.digest[:])})
		}
	}
	inv := Inventory{Source: Source{Fingerprint: hex.EncodeToString(hash.Sum(nil)), Files: len(files), Counter: CounterVersion}, Findings: []Finding{}, Warnings: warnings, Languages: []LanguageStat{}}
	inv.Source.RulesFingerprint = rules.fingerprint
	inv.Source.ContextFingerprint = pack.fingerprint
	if extended || pack.fingerprint != "" {
		inv.Source.CorpusVersion = PortfolioVersion
	}
	inv.Technologies = append([]Technology(nil), rules.technologies...)
	inv.Technologies = append(inv.Technologies, pack.catalog...)
	for _, name := range sortedKeys(languages) {
		inv.Languages = append(inv.Languages, languages[name])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	inv.Source.GitHead = readGitHead(ctx, root)
	cancel()
	for _, f := range s.found {
		sort.Slice(f.Evidence, func(i, j int) bool {
			a, b := f.Evidence[i], f.Evidence[j]
			if kindOrder(a.Kind) != kindOrder(b.Kind) {
				return kindOrder(a.Kind) < kindOrder(b.Kind)
			}
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Detail < b.Detail
		})
		f.EvidenceCount = len(f.Evidence)
		// Keep and hash every occurrence. The renderer chooses what to show up front.
		f.EvidenceFingerprint = fingerprintEvidence(f.Evidence)
		inv.Findings = append(inv.Findings, *f)
	}
	sort.Slice(inv.Findings, func(i, j int) bool { return inv.Findings[i].ID < inv.Findings[j].ID })
	sort.Strings(inv.Warnings)
	inv.Components = portfolioComponents(inv.Findings, pack.profiles)
	return inv, hashes, nil
}

func skipDirectory(name string) bool {
	if strings.HasPrefix(name, ".") && name != ".github" && name != ".devcontainer" {
		return true
	}
	switch name {
	case "node_modules", "deps", "_build", "vendor", "dist", "build", "coverage", "priv", "additional_projects", "test", "tests", "testdata", "tmp", "output", "__pycache__", "venv":
		return true
	}
	return false
}

// eligible matches supported files after path and filename exclusions.
func eligible(name string) bool {
	base := path.Base(name)
	switch base {
	case "package.json", "package-lock.json", "mix.exs", "mix.lock", "go.mod", ".tool-versions", "Dockerfile", "Chart.yaml", "CLAUDE.md", "AGENTS.md":
		return true
	}
	if strings.HasSuffix(base, ".Dockerfile") {
		return true
	}
	switch path.Ext(name) {
	case ".ex", ".exs", ".js", ".mjs", ".jsx", ".ts", ".tsx", ".go", ".py", ".sh", ".rb", ".tf":
		return true
	case ".yaml", ".yml", ".json":
		return strings.HasPrefix(name, "chart/") || strings.HasPrefix(name, "infrastructure/") || strings.HasPrefix(name, ".github/") || strings.HasPrefix(name, ".devcontainer/")
	}
	if name == "docs/coding-conventions.md" || name == "docs/feature_flags.md" {
		return true
	}
	if path.Ext(name) == ".md" {
		return false
	}
	return counterSupports(name)
}

func kindOrder(kind string) int {
	switch kind {
	case "declaration":
		return 0
	case "lock resolution":
		return 1
	case "tool pin":
		return 2
	case "configuration":
		return 3
	case "source reference":
		return 4
	case "documentation":
		return 5
	}
	return 6
}

func (s *scanner) add(id string, f inputFile, line int, kind, detail string) {
	if alias, ok := s.aliases[id]; ok {
		id = alias
	}
	finding := s.found[id]
	if finding == nil {
		tech, ok := s.known[id]
		if !ok {
			tech = Technology{ID: id, Name: id}
		}
		finding = &Finding{ID: id, Name: tech.Name, Quadrant: tech.Quadrant, Evidence: []Evidence{}, Presence: "detected", ReviewRequired: true}
		s.found[id] = finding
	}
	e := Evidence{f.name, line, kind, detail}
	if s.seen[id] == nil {
		s.seen[id] = map[Evidence]bool{}
	}
	if s.seen[id][e] {
		return
	}
	s.seen[id][e] = true
	finding.Evidence = append(finding.Evidence, e)
}

var safeVersion = regexp.MustCompile(`^[0-9vVxX*^~<>=|, .+_-]+$`)

func versionValue(value string) string {
	if len(value) <= 120 {
		if _, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v")); err == nil {
			return value
		}
	}
	if len(value) <= 120 && (safeVersion.MatchString(value) || value == "latest" || value == "workspace:*" || value == "catalog:") {
		return value
	}
	return "[non-version source reference omitted]"
}

func (s *scanner) dependency(f inputFile, ecosystem, name, value, kind string, line int, scope string) {
	if kind == "declaration" {
		key := ecosystem + ":" + path.Dir(f.name)
		if s.declared[key] == nil {
			s.declared[key] = map[string]bool{}
		}
		s.declared[key][name] = true
	}
	detail := ecosystem + " package " + name + ": " + versionValue(value)
	if scope != "" {
		detail += " [" + scope + "]"
	}
	s.add(ecosystem+":"+name, f, line, kind, detail)
}

var mixDep = regexp.MustCompile(`^\s*\{\s*:([a-zA-Z0-9_]+)\s*,(?:\s*"([^"]*)")?`)
var mixLocked = regexp.MustCompile(`^\s*"([^"]+)":\s*\{:hex,\s*:[^,]+,\s*"([^"]+)"`)
var goRequire = regexp.MustCompile(`^\s*(?:require\s+)?([\w.\-]+(?:/[\w.\-]+)+)\s+(v[^\s]+)`)

func (s *scanner) inspect(f inputFile, a analysis) error {
	base := path.Base(f.name)
	switch base {
	case "package.json":
		if err := s.npmManifest(f); err != nil {
			return err
		}
		// Package declarations handle built-in detection here.
		// Apply user-supplied rules separately.
		if len(s.patterns) > len(patterns) {
			s.references(f, a, s.patterns[len(patterns):])
		}
		return nil
	case "mix.exs":
		s.add("elixir", f, 1, "declaration", "Mix project manifest")
		for i, line := range strings.Split(string(f.data), "\n") {
			if m := mixDep.FindStringSubmatch(line); m != nil {
				value := m[2]
				if value == "" {
					value = "[non-version source reference omitted]"
				}
				scope := ""
				if strings.Contains(line, "only:") {
					scope = "restricted Mix environment"
				}
				s.dependency(f, "hex", m[1], value, "declaration", i+1, scope)
			}
		}
	case "go.mod":
		s.add("go", f, 1, "declaration", "Go module manifest")
		for i, line := range strings.Split(string(f.data), "\n") {
			if strings.Contains(line, "// indirect") {
				continue
			}
			if m := goRequire.FindStringSubmatch(line); m != nil {
				s.dependency(f, "go", m[1], m[2], "declaration", i+1, "")
			}
		}
	case ".tool-versions":
		for i, line := range strings.Split(string(f.data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			id := toolVersionIDs[fields[0]]
			if id != "" {
				s.add(id, f, i+1, "tool pin", fields[0]+" pin "+versionValue(fields[1]))
			}
		}
	case "Dockerfile":
		s.add("docker", f, 1, "configuration", "Container build definition")
	case "Chart.yaml":
		s.add("helm", f, 1, "configuration", "Helm chart metadata")
	}
	if strings.HasSuffix(base, ".Dockerfile") {
		s.add("docker", f, 1, "configuration", "Container build definition")
	}
	if strings.HasPrefix(f.name, ".github/workflows/") {
		s.add("github-actions", f, 1, "configuration", "GitHub Actions workflow definition")
	}
	if strings.HasSuffix(f.name, "ecs-task-definition.json") {
		s.add("amazon-ecs", f, 1, "configuration", "ECS task-definition file")
	}
	if path.Ext(f.name) == ".tf" {
		s.add("terraform", f, 1, "configuration", "Terraform configuration file")
	}
	if id := languageID(a.stat.Language); a.stat.Language != "" && id != "" {
		s.add(id, f, 1, "source reference", "SCC language classification: "+a.stat.Language)
		if strings.HasPrefix(id, "language:") {
			s.found[id].Name = a.stat.Language
			s.found[id].Quadrant = "Languages & Frameworks"
		}
	}
	s.references(f, a, s.patterns)
	return nil
}

func (s *scanner) references(f inputFile, a analysis, rules []sourcePattern) {
	base := path.Base(f.name)
	doc := path.Ext(f.name) == ".md"
	builtin := f.data
	if path.Ext(f.name) == ".go" {
		builtin = goReferenceText(f.data)
	}
	builtinLines := strings.Split(string(builtin), "\n")
	for i, raw := range strings.Split(string(f.data), "\n") {
		line := strings.TrimSpace(raw)
		if !doc && !a.isCodeLine(i) {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		for _, rule := range rules {
			if rule.doc != doc || (len(rule.languages) > 0 && !contains(rule.languages, a.stat.Language)) {
				continue
			}
			text := line
			if !rule.custom {
				text = builtinLines[i]
			}
			if rule.re.MatchString(text) {
				kind := "source reference"
				if doc {
					kind = "documentation"
				} else if path.Ext(f.name) == ".yml" || path.Ext(f.name) == ".yaml" || path.Ext(f.name) == ".json" || strings.HasPrefix(f.name, "config/") || base == "Dockerfile" {
					kind = "configuration"
				}
				s.add(rule.id, f, i+1, kind, rule.label)
			}
		}
	}
}

func (s *scanner) npmManifest(f inputFile) error {
	var obj struct {
		Dependencies map[string]string `json:"dependencies"`
		Dev          map[string]string `json:"devDependencies"`
		Optional     map[string]string `json:"optionalDependencies"`
		Peer         map[string]string `json:"peerDependencies"`
		Engines      map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(f.data, &obj); err != nil {
		return fmt.Errorf("invalid package.json: %w", err)
	}
	for _, section := range []struct {
		name string
		deps map[string]string
	}{{"dependencies", obj.Dependencies}, {"devDependencies", obj.Dev}, {"optionalDependencies", obj.Optional}, {"peerDependencies", obj.Peer}} {
		for name, value := range section.deps {
			s.dependency(f, "npm", name, value, "declaration", jsonKeyLine(f.data, name), section.name)
		}
	}
	if version, ok := obj.Engines["node"]; ok {
		s.add("nodejs", f, jsonKeyLine(f.data, "node"), "declaration", "Node engine constraint "+versionValue(version))
	}
	return nil
}

func jsonKeyLine(data []byte, key string) int {
	quoted, _ := json.Marshal(key)
	index := bytes.Index(data, quoted)
	if index < 0 {
		return 1
	}
	return bytes.Count(data[:index], []byte("\n")) + 1
}

func (s *scanner) npmLock(f inputFile) error {
	var obj struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(f.data, &obj); err != nil {
		return fmt.Errorf("invalid package-lock.json: %w", err)
	}
	for name := range s.declared["npm:"+path.Dir(f.name)] {
		entry, ok := obj.Packages["node_modules/"+name]
		version := entry.Version
		if !ok {
			version = obj.Dependencies[name].Version
		}
		if version != "" {
			s.dependency(f, "npm", name, version, "lock resolution", jsonKeyLine(f.data, "node_modules/"+name), "")
		}
	}
	return nil
}

func (s *scanner) mixLock(f inputFile) {
	for i, line := range strings.Split(string(f.data), "\n") {
		if m := mixLocked.FindStringSubmatch(line); m != nil && s.declared["hex:"+path.Dir(f.name)][m[1]] {
			s.dependency(f, "hex", m[1], m[2], "lock resolution", i+1, "")
		}
	}
}

type sourcePattern struct {
	id, label string
	re        *regexp.Regexp
	doc       bool
	languages []string
	custom    bool
}

func pattern(id, label, expression string) sourcePattern {
	return sourcePattern{id: id, label: label, re: regexp.MustCompile(expression)}
}

func syntaxPattern(id, label, expression string, languages ...string) sourcePattern {
	p := pattern(id, label, expression)
	p.languages = languages
	return p
}

var patterns = []sourcePattern{
	syntaxPattern("phoenix", "Phoenix endpoint/router runtime wiring", `\buse\s+Phoenix\.(?:Endpoint|Router|Controller)\b`, "Elixir"),
	syntaxPattern("phoenix-liveview", "LiveView runtime wiring", `\buse\s+Phoenix\.Live(?:View|Component)\b`, "Elixir"),
	syntaxPattern("ecto", "Ecto repository runtime wiring", `\buse\s+Ecto\.Repo\b`, "Elixir"),
	syntaxPattern("oban", "Oban job runtime wiring", `\buse\s+Oban\.Worker\b|\bOban\.(?:insert|insert!|start_link)\b`, "Elixir"),
	syntaxPattern("broadway", "Broadway consumer runtime wiring", `\buse\s+Broadway\b|\bBroadway\.start_link\b`, "Elixir"),
	syntaxPattern("otp-supervision", "OTP supervision runtime wiring", `\buse\s+(?:Application|Supervisor|DynamicSupervisor)\b|\bSupervisor\.start_link\b`, "Elixir"),
	syntaxPattern("react", "React module runtime wiring", `\bfrom\s+["']react["']|\brequire\(["']react["']\)`, "JavaScript", "JSX", "TypeScript", "Vue", "Svelte", "Astro", "MDX"),
	syntaxPattern("tailwind", "Tailwind style pipeline wiring", `@tailwind\s+(?:base|components|utilities)\b|@import\s+["']tailwindcss["']`, "CSS", "SCSS", "Sass", "LESS", "HTML", "Vue", "Svelte", "Astro"),
	syntaxPattern("ash", "Ash resource/domain reference", `\b(?:use|extensions:)\s+Ash\.`, "Elixir"),
	syntaxPattern("req", "Req HTTP/test reference", `\bReq\.(?:new|get|post|put|delete|Test)\b`, "Elixir"),
	pattern("hubspot", "HubSpot integration configuration/reference", `\bHUBSPOT_[A-Z_]+\b`),
	pattern("postmark", "Postmark integration configuration/reference", `\bPOSTMARK_[A-Z_]+\b|Swoosh\.Adapters\.Postmark`),
	pattern("myutilities", "MyUtilities integration configuration/reference", `\bMYUTILITIES_[A-Z_]+\b`),
	pattern("bindable", "Bindable integration configuration/reference", `\bBINDABLE_[A-Z_]+\b`),
	pattern("local-logic", "Local Logic integration reference", `\bLOCAL_LOGIC_[A-Z_]+\b`),
	pattern("mapbox", "Mapbox integration configuration/reference", `\bMAPBOX_[A-Z_]+\b`),
	pattern("smarty", "Smarty integration configuration/reference", `\bSMARTY_[A-Z_]+\b`),
	pattern("litellm", "LiteLLM gateway configuration/reference", `\bLITELLM_[A-Z_]+\b`),
	pattern("honeycomb", "Honeycomb exporter configuration/reference", `\bHONEYCOMB_[A-Z_]+\b|api\.honeycomb\.io`),
	pattern("opentelemetry", "OpenTelemetry instrumentation reference", `\b(?:OpenTelemetry|OpentelemetryPhoenix|OpentelemetryEcto)\.`),
	pattern("amazon-s3", "S3 API or permission reference", `\bExAws\.S3\b|\bs3:(?:GetObject|PutObject|ListBucket)\b`),
	pattern("amazon-sqs", "SQS consumer/queue reference", `\bBroadwaySQS\b|\bSQS_QUEUE_URL\b`),
	pattern("aws-kms", "KMS API/signer reference", `\bExAws\.KMS\b`),
	pattern("aws-secrets", "AWS secret-store configuration/reference", `secretsmanager:|ssm:|kind:\s*ExternalSecret\b`),
	pattern("amazon-ecs", "ECS delivery configuration/reference", `aws ecs\b|aws-actions/amazon-ecs|requiresCompatibilities`),
	pattern("amazon-eks", "EKS-specific configuration/reference", `eks\.amazonaws\.com|aws eks\b`),
	pattern("kubernetes", "Kubernetes workload/resource reference", `kind:\s*(?:Deployment|StatefulSet|DaemonSet|Pod)\b|\bkubectl\s`),
	pattern("postgresql", "PostgreSQL image declaration", `\bpostgres:(?:[0-9]+|latest)\b`),
	pattern("minio", "MinIO service declaration", `minio/minio|minio server`),
	pattern("nodejs", "Node image/setup reference", `\bnode:[0-9]+|nodesource\.com/setup_[0-9]+|actions/setup-node@`),
	pattern("erlang-otp", "OTP image version default", `\bARG\s+OTP_VERSION=`),
	pattern("k6", "k6 execution reference", `\bk6\s+run\b`),
	{id: "ash-first", label: "Documented Ash-first development guidance", re: regexp.MustCompile(`(?i)ash[- ]first|ash patterns, not raw ecto`), doc: true},
	{id: "traced-http", label: "Documented traced HTTP-client convention", re: regexp.MustCompile(`(?i)include OpenTelemetry tracing|always add OpenTelemetry tracing`), doc: true},
}

func readGitHead(ctx context.Context, root string) string {
	localEnv := exec.CommandContext(ctx, "git", "rev-parse", "--local-env-vars")
	localEnv.WaitDelay = 100 * time.Millisecond
	names, err := localEnv.Output()
	if err != nil {
		return ""
	}
	// Hooks export repository-local settings that override Git's -C selection.
	localNames := make(map[string]bool)
	for name := range strings.FieldsSeq(string(names)) {
		localNames[name] = true
	}
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "HEAD")
	cmd.Env = []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if runtime.GOOS == "windows" {
			name = strings.ToUpper(name)
		}
		if !localNames[name] {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.WaitDelay = 100 * time.Millisecond
	if b, err := cmd.Output(); err == nil && gitHeadPattern.Match(bytes.TrimSpace(b)) {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// Evidence contains only strings and integers, so JSON encoding cannot fail.
func fingerprintEvidence(evidence []Evidence) string {
	data, _ := json.Marshal(evidence)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
