package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLocalContextRebuildAndRepositoryUse(t *testing.T) {
	work := t.TempDir()
	beam := filepath.Join(work, "web")
	python := filepath.Join(work, "inference")
	write(t, beam, "mix.exs", "defp deps do\n[\n{:phoenix, \"1.8\"},\n{:ash, \"3\"}\n]\nend\n")
	write(t, python, "pyproject.toml", "[project]\ndependencies = [\n \"fastapi>=0.100\",\n \"langgraph>=1\",\n \"unfamiliar-package>=2\"\n]\n")
	write(t, python, "app.py", "API_KEY = 'DO_NOT_EXPORT_SOURCE_VALUES'\n")
	pack := filepath.Join(work, "context.json")
	call(t, 0, "discover", "--repo", beam, "--repo", python, "--out", pack)
	data, err := os.ReadFile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(work)) || bytes.Contains(data, []byte("DO_NOT_EXPORT_SOURCE_VALUES")) {
		t.Fatal("context exported local paths or raw source values")
	}
	var state struct {
		Kind        string
		Fingerprint string
		Snapshots   []struct {
			ID    string
			Files []struct{ Path, SHA256 string }
		}
		Profiles []struct{ ID string }
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Kind != "tech-radar-context" || len(state.Fingerprint) != 64 || len(state.Snapshots) != 2 || len(state.Snapshots[0].Files) == 0 {
		t.Fatal("missing frozen inputs/provenance")
	}
	profiles := map[string]bool{}
	for _, p := range state.Profiles {
		profiles[p.ID] = true
	}
	if !profiles["beam_web_data"] || !profiles["python_ai_api"] {
		t.Fatal("native context profiles not compiled")
	}
	second := call(t, 0, "discover", "--repo", python, "--repo", beam)
	if !bytes.Equal(data, []byte(second)) {
		t.Fatal("input order changed compiled context")
	}
	// Rebuilding still works after the original repo is removed.
	if err := os.RemoveAll(python); err != nil {
		t.Fatal(err)
	}
	if got := call(t, 0, "discover", "--from", pack); got != string(data) {
		t.Fatal("frozen context did not rebuild identically")
	}
	write(t, python, "pyproject.toml", "[project]\ndependencies = [\"fastapi>=0.100\", \"langgraph>=1\"]\n")
	with := call(t, 0, "scan", "--repo", python, "--context", pack)
	if !strings.Contains(with, `"id": "fastapi"`) || !strings.Contains(with, `"context_fingerprint"`) {
		t.Fatal("compiled context was not applied to the repository")
	}
	without := call(t, 0, "scan", "--repo", python)
	if !strings.Contains(without, `"id": "fastapi"`) || !strings.Contains(without, `"profile": "python_ai_api"`) {
		t.Fatal("standalone scan still requires a context to understand Python")
	}
	report := filepath.Join(work, "radar.html")
	call(t, 0, "generate", "--repo", python, "--context", pack, "--format", "html", "--out", report)
	html, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("FastAPI")) || !bytes.Contains(html, []byte("Assess")) {
		t.Fatal("context report has no native radar proposal")
	}
	call(t, 1, "scan", "--repo", python, "--previous", report)
	call(t, 0, "scan", "--repo", python, "--context", pack, "--previous", report)
	call(t, 1, "scan", "--repo", python, "--context", pack, "--out", pack, "--force")
	// Profiles still have to match the repo being scanned.
	call(t, 0, "scan", "--repo", beam, "--context", pack)
}

func TestDiscoverListAndSafeContextOutput(t *testing.T) {
	work := t.TempDir()
	repo := filepath.Join(work, "service")
	write(t, repo, "go.mod", "module example.test/service\n\ngo 1.26.4\n")
	list := write(t, work, "repositories.json", `{"schema_version":1,"repositories":[{"path":"service","name":"example-service"}]}`)
	pack := filepath.Join(work, "context.json")
	call(t, 0, "discover", "--repos", list, "--out", pack)
	call(t, 1, "discover", "--repos", list, "--out", pack)
	call(t, 0, "discover", "--from", pack, "--out", pack, "--force")
	human := write(t, work, "human.json", `{"notes":"keep"}`)
	call(t, 1, "discover", "--repos", list, "--out", human, "--force")
	call(t, 1, "discover", "--repos", list, "--out", list, "--force")
	call(t, 1, "discover", "--repos", list, "--repo", repo)
	duplicate := write(t, work, "duplicate.json", `{"schema_version":1,"repositories":[{"path":"service","path":"other"}]}`)
	call(t, 1, "discover", "--repos", duplicate)
	b, _ := os.ReadFile(human)
	if string(b) != `{"notes":"keep"}` {
		t.Fatal("overwrote human data")
	}
}
