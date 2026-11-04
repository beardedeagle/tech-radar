package radar

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestStandaloneUsesSupportedManifestParsersAndProfiles(t *testing.T) {
	for _, tt := range []struct {
		name, file, content, technology, profile string
	}{
		{"Python", "requirements.txt", "fastapi==0.115\npydantic==2.9\nopenai==1.0\n", "fastapi", "python_ai_api"},
		{"Ruby", "Gemfile", "gem 'rails', '8.0'\ngem 'activerecord', '8.0'\n", "rails", "ruby_rails"},
		{"PHP", "composer.json", `{"require":{"php":"^8.3","laravel/framework":"^12"}}`, "laravel", "php_application"},
		{"Node", "package.json", `{"dependencies":{"@apollo/server":"4","jose":"5"}}`, "apollo-graphql", "node_graphql_api"},
		{"Go", "go.mod", "module example.org/service\n\ngo 1.26\n", "go", "go_service"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, tt.file, tt.content)
			ordinary := scanTestRepository(t, root, ScanOptions{})
			finding(t, ordinary, tt.technology)
			matched := false
			for _, c := range ordinary.Components {
				matched = matched || c.Profile == tt.profile
			}
			if !matched {
				t.Fatalf("standalone scan did not select %s", tt.profile)
			}
			captured, _, err := CaptureRepository(root, DetectionRules{})
			if err != nil {
				t.Fatal(err)
			}
			if ordinary.Source.Fingerprint != captured.Source.Fingerprint || !reflect.DeepEqual(ordinary.Findings, captured.Findings) || !reflect.DeepEqual(ordinary.Components, captured.Components) {
				t.Fatal("ordinary scan and discovery disagree on the same repository")
			}
		})
	}
}

func TestLegacyContextStillReadsAndRebuilds(t *testing.T) {
	data, err := os.ReadFile("testdata/legacy-context.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ReadContext(data)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := pack.Bytes()
	if err != nil || !bytes.Equal(data, rebuilt) {
		t.Fatal("legacy context changed during validation or re-export")
	}
	updated, err := BuildContext(pack.pack.Snapshots)
	if err != nil || updated.pack.CatalogVersion != PortfolioVersion {
		t.Fatalf("could not rebuild legacy observations with current rules: %v", err)
	}
	legacyReport := Report{SchemaVersion: 2, GeneratorVersion: Version, Source: Source{Fingerprint: strings.Repeat("a", 64), ContextFingerprint: pack.fingerprint, CorpusVersion: "portfolio-1"}}
	data, err = Render(legacyReport, "json")
	if err != nil {
		t.Fatalf("legacy report provenance was rejected: %v", err)
	}
	if _, err := ReadReport(data); err != nil {
		t.Fatalf("legacy report cannot be reimported: %v", err)
	}
}

func TestDiscoveryFingerprintUsesCanonicalFileOrder(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "go.mod", "module example.org/service\n\ngo 1.26\n")
	fixture(t, root, "app.go", "package app\n")
	fixture(t, root, "app/worker.go", "package app\n")
	inv, files, err := CaptureRepository(root, DetectionRules{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildContext([]RepositorySnapshot{{ID: "local:example", Name: "example", Origin: "local", Status: "captured", Inventory: inv, Files: files}})
	if err != nil {
		t.Fatalf("a scan must validate against its own saved file hashes: %v", err)
	}
}
