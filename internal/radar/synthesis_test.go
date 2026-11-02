package radar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestScanRetainsEveryEvidenceReference(t *testing.T) {
	root := t.TempDir()
	for i := range 41 {
		fixture(t, root, fmt.Sprintf("apps/app%02d/package.json", i), `{"dependencies":{"react":"19"}}`)
	}
	inv, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, inv, "react")
	if len(f.Evidence) != 41 || f.EvidenceCount != 41 {
		t.Fatalf("lost occurrence evidence: retained %d of %d", len(f.Evidence), f.EvidenceCount)
	}
}

func beamFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	fixture(t, root, "mix.exs", "defp deps do\n[\n{:phoenix, \"~> 1.8\"},\n{:phoenix_live_view, \"~> 1.0\"},\n{:ash, \"~> 3.0\"},\n{:ecto_sql, \"~> 3.0\"},\n{:oban, \"~> 2.0\"},\n{:credo, \"~> 1.0\", only: :dev}\n]\nend\n")
	fixture(t, root, "lib/app_web/endpoint.ex", "defmodule AppWeb.Endpoint do\nuse Phoenix.Endpoint, otp_app: :app\nend\n")
	fixture(t, root, "lib/app/resource.ex", "defmodule App.Resource do\nuse Ash.Resource\nend\n")
	fixture(t, root, "bin/report.py", "print('helper')\n")
	return root
}

func TestNativeSynthesisPopulatesRadarWithoutInventingTeamDecisions(t *testing.T) {
	inv, err := Scan(beamFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(inv, Policy{}, nil, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.ID == "phoenix" && ring(f) != "Adopt" {
			t.Fatalf("wired framework has no contextual proposal: %s", ring(f))
		}
		if f.ID == "oban" && ring(f) != "Assess" {
			t.Fatalf("declaration-only job framework should need assessment: %s", ring(f))
		}
		if f.Decision.Ring != "" || f.Decision.Status == "approved" {
			t.Fatal("inference became a team decision")
		}
	}
	if len(r.Decisions) != 0 {
		t.Fatal("generated proposals polluted explicit policy")
	}
	b, err := Render(r, "json")
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Components []struct{ Profile string }
		Highlights []struct {
			ID      string
			Members []string
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Components) != 2 || state.Components[0].Profile != "beam_web_data" || state.Components[1].Profile != "event_driven_worker" {
		t.Fatalf("missing native web/worker profiles: %+v", state.Components)
	}
	var grouped bool
	for _, h := range state.Highlights {
		if h.ID == "python" {
			t.Fatal("incidental helper became an architectural highlight")
		}
		if h.ID == "phoenix" && strings.Join(h.Members, ",") == "phoenix,phoenix-liveview" {
			grouped = true
		}
	}
	if !grouped {
		t.Fatal("web framework family was not grouped")
	}
}

func TestExplicitChildDecisionSurvivesGroupingAndReplay(t *testing.T) {
	inv, err := Scan(beamFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{Ring: "Caution", Scope: "Demo UI", Rationale: "Team requires migration review", Status: "approved", Owner: "Team", Reference: "PR #2", Reviewed: "2026-10-30"}
	r, err := Build(inv, Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix-liveview": d, "python": {Exclude: true, Rationale: "Helper only"}}}, nil, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(r, "json")
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Highlights       []Finding
		ExcludedFindings []Finding `json:"excluded_findings"`
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	var kept bool
	for _, h := range state.Highlights {
		if h.ID == "phoenix-liveview" && h.Decision == d && ring(h) == "Caution" {
			kept = true
		}
	}
	if !kept || len(state.ExcludedFindings) != 1 || state.ExcludedFindings[0].ID != "python" || len(state.ExcludedFindings[0].Evidence) == 0 {
		t.Fatal("grouping or exclusion discarded a team decision or its evidence")
	}
	recovered, err := ReadReport(b)
	if err != nil {
		t.Fatal(err)
	}
	next, err := Build(inv, Policy{}, &recovered, "")
	if err != nil || next.Decisions["phoenix-liveview"] != d || !next.Decisions["python"].Exclude {
		t.Fatalf("replay lost team state: %v", err)
	}
}

func TestProfilesRespectComponentAndDependencyScope(t *testing.T) {
	root := beamFixture(t)
	fixture(t, root, "scripts/package.json", `{"devDependencies":{"react":"19"}}`)
	fixture(t, root, "web/package.json", `{"dependencies":{"react":"19","@radix-ui/react-dialog":"1","unfamiliar-ui":"2"}}`)
	inv, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(inv, Policy{}, nil, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(r, "json")
	var state struct {
		Components []struct{ Path, Profile string }
		Highlights []struct {
			ID      string
			Members []string
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Components) != 3 || state.Components[0].Path != "." || state.Components[0].Profile != "beam_web_data" || state.Components[1].Path != "." || state.Components[1].Profile != "event_driven_worker" || state.Components[2].Path != "web" || state.Components[2].Profile != "react_typescript_web" {
		t.Fatalf("profile crossed component or dev-only boundaries: %+v", state.Components)
	}
	var react bool
	for _, h := range state.Highlights {
		if h.ID == "react" && strings.Join(h.Members, ",") == "npm:@radix-ui/react-dialog,react" {
			react = true
		}
	}
	if !react || finding(t, inv, "npm:unfamiliar-ui").Presence != "detected" {
		t.Fatal("native UI grouping lost membership or unknown evidence")
	}
}

func TestRootApprovalDoesNotApproveGroupedChildren(t *testing.T) {
	inv, err := Scan(beamFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{Ring: "Adopt", Scope: "Phoenix endpoint only", Rationale: "Reviewed endpoint", Status: "approved", Owner: "Team", Reference: "PR #9", Reviewed: "2026-10-30"}
	r, err := Build(inv, Policy{SchemaVersion: 1, Rules: map[string]Decision{"phoenix": d}}, nil, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(r, "json")
	var state struct {
		Highlights []struct {
			ID       string
			Members  []string
			Decision Decision
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	var liveview bool
	for _, h := range state.Highlights {
		if h.ID == "phoenix" && len(h.Members) != 1 {
			t.Fatal("group extended endpoint-only approval to other technologies")
		}
		if h.ID == "phoenix-liveview" && h.Decision.Status != "approved" {
			liveview = true
		}
	}
	if !liveview {
		t.Fatal("unapproved child hidden under approved root")
	}
}

func TestNativeQualityToolsFormOneExplainableFamily(t *testing.T) {
	root := beamFixture(t)
	fixture(t, root, "tools/mix.exs", "defp deps do\n[\n{:phoenix, \"1\"},\n{:credo, \"1\", only: :dev},\n{:dialyxir, \"1\", only: :dev},\n{:sobelow, \"1\", only: :dev}\n]\nend\n")
	inv, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(inv, Policy{}, nil, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(r, "json")
	var state struct {
		Highlights []struct {
			ID      string
			Members []string
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	for _, h := range state.Highlights {
		if h.ID == "credo" && strings.Join(h.Members, ",") == "credo,dialyzer,sobelow" {
			return
		}
	}
	t.Fatal("quality packages still presented as independent architectural choices")
}

func TestReactBuildDependenciesDoNotDemoteAuthoredLanguageAndStyles(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19"},"devDependencies":{"typescript":"5","tailwindcss":"3"}}`)
	fixture(t, root, "src/app.tsx", "import React from 'react';\nexport const App = () => <main>Hello</main>;\n")
	fixture(t, root, "src/index.css", "@tailwind base;\n@tailwind components;\n@tailwind utilities;\n")
	inv, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(inv, Policy{}, nil, "Web")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(r, "json")
	var state struct {
		Findings []struct {
			ID         string
			Assessment struct{ Significance string }
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	for _, f := range state.Findings {
		if (f.ID == "typescript" || f.ID == "tailwind") && f.Assessment.Significance != "system_defining" {
			t.Fatalf("native frontend build idiom hid architectural choice %s", f.ID)
		}
	}
}

func TestUnrecognizedEcosystemIsNotAutomaticallySupportingCode(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "app.py", "def main():\n    print('service')\n")
	inv, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(inv, Policy{}, nil, "Unknown")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(r, "json")
	var state struct {
		Findings []struct {
			ID         string
			Assessment struct{ Significance string }
		}
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Findings) != 1 || state.Findings[0].ID != "python" || state.Findings[0].Assessment.Significance != "unknown" {
		t.Fatal("unsupported ecosystem was demoted using another ecosystem's assumptions")
	}
}
