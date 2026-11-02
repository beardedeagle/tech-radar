package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"tech-radar/internal/radar"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, stdout, stderr io.Writer) (exitCode int) {
	fail := func(err error) int { fmt.Fprintln(stderr, "tech-radar:", err); return 1 }
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: tech-radar generate|scan --repo PATH [--context PACK] [--rules FILE] [--grouping FILE] [--policy FILE] [--previous FILE] [--name NAME] [--format markdown|html|json] [--out FILE] [--force] [--self-contained]\n       tech-radar discover (--repo PATH ... | --org NAME | --repos FILE | --from PACK) [--rules FILE] [--out FILE] [--force]\n       tech-radar check (--repo PATH ... | --org NAME | --repos FILE | --from PACK) --policy FILE [--exceptions FILE] [--baseline FILE] [--as-of YYYY-MM-DD] [--rules FILE] [--out FILE] [--force]\n       tech-radar catalog|version")
		if err != nil {
			return fail(err)
		}
		return 0
	}
	command := args[0]
	switch command {
	case "check":
		code, err := checkCommand(args[1:], stdout, stderr)
		if err != nil {
			return fail(err)
		}
		return code
	case "discover":
		if err := discover(args[1:], stdout, stderr); err != nil {
			return fail(err)
		}
		return 0
	case "version":
		if _, err := fmt.Fprintln(stdout, "tech-radar", radar.Version, "("+radar.CounterVersion+")"); err != nil {
			return fail(err)
		}
		return 0
	case "catalog":
		if len(args) != 1 {
			return fail(fmt.Errorf("catalog does not accept arguments"))
		}
		b, err := json.MarshalIndent(radar.Catalog(), "", "  ")
		if err != nil {
			return fail(err)
		}
		_, err = fmt.Fprintln(stdout, string(b))
		if err != nil {
			return fail(err)
		}
		return 0
	case "generate", "scan":
	default:
		return fail(fmt.Errorf("unknown command %q", command))
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", "", "repository directory (read-only)")
	policyPath := flags.String("policy", "", "team policy JSON file")
	rulesPath := flags.String("rules", "", "custom detection rules JSON file")
	groupingPath := flags.String("grouping", "", "explicit grouping catalog overrides JSON file")
	contextPath := flags.String("context", "", "context JSON file from discover")
	previousPath := flags.String("previous", "", "prior tech-radar Markdown, HTML or JSON export")
	name := flags.String("name", "", "repository display name")
	format := flags.String("format", "", "markdown, html or json (default markdown; scan defaults to json)")
	out := flags.String("out", "-", "output file, or - for stdout")
	force := flags.Bool("force", false, "replace an unedited generated report")
	selfContained := flags.Bool("self-contained", false, "include all evidence in one Markdown or HTML file")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		return fail(fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " ")))
	}
	if *repo == "" {
		return fail(fmt.Errorf("--repo is required"))
	}
	if *format == "" {
		*format = "markdown"
		if command == "scan" {
			*format = "json"
		}
	}
	if *format != "markdown" && *format != "html" && *format != "json" {
		return fail(fmt.Errorf("unsupported --format %q", *format))
	}
	var output *outputFile
	if *out != "-" {
		var err error
		output, err = beginOutput(*out, *force, *policyPath, *rulesPath, *contextPath, *groupingPath)
		if err != nil {
			return fail(err)
		}
		defer func() {
			if err := output.Close(); err != nil {
				exitCode = fail(err)
			}
		}()
	}
	var rules radar.DetectionRules
	if *rulesPath != "" {
		data, err := readInput(*rulesPath)
		if err != nil {
			return fail(err)
		}
		rules, err = radar.ReadDetectionRules(data)
		if err != nil {
			return fail(err)
		}
	}
	var policy radar.Policy
	var grouping radar.GroupingRules
	if *groupingPath != "" {
		data, err := readInput(*groupingPath)
		if err != nil {
			return fail(err)
		}
		grouping, err = radar.ReadGroupingRules(data)
		if err != nil {
			return fail(err)
		}
	}
	if *policyPath != "" {
		b, err := readInput(*policyPath)
		if err != nil {
			return fail(err)
		}
		policy, err = radar.ReadPolicy(b)
		if err != nil {
			return fail(err)
		}
	}
	var previous *radar.Report
	if *previousPath != "" {
		report, err := readPrevious(*previousPath)
		if err != nil {
			return fail(err)
		}
		previous = &report
	}
	if previous != nil && previous.Source.RulesFingerprint != "" && *rulesPath == "" {
		return fail(fmt.Errorf("previous report used custom rules; pass --rules again, or an empty patterns array to remove them"))
	}
	if previous != nil && previous.Source.ContextFingerprint != "" && *contextPath == "" {
		return fail(fmt.Errorf("previous report used a context file; pass --context again"))
	}
	if previous != nil && previous.Source.GroupingFingerprint != "" && *groupingPath == "" {
		return fail(fmt.Errorf("previous report used custom grouping; pass --grouping again, or an empty families array to remove overrides"))
	}
	var inv radar.Inventory
	var err error
	var context radar.Context
	if *contextPath != "" {
		data, readErr := readInputLimit(*contextPath, radar.MaxContextBytes)
		if readErr != nil {
			return fail(readErr)
		}
		context, readErr = radar.ReadContext(data)
		if readErr != nil {
			return fail(readErr)
		}
	}
	inv, err = radar.ScanWithOptions(*repo, radar.ScanOptions{Rules: rules, Context: context, Grouping: grouping})
	if err != nil {
		return fail(err)
	}
	report, err := radar.BuildWithGrouping(inv, policy, previous, *name, grouping)
	if err != nil {
		return fail(err)
	}
	var b []byte
	if *out == "-" || *format == "json" || *selfContained {
		b, err = radar.Render(report, *format)
		if err != nil {
			return fail(err)
		}
	}
	if *out == "-" {
		_, err = stdout.Write(b)
		if err != nil {
			return fail(err)
		}
	} else {
		if *format != "json" && !*selfContained {
			err = output.writeDocument(report, *format, *policyPath, *rulesPath, *contextPath, *groupingPath)
		} else {
			err = output.write(b)
		}
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stderr, "Generated %s: %d entries, %d policy exclusions, %d scan warnings\n", *out, len(report.Findings), len(report.Excluded), len(report.Warnings))
	}
	return 0
}

// readPrevious checks the report and its adjacent evidence file, if present.
// Without the evidence file, saved decisions and review flags still carry forward.
func readPrevious(name string) (radar.Report, error) {
	b, err := readInput(name)
	if err != nil {
		return radar.Report{}, err
	}
	r, err := radar.ReadReport(b)
	if err != nil || r.EvidenceFile == "" {
		return r, err
	}
	companion := filepath.Join(filepath.Dir(name), r.EvidenceFile)
	info, err := os.Lstat(companion)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() {
		return r, fmt.Errorf("evidence companion must be a regular file")
	}
	confined, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return r, err
	}
	defer confined.Close()
	f, err := confined.Open(r.EvidenceFile)
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err = readOpenedInput(f, companion)
	if err != nil {
		return r, err
	}
	digest := sha256.Sum256(b)
	if hex.EncodeToString(digest[:]) != r.EvidenceSHA256 {
		return r, fmt.Errorf("evidence file checksum doesn't match; keep these files and regenerate from an intact report")
	}
	full, err := radar.ReadReport(b)
	if err != nil {
		return r, err
	}
	if full.EvidenceFile != "" {
		return r, fmt.Errorf("evidence companion cannot reference another companion")
	}
	return full, nil
}

// writeDocument writes the evidence file before the report that links it.
// A failed write can leave unused evidence, but can't leave a broken report link.
// Existing evidence files must match byte for byte.
func (f *outputFile) writeDocument(report radar.Report, format string, protectedPaths ...string) error {
	name := f.path
	evidence, err := radar.Render(report, "json")
	if err != nil {
		return err
	}
	digest := sha256.Sum256(evidence)
	sha := hex.EncodeToString(digest[:])
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)) + ".evidence." + sha + ".json"
	companion := filepath.Join(filepath.Dir(name), base)
	if _, err := outputPath(companion, true, protectedPaths...); err != nil {
		return err
	}
	document, err := radar.RenderDocument(report, format, base, sha)
	if err != nil {
		return err
	}
	prior, err := readInput(companion)
	if err == nil {
		if !bytes.Equal(prior, evidence) {
			return fmt.Errorf("refusing to replace different evidence companion %s", companion)
		}
	} else if os.IsNotExist(err) {
		if err := writeOutput(companion, evidence, false, protectedPaths...); err != nil {
			return err
		}
	} else {
		return err
	}
	return f.write(document)
}

const maxInputBytes = 16 << 20

func readInput(name string) ([]byte, error) {
	return readInputLimit(name, maxInputBytes)
}

func readInputLimit(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readOpenedInputLimit(f, name, limit)
}

func readOpenedInput(f *os.File, name string) ([]byte, error) {
	return readOpenedInputLimit(f, name, maxInputBytes)
}

func readOpenedInputLimit(f *os.File, name string, limit int64) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular input file", name)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d MiB input limit", name, limit>>20)
	}
	return b, nil
}

func rejectSymlinks(name string) error {
	for current := name; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink output path %s", current)
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

type outputState struct {
	info   os.FileInfo
	digest [32]byte
}

func outputPath(name string, force bool, protectedPaths ...string) (string, error) {
	abs, _, err := prepareOutput(name, force, protectedPaths...)
	return abs, err
}

func resolveOutput(name string, protectedPaths ...string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	// Resolve macOS system aliases such as /tmp. Other symlink parents stay rejected.
	if runtime.GOOS == "darwin" {
		for _, prefix := range []string{"/var", "/tmp", "/etc"} {
			if strings.HasPrefix(abs, prefix+"/") {
				resolved, err := filepath.EvalSymlinks(prefix)
				if err != nil {
					return "", err
				}
				abs = resolved + strings.TrimPrefix(abs, prefix)
				break
			}
		}
	}
	for _, inputPath := range protectedPaths {
		if inputPath == "" {
			continue
		}
		policyAbs, err := filepath.Abs(inputPath)
		if err != nil {
			return "", err
		}
		inputInfo, _ := os.Stat(inputPath)
		outputInfo, _ := os.Stat(abs)
		if policyAbs == abs || (inputInfo != nil && outputInfo != nil && os.SameFile(inputInfo, outputInfo)) {
			return "", fmt.Errorf("output must not replace an input file")
		}
	}
	switch filepath.Base(abs) {
	case "package.json", "package-lock.json", "mix.exs", "mix.lock", "go.mod", "go.sum", ".tool-versions", "pyproject.toml", "requirements.txt", "requirements-dev.txt", "Gemfile", "composer.json", "plugin.json", ".mcp.json", "SKILL.md":
		return "", fmt.Errorf("output must not replace a source manifest")
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if ext != ".md" && ext != ".html" && ext != ".json" {
		return "", fmt.Errorf("output needs a .md, .html or .json filename")
	}
	if err := rejectSymlinks(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func prepareOutput(name string, force bool, protectedPaths ...string) (string, outputState, error) {
	var priorState outputState
	abs, err := resolveOutput(name, protectedPaths...)
	if err != nil {
		return "", priorState, err
	}
	if info, err := os.Lstat(abs); err == nil {
		if !force {
			return "", priorState, fmt.Errorf("%s already exists; use --force only to replace a generated export", name)
		}
		f, err := os.Open(abs)
		if err != nil {
			return "", priorState, err
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil {
			return "", priorState, err
		}
		if !os.SameFile(info, opened) {
			return "", priorState, fmt.Errorf("output changed while reading: %s", name)
		}
		prior, err := readOpenedInputLimit(f, abs, radar.MaxContextBytes)
		if err != nil {
			return "", priorState, err
		}
		if _, err := radar.ReadReport(prior); err != nil {
			if _, err := radar.ReadContext(prior); err != nil {
				if _, err := radar.ReadCheckResult(prior); err != nil {
					return "", priorState, fmt.Errorf("refusing to overwrite a non-generated or edited file: %s", name)
				}
			}
		}
		priorState = outputState{info: opened, digest: sha256.Sum256(prior)}
	} else if !os.IsNotExist(err) {
		return "", priorState, err
	}
	return abs, priorState, nil
}

func writeOutput(name string, data []byte, force bool, protectedPaths ...string) (err error) {
	f, err := beginOutput(name, force, protectedPaths...)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return f.write(data)
}

// write stages data, then checks the original output state before publishing it.
func (output *outputFile) write(data []byte) error {
	dir := filepath.Dir(output.path)
	f, err := os.CreateTemp(dir, ".tech-radar-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, 0644); err != nil {
		return err
	}
	return publishOutput(tmp, output.path, output.prior)
}

func publishOutput(tmp, abs string, prior outputState) error {
	if prior.info == nil {
		// Link fails if another process created the destination while we were writing.
		return os.Link(tmp, abs)
	}
	if err := rejectSymlinks(abs); err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(prior.info, info) {
		return fmt.Errorf("output changed during generation; keep the edit and regenerate: %s", abs)
	}
	data, err := readInputLimit(abs, radar.MaxContextBytes)
	if err != nil {
		return err
	}
	if sha256.Sum256(data) != prior.digest {
		return fmt.Errorf("output changed during generation; keep the edit and regenerate: %s", abs)
	}
	// Another program can still edit the file between this check and Rename.
	return os.Rename(tmp, abs)
}
