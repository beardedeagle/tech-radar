package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"tech-radar/internal/radar"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, stdout, stderr io.Writer) (exitCode int) {
	fail := func(err error) int { fmt.Fprintln(stderr, "tech-radar:", err); return 1 }
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || (args[0] == "help" && len(args) == 1) {
		if err := writeHelp("", nil, stdout); err != nil {
			return fail(err)
		}
		return 0
	}
	if args[0] == "help" {
		if len(args) != 2 {
			return fail(fmt.Errorf("usage: tech-radar help <command>"))
		}
		return run([]string{args[1], "--help"}, stdout, stderr)
	}
	command := args[0]
	if (command == "catalog" || command == "version") && len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		if err := writeHelp(command, nil, stdout); err != nil {
			return fail(err)
		}
		return 0
	}
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
		return fail(fmt.Errorf("unknown command %q; run 'tech-radar --help' for commands", command))
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	repo := flags.String("repo", "", "read repository at `PATH` without running its code (required)")
	policyPath := flags.String("policy", "", "read display decisions from JSON `FILE`, separate from check's enforcement policy")
	rulesPath := flags.String("rules", "", "add detection patterns from JSON `FILE`")
	groupingPath := flags.String("grouping", "", "override technology families with JSON `FILE`")
	contextPath := flags.String("context", "", "use shared context from discover's JSON `FILE`")
	previousPath := flags.String("previous", "", "carry decisions and compare placements with prior report `FILE`")
	name := flags.String("name", "", "set the repository display name to `NAME`")
	model := flags.String("model", "", "use `MODEL`: recommendation or lifecycle; inherits policy or previous model")
	view := flags.String("view", "", "use `VIEW`: radar or table; inherits same-model previous view or model default")
	defaultFormat := "markdown"
	if command == "scan" {
		defaultFormat = "json"
	}
	format := flags.String("format", "", "write `FORMAT`: markdown, html or json (default: "+defaultFormat+")")
	out := flags.String("out", "-", "write output to `FILE`, or - for stdout")
	force := flags.Bool("force", false, "replace an unedited generated report")
	selfContained := flags.Bool("self-contained", false, "include all evidence in one Markdown or HTML file")
	if err := parseCommandFlags(flags, args[1:], stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return fail(err)
	}
	if flags.NArg() != 0 {
		return fail(fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " ")))
	}
	if *repo == "" {
		return fail(fmt.Errorf("--repo is required; run 'tech-radar %s --help' for examples", command))
	}
	if *format == "" {
		*format = defaultFormat
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
	var previous *radar.Report
	if *previousPath != "" {
		report, err := readPrevious(*previousPath)
		if err != nil {
			return fail(err)
		}
		previous = &report
	}
	if *policyPath != "" {
		b, err := readInput(*policyPath)
		if err != nil {
			return fail(err)
		}
		fallback := radar.Model(*model)
		if fallback == "" && previous != nil {
			fallback = previous.Model
		}
		policy, err = radar.ReadPolicyForModel(b, fallback)
		if err != nil {
			return fail(err)
		}
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
	report, err := radar.BuildWithOptions(inv, policy, previous, *name, radar.BuildOptions{Model: radar.Model(*model), View: radar.View(*view), Grouping: grouping})
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
