package radar

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGitHeadLookupUsesSelectedRepository(t *testing.T) {
	outer := t.TempDir()
	selected := t.TempDir()
	for _, root := range []string{outer, selected} {
		runFixtureGit(t, root, "init", "--quiet")
		runFixtureGit(t, root, "commit", "--quiet", "--allow-empty", "-m", root)
	}
	selectedHead := runFixtureGit(t, selected, "rev-parse", "HEAD")
	outerHead := runFixtureGit(t, outer, "rev-parse", "HEAD")
	if selectedHead == outerHead {
		t.Fatal("fixture repositories must have different revisions")
	}
	linked := filepath.Join(t.TempDir(), "linked worktree")
	runFixtureGit(t, selected, "worktree", "add", "--quiet", "-b", "linked", linked)
	runFixtureGit(t, linked, "commit", "--quiet", "--allow-empty", "-m", "linked worktree")
	linkedHead := runFixtureGit(t, linked, "rev-parse", "HEAD")

	// Fixture mutations above use their own environment even when this test starts in a hook.
	for name, value := range map[string]string{
		"GIT_DIR":              filepath.Join(outer, ".git"),
		"GIT_WORK_TREE":        outer,
		"GIT_COMMON_DIR":       filepath.Join(outer, ".git"),
		"GIT_INDEX_FILE":       filepath.Join(outer, ".git", "index"),
		"GIT_OBJECT_DIRECTORY": filepath.Join(outer, ".git", "objects"),
		"GIT_PREFIX":           "outer/",
		"GIT_CONFIG_COUNT":     "1",
		"GIT_CONFIG_KEY_0":     "core.bare",
		"GIT_CONFIG_VALUE_0":   "true",
	} {
		if runtime.GOOS == "windows" {
			name = strings.ToLower(name)
		}
		t.Setenv(name, value)
	}
	for _, tc := range []struct {
		name string
		root string
		head string
	}{
		{"repository", selected, selectedHead},
		{"linked worktree", linked, linkedHead},
		{"non-Git directory", t.TempDir(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if head := readGitHead(ctx, tc.root); head != tc.head {
				t.Errorf("Git HEAD = %q, want selected root's %q (outer HEAD %q)", head, tc.head, outerHead)
			}
			inv, files, err := CaptureRepository(tc.root, DetectionRules{})
			if err != nil {
				t.Fatal(err)
			}
			if inv.Source.GitHead != tc.head {
				t.Errorf("captured Git HEAD = %q, want %q", inv.Source.GitHead, tc.head)
			}
			_, err = BuildContext([]RepositorySnapshot{{
				ID: "local:fixture", Name: "fixture", Origin: "local", Status: "captured",
				Revision: tc.head, Inventory: inv, Files: files,
			}})
			if err != nil {
				t.Fatalf("captured revision cannot build context: %v", err)
			}
		})
	}
	if os.Getenv("GIT_DIR") != filepath.Join(outer, ".git") {
		t.Fatal("metadata lookup changed the process Git environment")
	}
}

func runFixtureGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args = append([]string{"-C", root, "-c", "core.hooksPath=", "-c", "commit.gpgsign=false", "-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Env = []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(value), "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture Git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestGitHeadLookupHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a Unix executable fixture")
	}
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"local environment", "#!/bin/sh\nexec sleep 30\n"},
		{"HEAD", "#!/bin/sh\nif [ \"$1\" = rev-parse ]; then printf 'GIT_DIR\\n'; exit 0; fi\nexec sleep 30\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(tc.script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			if head := readGitHead(ctx, t.TempDir()); head != "" {
				t.Fatalf("unexpected revision: %s", head)
			}
			if time.Since(started) > time.Second {
				t.Fatal("Git metadata lookup ignored cancellation")
			}
		})
	}
}
