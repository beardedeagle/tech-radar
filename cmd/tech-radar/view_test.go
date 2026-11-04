package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tech-radar/internal/radar"
)

func TestCLIViewSelectionAndPortableReplay(t *testing.T) {
	for _, command := range []string{"scan", "generate"} {
		for _, model := range []string{"recommendation", "lifecycle"} {
			for _, view := range []string{"", "radar", "table"} {
				// Both commands cover model defaults; explicit formats share the
				// same view and replay pipeline, covered below by generate.
				if command == "scan" && view != "" {
					continue
				}
				t.Run(command+"/"+model+"/"+view, func(t *testing.T) {
					repo, work := t.TempDir(), t.TempDir()
					write(t, repo, "package.json", `{"dependencies":{"react":"19"}}`)
					args := []string{command, "--repo", repo, "--model", model, "--format", "json"}
					if view != "" {
						args = append(args, "--view", view)
					}
					first := call(t, 0, args...)
					r, err := radar.ReadReport([]byte(first))
					want := view
					if want == "" {
						want = "radar"
						if model == "lifecycle" {
							want = "table"
						}
					}
					if err != nil || string(r.View) != want {
						t.Fatalf("wrong view: %q, %v", r.View, err)
					}
					previous := write(t, work, "first.json", first)
					next := call(t, 0, command, "--repo", repo, "--previous", previous, "--format", "json")
					r, err = radar.ReadReport([]byte(next))
					if err != nil || string(r.View) != want {
						t.Fatal("omitted CLI view failed to inherit")
					}
					dest := filepath.Join(work, "report.html")
					previous = write(t, work, "next.json", next)
					call(t, 0, command, "--repo", repo, "--previous", previous, "--format", "html", "--out", dest)
					withCompanion, err := readPrevious(dest)
					if err != nil || !reflect.DeepEqual(r, withCompanion) {
						t.Fatal("companion lost selected view or state")
					}
					data, _ := os.ReadFile(dest)
					compact, err := radar.ReadReport(data)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(filepath.Join(work, compact.EvidenceFile)); err != nil {
						t.Fatal(err)
					}
					call(t, 0, command, "--repo", repo, "--previous", dest, "--format", "html", "--out", dest, "--force")
					again, _ := os.ReadFile(dest)
					if !bytes.Equal(data, again) {
						t.Fatal("missing companion replay changed table/radar output")
					}
				})
			}
		}
	}
}

func TestCLIRejectsInvalidViewAndKeepsFormatSeparate(t *testing.T) {
	repo := t.TempDir()
	write(t, repo, "go.mod", "module example.test/app\n\ngo 1.26.7\n")
	for _, command := range []string{"scan", "generate"} {
		call(t, 1, command, "--repo", repo, "--view", "grid")
		call(t, 1, command, "--repo", repo, "--format", "table")
	}
	if !strings.Contains(call(t, 0, "--help"), "--view radar|table") {
		t.Fatal("view selection absent from CLI help")
	}
}
