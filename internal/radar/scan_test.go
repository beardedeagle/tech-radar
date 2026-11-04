package radar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func fixture(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitModuleRulesSupportDifferentApplicationNamespaces(t *testing.T) {
	for _, namespace := range []string{"Example", "Example.Connectors"} {
		t.Run(namespace, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, "lib/connectors.ex", fmt.Sprintf("%[1]s.HubSpot.call()\n%[1]s.MyUtilities.call()\n%[1]s.Bindable.call()\n%[1]s.LocalLogic.call()\n%[1]s.Mapbox.call()\n%[1]s.Smarty.call()\n%[1]s.Auth.Signer.Kms.sign()\n", namespace))
			fixture(t, root, "docs/feature_flags.md", namespace+".FeatureFlags enables actor-scoped flags.\n")
			var patterns []detectionPattern
			for _, mapping := range [][2]string{{"hubspot", "HubSpot"}, {"myutilities", "MyUtilities"}, {"bindable", "Bindable"}, {"local-logic", "LocalLogic"}, {"mapbox", "Mapbox"}, {"smarty", "Smarty"}, {"aws-kms", "Auth.Signer.Kms"}, {"feature-flags", "FeatureFlags"}} {
				patterns = append(patterns, detectionPattern{ID: mapping[0], Pattern: `\b` + regexp.QuoteMeta(namespace+"."+mapping[1]) + `\b`, Detail: "Configured integration reference", Documentation: mapping[0] == "feature-flags"})
			}
			data, err := json.Marshal(struct {
				SchemaVersion int                `json:"schema_version"`
				Patterns      []detectionPattern `json:"patterns"`
			}{1, patterns})
			if err != nil {
				t.Fatal(err)
			}
			rules, err := ReadDetectionRules(data)
			if err != nil {
				t.Fatal(err)
			}
			inv := scanTestRepository(t, root, ScanOptions{Rules: rules})
			for _, id := range []string{"hubspot", "myutilities", "bindable", "local-logic", "mapbox", "smarty", "aws-kms", "feature-flags"} {
				f := finding(t, inv, id)
				if len(f.Evidence) != 1 {
					t.Fatalf("%s: expected one reference, got %d", id, len(f.Evidence))
				}
			}
		})
	}
}

func TestGenericModuleDetectionIgnoresCommentsAndLongerNames(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "lib/example.ex", "# Example.HubSpot.call()\nExample.HubSpotBackup.call()\nExample.Auth.Signer.KmsBackup.sign()\n")
	rules, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"hubspot","pattern":"\\bExample\\.HubSpot\\b","detail":"Integration reference"},{"id":"aws-kms","pattern":"\\bExample\\.Auth\\.Signer\\.Kms\\b","detail":"Signer reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv := scanTestRepository(t, root, ScanOptions{Rules: rules})
	for _, f := range inv.Findings {
		if f.ID == "hubspot" || f.ID == "aws-kms" {
			t.Fatalf("non-reference matched: %s", f.ID)
		}
	}
}

func TestApprovalFlagsChangedEvidenceBeyondDisplayLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 31; i++ {
		fixture(t, root, fmt.Sprintf("app%02d/package.json", i), `{"dependencies":{"react":"19.0.0"}}`)
	}
	inv := scanTestRepository(t, root, ScanOptions{})
	policy := Policy{SchemaVersion: 1, Rules: map[string]Decision{"react": {Ring: "Adopt", Scope: "Web", Rationale: "Team decision", Owner: "Team", Status: "approved", Reference: "PR #1", Reviewed: "2026-10-30"}}}
	previous := buildTestReport(t, inv, policy, nil, "Demo", BuildOptions{})
	fixture(t, root, "app30/package.json", `{"dependencies":{"react":"20.0.0"}}`)
	inv = scanTestRepository(t, root, ScanOptions{})
	next := buildTestReport(t, inv, Policy{}, &previous, "", BuildOptions{})
	if len(next.Findings) != 1 || !next.Findings[0].EvidenceChanged || !next.Findings[0].ReviewRequired {
		t.Fatal("changed evidence outside the display limit retained an unflagged approval")
	}
}

func TestGeneratedJSONDoesNotBecomeScannerInput(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "package.json", `{"dependencies":{"react":"19.0.0"}}`)
	initial := scanTestRepository(t, root, ScanOptions{})
	report := buildTestReport(t, initial, Policy{}, nil, "Demo", BuildOptions{})
	data, err := Render(report, "json")
	if err != nil {
		t.Fatal(err)
	}
	fixture(t, root, "infrastructure/radar.json", string(data))
	next := scanTestRepository(t, root, ScanOptions{})
	if !reflect.DeepEqual(initial, next) {
		t.Fatal("generated JSON changed its own scan inputs")
	}
}

func finding(t *testing.T, inv Inventory, id string) Finding {
	t.Helper()
	for _, f := range inv.Findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("missing finding %q in %#v", id, inv.Findings)
	return Finding{}
}

// Check lockfile versions, private values and unknown dependency handling.
func TestScanManifestEvidenceAndNoAdoption(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "mix.exs", "defmodule Demo.MixProject do\n  defp deps do\n    [\n      {:phoenix, \"~> 1.8\"},\n      {:unknown_hex, \"~> 2.0\"}\n    ]\n  end\nend\n")
	fixture(t, root, "mix.lock", "%{\n  \"phoenix\": {:hex, :phoenix, \"1.8.16\", \"hash\", []}\n}\n")
	fixture(t, root, "assets/package.json", "{\"dependencies\":{\"@sentry/browser\":\"^11.0.0\",\"unknown-ui\":\"1.2.3\"}}")
	fixture(t, root, ".tool-versions", "elixir 1.20.2-otp-29\nerlang 29.0\n")
	fixture(t, root, "Dockerfile", "FROM postgres:15\n")
	fixture(t, root, "config/runtime.exs", "config :demo, token: \"DO_NOT_EXPORT_THIS_VALUE\"\nkey = System.get_env(\"HUBSPOT_API_KEY\")\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	phoenix := finding(t, inv, "phoenix")
	if len(phoenix.Evidence) != 2 {
		t.Fatalf("want constraint and resolved version, got %#v", phoenix.Evidence)
	}
	if phoenix.Evidence[0].Path != "mix.exs" || phoenix.Evidence[0].Line != 4 {
		t.Fatalf("wrong declaration location: %#v", phoenix.Evidence)
	}
	if !strings.Contains(phoenix.Evidence[1].Detail, "1.8.16") {
		t.Fatal("lost lockfile version")
	}
	if finding(t, inv, "npm:unknown-ui").Quadrant != "" {
		t.Fatal("guessed unknown package quadrant")
	}
	_ = finding(t, inv, "hex:unknown_hex")
	_ = finding(t, inv, "hubspot")
	_ = finding(t, inv, "postgresql")
	for _, f := range inv.Findings {
		if f.Decision.Ring != "" || !f.ReviewRequired {
			t.Fatalf("automatic endorsement: %#v", f)
		}
	}
	b, _ := json.Marshal(inv)
	if strings.Contains(string(b), "DO_NOT_EXPORT") || strings.Contains(string(b), root) {
		t.Fatal("source value or absolute path leaked")
	}
}

func TestScanIgnoresDependenciesCredentialsAndSymlinks(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	fixture(t, root, "package.json", "{\"dependencies\":{\"react\":\"^19\"}}")
	fixture(t, root, "node_modules/demo/package.json", "{broken")
	fixture(t, root, "deps/demo/mix.exs", "{:ash, \"~> 3\"}")
	fixture(t, root, ".env.local", "HUBSPOT_API_KEY=SECRET")
	fixture(t, external, "package.json", "{broken")
	if err := os.Symlink(filepath.Join(external, "package.json"), filepath.Join(root, "linked-package.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "external")); err != nil {
		t.Fatal(err)
	}
	inv := scanTestRepository(t, root, ScanOptions{})
	if len(inv.Findings) != 1 || inv.Findings[0].ID != "react" {
		t.Fatalf("crossed scan boundary: %#v", inv.Findings)
	}
}

func TestScanRejectsMalformedSupportedManifest(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "assets/package.json", "{broken")
	if _, err := ScanWithOptions(root, ScanOptions{}); err == nil || !strings.Contains(err.Error(), "assets/package.json") {
		t.Fatalf("want contextual manifest error, got %v", err)
	}
}

func TestScanDeterministicAndIgnoresGeneratedDocuments(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	files := [][2]string{{"package.json", `{"dependencies":{"react":"^19","z-package":"1.0"}}`}, {"src/app.js", "import React from 'react';\n"}, {"TECHNOLOGY_RADAR.md", "<!-- tech-radar:generated -->\nHUBSPOT_API_KEY\n"}}
	for _, f := range files {
		fixture(t, a, f[0], f[1])
	}
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		fixture(t, b, f[0], f[1])
	}
	x := scanTestRepository(t, a, ScanOptions{})
	y := scanTestRepository(t, b, ScanOptions{})
	if !reflect.DeepEqual(x, y) {
		t.Fatalf("scan depends on filesystem creation order:\n%#v\n%#v", x, y)
	}
	before := x.Source.Fingerprint
	fixture(t, a, "TECHNOLOGY_RADAR.md", "<!-- tech-radar:generated -->\nchanged generated output\n")
	x = scanTestRepository(t, a, ScanOptions{})
	if x.Source.Fingerprint != before {
		t.Fatal("generated output became scanner input")
	}
}

func TestScanGoModulesAndCommentedMixDependencies(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "go.mod", "module example.test/app\n\ngo 1.23\nrequire example.test/library v1.2.3\n")
	fixture(t, root, "mix.exs", "# {:phoenix, \"~> 1.8\"}\n# use Ash.Resource\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	_ = finding(t, inv, "go")
	_ = finding(t, inv, "go:example.test/library")
	for _, f := range inv.Findings {
		if f.ID == "phoenix" || f.ID == "ash" {
			t.Fatalf("comment became a dependency: %#v", f)
		}
	}
}

// Check SCC language detection and comment handling.
func TestSCCCountsLanguagesAndIgnoresBlockCommentReferences(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "src/Main.java", "// header\nclass Main {}\n\n")
	fixture(t, root, "src/app.js", "/*\nHUBSPOT_API_KEY is only a comment\n*/\nconsole.log('hello');\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	_ = finding(t, inv, "language:java")
	for _, f := range inv.Findings {
		if f.ID == "hubspot" {
			t.Fatal("block comment became integration evidence")
		}
	}
	var java LanguageStat
	for _, s := range inv.Languages {
		if s.Language == "Java" {
			java = s
		}
	}
	if java.Files != 1 || java.Code != 1 || java.Comments != 1 || java.Blank != 1 || java.Lines != 3 {
		t.Fatalf("incorrect SCC language summary: %#v", java)
	}
	if inv.Source.Counter != "scc/v4 v4.1.0" {
		t.Fatalf("counter provenance missing: %q", inv.Source.Counter)
	}
}

func TestGoRegexDefinitionsDoNotBecomeTechnologyUse(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "recognizers.go", `package demo
import rx "regexp"
func compile(expression string) *rx.Regexp { return rx.MustCompile(expression) }
func forward(expression string) *rx.Regexp { return compile(expression) }
var expressions = []*rx.Regexp{
 forward("aws ecs|aws eks|secretsmanager:|minio/minio|actions/setup-node@"),
 rx.MustCompile("minio server"),
}
var labels = []string{"Req / Req.Test", "from 'react'", "use Phoenix.Router"}
`)
	fixture(t, root, "usage.go", `package demo
import "os"
var key = os.Getenv("HUBSPOT_API_KEY")
var permissions = []string{"secretsmanager:GetSecretValue"}
var command = "aws ecs describe-services"
var message = "hello" // HONEYCOMB_API_KEY
`)
	fixture(t, root, "client.ex", "Req.new()\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	found := map[string]Finding{}
	for _, f := range inv.Findings {
		found[f.ID] = f
	}
	for _, id := range []string{"go", "hubspot", "aws-secrets", "amazon-ecs", "req"} {
		f, ok := found[id]
		if !ok {
			t.Errorf("missing legitimate finding %s", id)
			continue
		}
		if id == "go" {
			continue
		}
		want := "usage.go"
		if id == "req" {
			want = "client.ex"
		}
		if len(f.Evidence) != 1 || f.Evidence[0].Path != want {
			t.Errorf("%s evidence = %+v, want one reference in %s", id, f.Evidence, want)
		}
	}
	for _, id := range []string{"amazon-eks", "minio", "nodejs", "react", "phoenix", "honeycomb"} {
		if f, ok := found[id]; ok {
			t.Errorf("regex or catalog text became %s: %+v", id, f.Evidence)
		}
	}
}

func TestCustomRulesStillInspectGoRegexText(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "matcher.go", `package demo
import "regexp"
var matcher = regexp.MustCompile("minio/minio")
`)
	rules, err := ReadDetectionRules([]byte(`{"schema_version":1,"patterns":[{"id":"example-pattern","name":"Example pattern","quadrant":"Tools","pattern":"minio/minio","detail":"Explicit regex reference"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	inv := scanTestRepository(t, root, ScanOptions{Rules: rules})
	var custom *Finding
	for i := range inv.Findings {
		f := &inv.Findings[i]
		if f.ID == "minio" {
			t.Errorf("regex text became built-in MinIO usage: %+v", f.Evidence)
		}
		if f.ID == "example-pattern" {
			custom = f
		}
	}
	if custom == nil || len(custom.Evidence) != 1 || custom.Evidence[0].Line != 3 {
		t.Fatalf("explicit rule evidence = %+v, want matcher.go:3", custom)
	}
}

func TestFrameworkSyntaxStaysDetectedInItsSourceLanguages(t *testing.T) {
	for _, tc := range []struct{ name, source, id string }{
		{"app.ex", "use Phoenix.Router\n", "phoenix"},
		{"app.exs", "Req.new()\n", "req"},
		{"app.js", "import React from 'react';\n", "react"},
		{"app.jsx", "import React from 'react';\nexport default () => <div />;\n", "react"},
		{"app.tsx", "import React from 'react';\n", "react"},
		{"app.vue", "<script>import React from 'react';</script>\n", "react"},
		{"app.svelte", "<script>import React from 'react';</script>\n", "react"},
		{"style.css", "@import 'tailwindcss';\n", "tailwind"},
		{"style.scss", "@import 'tailwindcss';\n", "tailwind"},
		{"style.less", "@import 'tailwindcss';\n", "tailwind"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, tc.name, tc.source)
			inv := scanTestRepository(t, root, ScanOptions{})
			for _, f := range inv.Findings {
				if f.ID == tc.id && len(f.Evidence) > 0 {
					return
				}
			}
			t.Fatalf("missing %s in %s: %+v", tc.id, tc.name, inv.Findings)
		})
	}
}

func TestIgnoreFileCountsDoNotBecomeALanguageChoice(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, ".gitignore", "generated/\n")
	fixture(t, root, "app.go", "package demo\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	goFound, ignoreCounted := false, false
	for _, f := range inv.Findings {
		if f.ID == "language:gitignore" {
			t.Errorf("ignore syntax became a language choice: %+v", f)
		}
		goFound = goFound || f.ID == "go"
	}
	for _, stat := range inv.Languages {
		ignoreCounted = ignoreCounted || (strings.EqualFold(stat.Language, "gitignore") && stat.Files == 1)
	}
	if !goFound || !ignoreCounted {
		t.Fatalf("lost Go finding or ignore-file counts: %+v", inv)
	}
}
