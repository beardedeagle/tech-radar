package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestOutputLockPrecedesPreviousAndFromReads(t *testing.T) {
	for _, command := range []string{"scan", "discover"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			repo := filepath.Join(root, "repo")
			write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
			name := filepath.Join(root, "output.json")
			call(t, 0, command, "--repo", repo, "--out", name)
			before, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			lock := flock.New(name+".lock", flock.SetFlag(os.O_CREATE|os.O_RDWR))
			locked, err := lock.TryLock()
			if err != nil || !locked {
				t.Fatalf("hold output lock: %v", err)
			}
			t.Cleanup(func() { _ = lock.Close() })
			broken := write(t, root, "broken.json", "invalid saved state")
			args := []string{command, "--out", name, "--force"}
			if command == "scan" {
				args = append(args, "--repo", repo, "--previous", broken)
			} else {
				args = append(args, "--from", broken)
			}
			var stderr bytes.Buffer
			if code := run(args, io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), "another tech-radar run") {
				t.Fatalf("read saved state before rejecting contention: code %d: %s", code, stderr.String())
			}
			after, err := os.ReadFile(name)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("contending run changed the export")
			}
		})
	}
}

// Use an empty remote repo to test discovery without an archive.
func emptyRemote(t *testing.T, duringRead func()) string {
	t.Helper()
	t.Setenv("GH_TOKEN", "fixture-token")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	called := false
	http.DefaultTransport = signalTransport(func(r *http.Request) (*http.Response, error) {
		if !called {
			called = true
			duringRead()
		}
		status, body := 200, `{"full_name":"example/service","default_branch":"main"}`
		if strings.Contains(r.URL.Path, "/commits/") {
			status, body = 409, `{"message":"Git Repository is empty."}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Request: r, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return write(t, t.TempDir(), "repos.json", `{"schema_version":1,"repositories":[{"github":"example/service"}]}`)
}

func TestDiscoveryRetainsEarlyOutputBaseline(t *testing.T) {
	for _, initial := range []string{"absent", "existing"} {
		t.Run(initial, func(t *testing.T) {
			root := t.TempDir()
			repo := filepath.Join(root, "repo")
			write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
			name := filepath.Join(root, "context.json")
			if initial == "existing" {
				call(t, 0, "discover", "--repo", repo, "--out", name)
			}
			other := filepath.Join(root, "other")
			write(t, other, "package.json", `{"dependencies":{"react":"19"}}`)
			newer := call(t, 0, "discover", "--repo", other)
			list := emptyRemote(t, func() {
				if err := os.WriteFile(name, []byte(newer), 0600); err != nil {
					t.Fatal(err)
				}
			})
			var stderr bytes.Buffer
			if code := run([]string{"discover", "--repos", list, "--out", name, "--force"}, io.Discard, &stderr); code != 1 {
				t.Fatalf("accepted a later baseline: code %d: %s", code, stderr.String())
			}
			after, err := os.ReadFile(name)
			if err != nil || string(after) != newer {
				t.Fatal("overwrote a valid export changed during discovery")
			}
		})
	}
}

func TestDiscoveryHoldsOutputLockDuringCollection(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
	name := filepath.Join(root, "context.json")
	call(t, 0, "discover", "--repo", repo, "--out", name)
	list := emptyRemote(t, func() {
		var stderr bytes.Buffer
		if code := run([]string{"discover", "--from", name, "--out", name, "--force"}, io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), "another tech-radar run") {
			t.Errorf("concurrent rebuild was not excluded: code %d: %s", code, stderr.String())
		}
	})
	call(t, 0, "discover", "--repos", list, "--out", name, "--force")
}

func TestOutputLockSurvivesReleaseAndFailure(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
	name := filepath.Join(root, "radar.json")
	broken := write(t, root, "broken.json", "invalid saved state")
	call(t, 1, "scan", "--repo", repo, "--previous", broken, "--out", name)
	first, err := os.Lstat(name + ".lock")
	if err != nil || !first.Mode().IsRegular() {
		t.Fatalf("failed run did not retain a regular sidecar: %v", err)
	}
	pinned, err := os.Open(name + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	call(t, 0, "scan", "--repo", repo, "--out", name)
	call(t, 0, "scan", "--repo", repo, "--previous", name, "--out", name, "--force")
	after, err := os.Lstat(name + ".lock")
	if err != nil || !os.SameFile(first, after) {
		t.Fatal("release replaced the stable lock inode")
	}
}

func TestOutputRejectsSymlinkLock(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
	name := filepath.Join(root, "radar.json")
	target := write(t, root, "notes.txt", "keep these notes")
	if err := os.Symlink(target, name+".lock"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	call(t, 1, "scan", "--repo", repo, "--out", name)
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("published despite a symlink lock")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep these notes" {
		t.Fatal("modified the lock symlink target")
	}
}

func TestOutputLockChild(t *testing.T) {
	if os.Getenv("TECH_RADAR_LOCK_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	list := emptyRemote(t, func() {
		fmt.Fprintln(os.Stdout, "LOCK_READY")
		_, _ = io.Copy(io.Discard, os.Stdin)
	})
	os.Exit(run([]string{"discover", "--repos", list, "--out", os.Getenv("TECH_RADAR_LOCK_OUT")}, os.Stdout, os.Stderr))
}

func TestOutputLockReleasedAfterProcessDeath(t *testing.T) {
	name := filepath.Join(t.TempDir(), "context.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOutputLockChild$")
	cmd.Env = append(os.Environ(), "TECH_RADAR_LOCK_CHILD=1", "TECH_RADAR_LOCK_OUT="+name,
		"TMPDIR="+filepath.Dir(name), "TMP="+filepath.Dir(name), "TEMP="+filepath.Dir(name))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	lines := bufio.NewScanner(out)
	if !lines.Scan() || lines.Text() != "LOCK_READY" {
		_ = cmd.Wait()
		t.Fatalf("child did not start collection: %s", stderr.String())
	}
	lock := flock.New(name+".lock", flock.SetFlag(os.O_CREATE|os.O_RDWR))
	t.Cleanup(func() { _ = lock.Close() })
	locked, err := lock.TryLock()
	if err != nil || locked {
		t.Fatalf("child did not hold the output lock: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	locked, err = lock.TryLock()
	if err != nil || !locked {
		t.Fatalf("process death left stale lock ownership: %v", err)
	}
}
