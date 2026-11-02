package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tech-radar/internal/radar"
)

func TestPluginVersionsMatchCLI(t *testing.T) {
	files := []struct {
		path   string
		plugin string
	}{
		{path: "plugin/tech-radar/plugin.json"},
		{path: "plugin/tech-radar/.codex-plugin/plugin.json"},
		{path: "plugin/tech-radar/.claude-plugin/plugin.json"},
		{path: ".claude-plugin/marketplace.json", plugin: "tech-radar"},
	}
	for _, file := range files {
		t.Run(file.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file.path)))
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				Version string `json:"version"`
				Plugins []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"plugins"`
			}
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			version := metadata.Version
			if file.plugin != "" {
				matches := 0
				for _, plugin := range metadata.Plugins {
					if plugin.Name == file.plugin {
						version = plugin.Version
						matches++
					}
				}
				if matches != 1 {
					t.Fatalf("expected one %q marketplace entry, got %d", file.plugin, matches)
				}
			}
			if version != radar.Version {
				t.Errorf("version = %q; must match CLI version %q", version, radar.Version)
			}
		})
	}
}
