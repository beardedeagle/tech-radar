package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"tech-radar/internal/radar"
)

// checkCommand evaluates explicit policy against current or frozen observations.
// Collection/input/output failures are errors; violations/gaps are exit results.
func checkCommand(args []string, stdout, stderr io.Writer) (code int, err error) {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	var paths repoPaths
	flags.Var(&paths, "repo", "inspect local repository at `PATH` (repeatable)")
	org := flags.String("org", "", "inspect accessible repositories in GitHub organization `NAME`")
	list := flags.String("repos", "", "read repositories from schema-1 JSON `FILE`")
	from := flags.String("from", "", "check saved context or check JSON `FILE` without rescanning")
	policyPath := flags.String("policy", "", "read approved enforcement policy from JSON `FILE` (required)")
	exceptionsPath := flags.String("exceptions", "", "read approved, policy-bound exceptions from JSON `FILE`")
	baselinePath := flags.String("baseline", "", "compare with previous context or check JSON `FILE` for drift and existing-only rules")
	rulesPath := flags.String("rules", "", "add detection patterns from JSON `FILE`")
	asOf := flags.String("as-of", time.Now().UTC().Format(time.DateOnly), "evaluate approvals on `YYYY-MM-DD` (UTC today unless supplied)")
	out := flags.String("out", "-", "write check JSON to `FILE`, or - for stdout")
	force := flags.Bool("force", false, "replace an unedited generated export")
	if err := parseCommandFlags(flags, args, stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 1, err
	}
	selectors := 0
	for _, yes := range []bool{len(paths) > 0, *org != "", *list != "", *from != ""} {
		if yes {
			selectors++
		}
	}
	if selectors != 1 || flags.NArg() != 0 || *policyPath == "" {
		return 1, fmt.Errorf("check requires --policy and exactly one of --repo, --org, --repos or --from")
	}
	if *from != "" && *rulesPath != "" {
		return 1, fmt.Errorf("--from uses saved observations; supply --rules during capture")
	}
	if _, err := time.Parse(time.DateOnly, *asOf); err != nil {
		return 1, fmt.Errorf("as-of must be YYYY-MM-DD")
	}
	var output *outputFile
	if *out != "-" {
		if strings.ToLower(filepath.Ext(*out)) != ".json" {
			return 1, fmt.Errorf("check output requires a .json filename")
		}
		output, err = beginOutput(*out, *force, *policyPath, *exceptionsPath, *rulesPath, *list)
		if err != nil {
			return 1, err
		}
		defer func() {
			if closeErr := output.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
				code = 1
			}
		}()
	}
	var ignored [][32]byte
	data, err := readInputLimit(*policyPath, radar.MaxFileBytes)
	if err != nil {
		return 1, err
	}
	ignored = append(ignored, sha256.Sum256(data))
	policy, err := radar.ReadCheckPolicy(data)
	if err != nil {
		return 1, err
	}
	options := radar.CheckOptions{AsOf: *asOf}
	if *exceptionsPath != "" {
		data, err := readInputLimit(*exceptionsPath, radar.MaxFileBytes)
		if err != nil {
			return 1, err
		}
		ignored = append(ignored, sha256.Sum256(data))
		options.Exceptions, err = radar.ReadCheckExceptions(data, policy)
		if err != nil {
			return 1, err
		}
	}
	if *baselinePath != "" {
		data, err := readInputLimit(*baselinePath, radar.MaxContextBytes)
		if err != nil {
			return 1, err
		}
		ignored = append(ignored, sha256.Sum256(data))
		options.Baseline, err = readCheckSnapshots(data)
		if err != nil {
			return 1, fmt.Errorf("baseline: %w", err)
		}
	}
	if *list != "" {
		data, err := readInput(*list)
		if err != nil {
			return 1, err
		}
		ignored = append(ignored, sha256.Sum256(data))
	}
	var snapshots []radar.RepositorySnapshot
	if *from != "" {
		data, err := readInputLimit(*from, radar.MaxContextBytes)
		if err != nil {
			return 1, err
		}
		snapshots, err = readCheckSnapshots(data)
		if err != nil {
			return 1, err
		}
	} else {
		var rules radar.DetectionRules
		if *rulesPath != "" {
			data, err := readInput(*rulesPath)
			if err != nil {
				return 1, err
			}
			rules, err = radar.ReadDetectionRules(data)
			if err != nil {
				return 1, err
			}
		}
		snapshots, err = collectRepositoryInputs(paths, *org, *list, rules, stderr, ignored...)
		if err != nil {
			return 1, err
		}
		// Apply the same snapshot validation and duplicate-identity checks as discover.
		pack, err := radar.BuildContext(snapshots)
		if err != nil {
			return 1, err
		}
		snapshots = pack.Repositories()
	}
	result, err := radar.Check(snapshots, policy, options)
	if err != nil {
		return 1, err
	}
	data, err = result.Bytes()
	if err != nil {
		return 1, err
	}
	if output == nil {
		_, err = stdout.Write(data)
	} else {
		err = output.write(data)
	}
	if err != nil {
		return 1, err
	}
	return result.ExitCode(), nil
}

func readCheckSnapshots(data []byte) ([]radar.RepositorySnapshot, error) {
	if result, err := radar.ReadCheckResult(data); err == nil {
		snapshots := make([]radar.RepositorySnapshot, 0, len(result.Repositories))
		for _, repo := range result.Repositories {
			snapshots = append(snapshots, repo.Snapshot)
		}
		return snapshots, nil
	}
	if pack, err := radar.ReadContext(data); err == nil {
		return pack.Repositories(), nil
	}
	return nil, fmt.Errorf("expected an intact generated check result or discovery context")
}
