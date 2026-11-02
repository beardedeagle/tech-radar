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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

type signalTransport func(*http.Request) (*http.Response, error)

func (f signalTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type pendingArchive struct{ context.Context }

func (b pendingArchive) Read([]byte) (int, error) { <-b.Done(); return 0, b.Err() }
func (pendingArchive) Close() error               { return nil }

func TestDiscoverySignalChild(t *testing.T) {
	if os.Getenv("TECH_RADAR_SIGNAL_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	http.DefaultTransport = signalTransport(func(r *http.Request) (*http.Response, error) {
		body := io.ReadCloser(io.NopCloser(strings.NewReader(`{"full_name":"example/service","default_branch":"main"}`)))
		if strings.Contains(r.URL.Path, "/commits/") {
			body = io.NopCloser(strings.NewReader(`{"sha":"` + strings.Repeat("a", 40) + `"}`))
		}
		if strings.Contains(r.URL.Path, "/tarball/") {
			body = pendingArchive{r.Context()}
			fmt.Fprintln(os.Stdout, "CAPTURE_STARTED")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Request: r, Body: body}, nil
	})
	os.Exit(run([]string{"discover", "--repos", os.Getenv("TECH_RADAR_SIGNAL_LIST")}, os.Stdout, os.Stderr))
}

func TestDiscoverySIGTERMCleansCapturedSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not deliver Unix SIGTERM")
	}
	root := t.TempDir()
	list := write(t, root, "repos.json", `{"schema_version":1,"repositories":[{"github":"example/service"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDiscoverySignalChild$")
	cmd.Env = append(os.Environ(), "TECH_RADAR_SIGNAL_CHILD=1", "TECH_RADAR_SIGNAL_LIST="+list, "TMPDIR="+root, "GH_CONFIG_DIR="+root, "GH_TOKEN=fixture-token", "GITHUB_TOKEN=")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	lines := bufio.NewScanner(out)
	ready := false
	for lines.Scan() {
		if lines.Text() == "CAPTURE_STARTED" {
			ready = true
			break
		}
	}
	if !ready {
		cmd.Wait()
		t.Fatalf("capture did not start: %s", stderr.String())
	}
	dirs, err := filepath.Glob(filepath.Join(root, "tech-radar-discovery-*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("capture directory missing: %v, %v", dirs, err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("expected graceful cancellation, got %v: %s", err, stderr.String())
	}
	if _, err := os.Stat(dirs[0]); !os.IsNotExist(err) {
		t.Fatalf("captured source remains after SIGTERM: %v", err)
	}
}
