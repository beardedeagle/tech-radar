package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestTopLevelHelpAndOutputFailures(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}, {"version"}} {
		var out, stderr bytes.Buffer
		if code := run(args, &out, &stderr); code != 0 || out.Len() == 0 {
			t.Fatalf("%v: code %d, output %q, error %q", args, code, out.String(), stderr.String())
		}
		if code := run(args, failedOutput{}, io.Discard); code != 1 {
			t.Fatalf("%v ignored a failed stdout write", args)
		}
	}
}

func TestDiscoverRejectsDuplicatePathsBeforeScanning(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", "not valid JSON")
	var out, stderr bytes.Buffer
	if code := run([]string{"discover", "--repo", root, "--repo", root + string(filepath.Separator) + "."}, &out, &stderr); code != 1 || !strings.Contains(stderr.String(), "duplicate --repo") {
		t.Fatalf("duplicate selection reached the scanner: code %d, %s", code, stderr.String())
	}
}

func TestOutputPublicationRejectsAnEditAfterValidation(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, repo, "go.mod", "module example.test/repo\n\ngo 1.26.4\n")
	name := filepath.Join(root, "radar.json")
	call(t, 0, "scan", "--repo", repo, "--out", name)
	abs, prior, err := prepareOutput(name, true)
	if err != nil {
		t.Fatal(err)
	}
	tmp := write(t, root, "staged.json", "replacement")
	if err := os.WriteFile(name, []byte("human edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishOutput(tmp, abs, prior); err == nil {
		t.Fatal("published over an edit made after validation")
	}
	got, err := os.ReadFile(name)
	if err != nil || string(got) != "human edit" {
		t.Fatalf("lost manual edit: %q, %v", got, err)
	}
}

func TestOutputPublicationDoesNotClobberANewDestination(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "radar.json")
	abs, prior, err := prepareOutput(name, true)
	if err != nil {
		t.Fatal(err)
	}
	tmp := write(t, root, "staged.json", "replacement")
	if err := os.WriteFile(name, []byte("human file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishOutput(tmp, abs, prior); !os.IsExist(err) {
		t.Fatalf("expected no-clobber failure, got %v", err)
	}
	got, err := os.ReadFile(name)
	if err != nil || string(got) != "human file" {
		t.Fatalf("lost newly created file: %q, %v", got, err)
	}
}
