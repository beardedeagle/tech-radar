package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"tech-radar/internal/jsonutil"
	"tech-radar/internal/radar"
)

type repoPaths []string

func (p *repoPaths) String() string { return strings.Join(*p, ", ") }
func (p *repoPaths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("--repo needs a directory")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return err
	}
	for _, selected := range *p {
		if selected == abs {
			return fmt.Errorf("duplicate --repo path %q", value)
		}
	}
	*p = append(*p, abs)
	return nil
}

type repositoryInput struct {
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
	GitHub string `json:"github,omitempty"`
}

// discover collects repo findings or rebuilds a saved context.
// Rebuilding doesn't refresh repo membership or revisions.
func discover(args []string, stdout, stderr io.Writer) (err error) {
	flags := flag.NewFlagSet("discover", flag.ContinueOnError)
	var paths repoPaths
	flags.Var(&paths, "repo", "inspect local repository at `PATH` (repeatable)")
	org := flags.String("org", "", "inspect accessible repositories in GitHub organization `NAME`")
	list := flags.String("repos", "", "read repositories from schema-1 JSON `FILE`")
	from := flags.String("from", "", "rebuild saved context from `FILE` without rescanning")
	rulesPath := flags.String("rules", "", "add detection patterns from JSON `FILE`")
	out := flags.String("out", "-", "write context JSON to `FILE`, or - for stdout")
	force := flags.Bool("force", false, "replace an unedited generated report")
	if err := parseCommandFlags(flags, args, stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	selectors := 0
	for _, yes := range []bool{len(paths) > 0, *org != "", *list != "", *from != ""} {
		if yes {
			selectors++
		}
	}
	if selectors != 1 || flags.NArg() != 0 {
		return fmt.Errorf("choose exactly one of --repo, --org, --repos or --from; no positional arguments")
	}
	if *from != "" && *rulesPath != "" {
		return fmt.Errorf("--from uses saved findings; use --rules when collecting from repos")
	}
	var output *outputFile
	if *out != "-" {
		if strings.ToLower(filepath.Ext(*out)) != ".json" {
			return fmt.Errorf("context output requires a .json filename")
		}
		output, err = beginOutput(*out, *force, *list, *rulesPath)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, output.Close()) }()
	}
	var pack radar.Context
	if *from != "" {
		data, err := readInputLimit(*from, radar.MaxContextBytes)
		if err != nil {
			return err
		}
		pack, err = radar.ReadContext(data)
		if err != nil {
			return err
		}
	} else {
		var rules radar.DetectionRules
		if *rulesPath != "" {
			data, err := readInput(*rulesPath)
			if err != nil {
				return err
			}
			rules, err = radar.ReadDetectionRules(data)
			if err != nil {
				return err
			}
		}
		snapshots, err := collectRepositoryInputs(paths, *org, *list, rules, stderr)
		if err != nil {
			return err
		}
		pack, err = radar.BuildContext(snapshots)
		if err != nil {
			return err
		}
	}
	data, err := pack.Bytes()
	if err != nil {
		return err
	}
	if *out == "-" {
		_, err = stdout.Write(data)
		return err
	}
	if err := output.write(data); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Generated context: %s\n", *out)
	return nil
}

func readRepositoryList(data []byte) ([]repositoryInput, error) {
	var list struct {
		SchemaVersion int               `json:"schema_version"`
		Repositories  []repositoryInput `json:"repositories"`
	}
	if err := jsonutil.DecodeStrict(data, &list); err != nil {
		return nil, fmt.Errorf("repository list: %w", err)
	}
	if list.SchemaVersion != 1 || len(list.Repositories) == 0 {
		return nil, fmt.Errorf("repository list requires schema_version 1 and nonempty repositories")
	}
	for _, input := range list.Repositories {
		if (input.Path == "") == (input.GitHub == "") || (input.GitHub != "" && input.Name != "") {
			return nil, fmt.Errorf("each repository needs either path (optional name) or github")
		}
	}
	return list.Repositories, nil
}
