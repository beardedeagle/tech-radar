package radar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// CaptureRepository collects findings and file hashes for discovery.
// It reads eligible files inside the repo and omits absolute paths and raw values.
// ignored holds SHA-256 digests of explicit runtime inputs to omit from capture.
func CaptureRepository(root string, rules DetectionRules, ignored ...[32]byte) (Inventory, []FileHash, error) {
	return scanRepository(root, rules, true, Context{}, ignored...)
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
