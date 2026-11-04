package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestCommandHelpRoutesAndOutputFailures(t *testing.T) {
	for _, command := range []string{"generate", "scan", "discover", "check", "catalog", "version"} {
		t.Run(command, func(t *testing.T) {
			var expected string
			for _, args := range [][]string{{command, "--help"}, {command, "-h"}, {"help", command}} {
				var out, stderr bytes.Buffer
				if code := run(args, &out, &stderr); code != 0 || stderr.Len() != 0 {
					t.Fatalf("%v: code=%d stderr=%q", args, code, stderr.String())
				}
				if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "Examples:") {
					t.Fatalf("%v has no command guidance: %s", args, out.String())
				}
				if expected != "" && out.String() != expected {
					t.Fatalf("%v differs from --help", args)
				}
				expected = out.String()
				if code := run(args, failedOutput{}, io.Discard); code != 1 {
					t.Fatalf("%v ignored a failed help write", args)
				}
			}
		})
	}
}

func TestQuickStartHelpExamplesGenerateReadableReports(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	write(t, root, "package.json", `{"dependencies":{"react":"19"}}`)
	var help bytes.Buffer
	if code := run([]string{"--help"}, &help, io.Discard); code != 0 {
		t.Fatalf("help exit: %d", code)
	}
	examples := 0
	for _, line := range strings.Split(help.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "tech-radar generate ") {
			continue
		}
		args := strings.Fields(line)[1:]
		var stderr bytes.Buffer
		if code := run(args, io.Discard, &stderr); code != 0 {
			t.Fatalf("example failed: %s: %s", line, stderr.String())
		}
		var output string
		for i, arg := range args {
			if arg == "--out" && i+1 < len(args) {
				output = args[i+1]
			}
		}
		if report, err := readPrevious(output); err != nil || len(report.Findings) == 0 {
			t.Fatalf("example output cannot be used for the next run: %s: %v", line, err)
		}
		examples++
	}
	if examples != 2 {
		t.Fatalf("expected recommendation and lifecycle quick starts, got %d", examples)
	}
}

func TestInvalidFlagsPointToCommandHelp(t *testing.T) {
	for _, command := range []string{"generate", "scan", "discover", "check"} {
		var out, stderr bytes.Buffer
		if code := run([]string{command, "--not-an-option"}, &out, &stderr); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "tech-radar "+command+" --help") {
			t.Fatalf("%s error has no help direction: %s", command, stderr.String())
		}
	}
}
