package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

const handbookURL = "https://beardedeagle.github.io/tech-radar/"

var helpCommands = map[string]struct {
	summary, usage, examples, notes string
}{
	"generate": {
		summary: "Create a technology radar from repository evidence.",
		usage:   "--repo PATH [options]",
		examples: `  Create an HTML radar:
    tech-radar generate --repo . --format html --out TECHNOLOGY_RADAR.html

  Start a lifecycle baseline with a table:
    tech-radar generate --repo . --model lifecycle --format html --out LIFECYCLE.html

  Update a radar while keeping its decisions and placement history:
    tech-radar generate --repo . --previous TECHNOLOGY_RADAR.html --format html --out NEXT_RADAR.html`,
		notes: `Models:
  recommendation  Suggest Adopt, Trial, Assess or Caution from source evidence.
  lifecycle       Use supplied Adopt, Experiment, Hold or Retire decisions.
                  Entries without a decision stay Needs decision.

Views:
  radar  Circular chart in HTML, quadrant/ring matrix in Markdown.
  table  Grouped technology choices with decision and review details.
  Defaults: radar for recommendation, table for lifecycle. A same-model previous
  report supplies its view unless --view overrides it.

Markdown is the default output format. HTML and Markdown files keep full evidence
in an adjacent JSON file. Commit both files, or use --self-contained to embed it.
An initial lifecycle baseline has no missing-reference review gaps; passing
--previous activates those checks on subsequent runs.`,
	},
	"scan": {
		summary: "Export repository findings and full evidence as JSON by default.",
		usage:   "--repo PATH [options]",
		examples: `  Export findings for further analysis:
    tech-radar scan --repo . --out inventory.json

  Include additional detection rules:
    tech-radar scan --repo . --rules rules.json --out inventory.json`,
		notes: `Scan and generate use the same repository inspection and report model.
Scan defaults to JSON; generate defaults to Markdown. Use generate for a readable
radar, or --format to choose another encoding. Repository code is not executed.`,
	},
	"discover": {
		summary: "Build shared technology context from repositories or a GitHub organization.",
		usage:   "(--repo PATH ... | --org NAME | --repos FILE | --from FILE) [options]",
		examples: `  Combine local repositories:
    tech-radar discover --repo ./api --repo ./web --out context.json

  Inspect accessible repositories in a GitHub organization:
    tech-radar discover --org example-org --out context.json

  Use that context when generating a repository's radar:
    tech-radar generate --repo ./api --context context.json --format html --out RADAR.html`,
		notes: `Choose exactly one input mode. Repeat --repo for local repositories, or use
--repos for a schema-1 JSON repository list. --from rebuilds saved context without
refreshing repository membership or revisions. Output is JSON.`,
	},
	"check": {
		summary: "Check technology policy violations and changes since a baseline.",
		usage:   "(--repo PATH ... | --org NAME | --repos FILE | --from FILE) --policy FILE [options]",
		examples: `  Check a local repository against an approved standard:
    tech-radar check --repo . --policy standard.json --out check.json

  Compare against a previous check:
    tech-radar check --repo . --policy standard.json --baseline check.json --out next-check.json`,
		notes: `Choose exactly one input mode. --policy requires an approved enforcement policy,
separate from generate's display decisions. --baseline compares technologies,
versions and evidence, and is required for existing-only rules. --from checks
saved observations without rescanning repositories. Output is JSON.

Exit codes:
  0  No observed violations or recorded gaps.
  1  The command failed.
  2  Policy violations, possibly with gaps.
  3  Gaps only.`,
	},
	"catalog": {
		summary:  "List built-in technology names and IDs as JSON.",
		examples: "  tech-radar catalog\n  tech-radar catalog > catalog.json",
		notes:    "Use these IDs in detection, grouping and policy inputs.",
	},
	"version": {
		summary:  "Print the CLI and language-counter versions.",
		examples: "  tech-radar version",
	},
}

func writeHelp(command string, flags *flag.FlagSet, output io.Writer) error {
	var b strings.Builder
	if command == "" {
		b.WriteString("Inspect repositories, build technology radars, and check technology policy.\n\nUsage:\n  tech-radar <command> [options]\n\nCommands:\n")
		for _, name := range []string{"generate", "scan", "discover", "check", "catalog", "version"} {
			fmt.Fprintf(&b, "  %-10s %s\n", name, helpCommands[name].summary)
		}
		b.WriteString(`  help       Show help for a command.

Quick start:
  Create an HTML radar for the current repository:
    tech-radar generate --repo . --format html --out TECHNOLOGY_RADAR.html

  Start a lifecycle table with unplaced technologies marked Needs decision:
    tech-radar generate --repo . --model lifecycle --format html --out LIFECYCLE.html

The default recommendation model suggests Adopt, Trial, Assess or Caution.
Lifecycle uses supplied Adopt, Experiment, Hold or Retire decisions.
Reports group the main findings and keep full evidence in a companion JSON file.

Use tech-radar <command> --help or tech-radar help <command> for options,
defaults and more examples.
`)
	} else {
		h, ok := helpCommands[command]
		if !ok {
			return fmt.Errorf("unknown command %q; run 'tech-radar --help' for commands", command)
		}
		fmt.Fprintf(&b, "%s\n\nUsage:\n  tech-radar %s", h.summary, command)
		if h.usage != "" {
			fmt.Fprintf(&b, " %s", h.usage)
		}
		fmt.Fprintf(&b, "\n\nExamples:\n%s\n", h.examples)
		if h.notes != "" {
			fmt.Fprintf(&b, "\n%s\n", h.notes)
		}
		if flags != nil {
			b.WriteString("\nOptions:\n")
			tabs := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
			flags.VisitAll(func(f *flag.Flag) {
				value, description := flag.UnquoteUsage(f)
				option := "--" + f.Name
				if value != "" {
					option += " " + value
				}
				if f.DefValue != "" && f.DefValue != "false" {
					description += " (default: " + f.DefValue + ")"
				}
				fmt.Fprintf(tabs, "  %s\t%s\n", option, description)
			})
			fmt.Fprintln(tabs, "  -h, --help\tShow this help.")
			tabs.Flush()
		}
	}
	fmt.Fprintf(&b, "\nHandbook: %s\n", handbookURL)
	_, err := io.WriteString(output, b.String())
	return err
}

// Keep help on stdout and parse errors with their command-specific help hint.
func parseCommandFlags(flags *flag.FlagSet, args []string, stdout io.Writer) error {
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		if err := writeHelp(flags.Name(), flags, stdout); err != nil {
			return err
		}
		return flag.ErrHelp
	}
	if err != nil {
		return fmt.Errorf("%w; run 'tech-radar %s --help' for options and examples", err, flags.Name())
	}
	return nil
}
