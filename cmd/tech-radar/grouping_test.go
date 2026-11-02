package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExplicitGroupingInputAndReplay(t *testing.T) {
	repo, work := t.TempDir(), t.TempDir()
	write(t, repo, "package.json", `{"dependencies":{"example-core":"1","example-plugin":"1"}}`)
	groups := write(t, repo, "grouping.py", `{"schema_version":1,"families":[{"id":"example-framework","name":"Example framework","quadrant":"Languages & Frameworks","significance":"system_defining","relationship":"framework_extensions","scope":"component","members":["npm:example-core","npm:example-plugin"]}]}`)
	data := call(t, 0, "scan", "--repo", repo, "--grouping", groups)
	var report struct {
		Source struct {
			Files               int
			GroupingFingerprint string `json:"grouping_fingerprint"`
		}
		Highlights []struct {
			Name     string
			FamilyID string `json:"family_id"`
		}
	}
	if err := json.Unmarshal([]byte(data), &report); err != nil {
		t.Fatal(err)
	}
	if report.Source.Files != 1 || len(report.Source.GroupingFingerprint) != 64 || len(report.Highlights) != 1 || report.Highlights[0].Name != "Example framework" || report.Highlights[0].FamilyID != "example-framework" {
		t.Fatal("grouping input was not applied, fingerprinted, or excluded from evidence")
	}
	previous := write(t, work, "previous.json", data)
	call(t, 1, "scan", "--repo", repo, "--previous", previous)
	if got := call(t, 0, "scan", "--repo", repo, "--grouping", groups, "--previous", previous); got != data {
		t.Fatal("explicit grouping did not replay deterministically")
	}
	call(t, 1, "generate", "--repo", repo, "--grouping", groups, "--out", groups, "--force")
	invalid := write(t, work, "invalid.json", `{"schema_version":1,"families":[{"id":"x","name":"X","quadrant":"Tools","significance":"workflow_enabling","relationship":"workflow_tools","scope":"component","members":["npm:example-core"],"status":"approved"}]}`)
	call(t, 1, "scan", "--repo", repo, "--grouping", invalid)
	clear := write(t, work, "clear.json", `{"schema_version":1,"families":[]}`)
	without := call(t, 0, "scan", "--repo", repo, "--grouping", clear, "--previous", previous)
	if strings.Contains(without, `"name": "Example framework"`) {
		t.Fatal("removing runtime grouping retained its derived family")
	}
}
