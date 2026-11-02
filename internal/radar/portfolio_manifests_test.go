package radar

import (
	"encoding/json"
	"strings"
	"testing"
)

func portfolioManifestScanner() *scanner {
	s := &scanner{known: map[string]Technology{}, aliases: map[string]string{}, found: map[string]*Finding{}, declared: map[string]map[string]bool{}, seen: map[string]map[Evidence]bool{}}
	for _, tech := range Catalog() {
		s.known[tech.ID] = tech
		for _, alias := range tech.Hex {
			s.aliases["hex:"+alias] = tech.ID
		}
		for _, alias := range tech.NPM {
			s.aliases["npm:"+alias] = tech.ID
		}
	}
	return s
}

// Check dependency names, aliases, locations and scopes.
func TestPortfolioManifestLiteralDependencies(t *testing.T) {
	tests := []struct {
		name, input string
		want        map[string]Evidence
	}{
		{"pyproject.toml", "[project]\ndependencies = [\n  \"FastAPI>=0.100\", # comment\n  \"sample_widget[http]>=1.2; python_version > '3.9'\",\n]\n[project.optional-dependencies]\ntest = [\"pytest>=8\"]\n", map[string]Evidence{
			"fastapi":            {"pyproject.toml", 3, "declaration", "pypi package fastapi: >=0.100 [runtime]"},
			"pypi:sample-widget": {"pyproject.toml", 4, "declaration", "pypi package sample-widget: >=1.2 [restricted]"},
			"pypi:pytest":        {"pyproject.toml", 7, "declaration", "pypi package pytest: >=8 [restricted]"},
		}},
		{"requirements-dev.txt", "# fastapi>=1\npydantic>=2\ncustom_pkg==1.0\n", map[string]Evidence{
			"pydantic":        {"requirements-dev.txt", 2, "declaration", "pypi package pydantic: >=2 [development]"},
			"pypi:custom-pkg": {"requirements-dev.txt", 3, "declaration", "pypi package custom-pkg: ==1.0 [development]"},
		}},
		{"Gemfile", "# gem 'sidekiq'\ngem 'rails', '~> 7.2'\ngroup :development, :test do\n  gem \"custom_gem\", \"1.2\"\nend\ngem 'sidekiq'\n", map[string]Evidence{
			"rails":          {"Gemfile", 2, "declaration", "gem package rails: ~> 7.2 [runtime]"},
			"gem:custom_gem": {"Gemfile", 4, "declaration", "gem package custom_gem: 1.2 [development]"},
			"sidekiq":        {"Gemfile", 6, "declaration", "gem package sidekiq: * [runtime]"},
		}},
		{"composer.json", "{\n\"require\": {\"php\": \"^8.3\", \"laravel/framework\": \"^11\", \"sample/library\": \"1.2\"},\n\"require-dev\": {\"phpunit/phpunit\": \"^11\"}\n}", map[string]Evidence{
			"laravel":                  {"composer.json", 2, "declaration", "composer package laravel/framework: ^11 [runtime]"},
			"composer:sample/library":  {"composer.json", 2, "declaration", "composer package sample/library: 1.2 [runtime]"},
			"composer:phpunit/phpunit": {"composer.json", 3, "declaration", "composer package phpunit/phpunit: ^11 [development]"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := portfolioManifestScanner()
			warnings, err := s.inspectPortfolioManifest(inputFile{tt.name, []byte(tt.input)})
			if err != nil || len(warnings) != 0 {
				t.Fatalf("warnings=%v err=%v", warnings, err)
			}
			for id, want := range tt.want {
				got := s.found[id]
				if got == nil || len(got.Evidence) != 1 || got.Evidence[0] != want {
					t.Errorf("%s: got %#v; want %#v", id, got, want)
				}
			}
		})
	}
}

// Dynamic declarations and private URLs must stay out of evidence.
func TestPortfolioManifestCoverageAndSanitization(t *testing.T) {
	for _, tt := range []struct{ name, input, want string }{
		{"requirements.txt", "-r extra.txt\nhttps://example.invalid/token.whl\nliteral_pkg @ https://example.invalid/private?token=secret\n", "pypi:literal-pkg"},
		{"Gemfile", "gem ENV.fetch('PACKAGE')\ngem 'rails', git: 'https://example.invalid/secret'\neval_gemfile 'extra.rb'\n", "rails"},
		{"pyproject.toml", "[project]\ndynamic = [\"dependencies\"]\n[tool.poetry.dependencies]\npython = \"^3.12\"\nfastapi = {version = \"^0.100\", extras = [\"standard\"]}\nprivate_lib = {git = \"https://example.invalid/secret\"}\n[tool.poetry.group.dev.dependencies]\npytest = \"^8\"\n", "fastapi"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := portfolioManifestScanner()
			warnings, err := s.inspectPortfolioManifest(inputFile{tt.name, []byte(tt.input)})
			if err != nil || len(warnings) == 0 || s.found[tt.want] == nil {
				t.Fatalf("warnings=%v err=%v findings=%v", warnings, err, s.found)
			}
			data, _ := json.Marshal(struct {
				Findings map[string]*Finding
				Warnings []string
			}{s.found, warnings})
			for _, forbidden := range []string{"https://", "secret", "ENV.fetch", "extra.rb"} {
				if strings.Contains(string(data), forbidden) {
					t.Errorf("retained raw source %q: %s", forbidden, data)
				}
			}
			if tt.name == "pyproject.toml" {
				if got := s.found["pypi:pytest"]; got == nil || !strings.Contains(got.Evidence[0].Detail, "[development]") {
					t.Errorf("missing Poetry dev scope: %#v", got)
				}
			}
		})
	}
}

func TestPortfolioManifestJSONErrorsAndBoundaries(t *testing.T) {
	for _, name := range []string{"composer.json", "plugin.json", ".mcp.json"} {
		s := portfolioManifestScanner()
		_, err := s.inspectPortfolioManifest(inputFile{"service/" + name, []byte(`{"broken":`)})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: missing contextual JSON error: %v", name, err)
		}
	}
	s := portfolioManifestScanner()
	for _, name := range []string{"composer.json", "Gemfile", "pyproject.toml", "requirements.txt", "plugin.json", ".mcp.json", "SKILL.md"} {
		input := ""
		if strings.HasSuffix(name, ".json") {
			input = "{}"
		}
		_, err := s.inspectPortfolioManifest(inputFile{name, []byte(input)})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, id := range []string{"python", "ruby", "language:php", "plugin-artifact"} {
		if s.found[id] == nil {
			t.Errorf("missing manifest language/boundary %q", id)
		}
	}

}

// Examples in quoted documentation aren't package declarations.
func TestPortfolioManifestSkipsMultilineDocumentation(t *testing.T) {
	s := portfolioManifestScanner()
	_, err := s.inspectPortfolioManifest(inputFile{"pyproject.toml", []byte("[project]\ndescription = \"\"\"\n[tool.poetry.dependencies]\nphantom = \"1.0\"\n\"\"\"\n[project.optional-dependencies]\ntest = [\"pytest>=8\"]\n")})
	if err != nil {
		t.Fatal(err)
	}
	if s.found["pypi:phantom"] != nil || s.found["pypi:pytest"] == nil {
		t.Fatalf("documentation became dependencies: %#v", s.found)
	}
}

func TestPortfolioManifestComposerRepeatedDependencyLine(t *testing.T) {
	s := portfolioManifestScanner()
	_, err := s.inspectPortfolioManifest(inputFile{"composer.json", []byte("{\n\"require\": {\"sample/library\": \"^1\"},\n\"require-dev\": {\"sample/library\": \"^2\"}\n}")})
	if err != nil {
		t.Fatal(err)
	}
	got := s.found["composer:sample/library"]
	if got == nil || len(got.Evidence) != 2 || got.Evidence[0].Line != 2 || got.Evidence[1].Line != 3 {
		t.Fatalf("wrong declaration provenance: %#v", got)
	}
}

func TestPortfolioManifestDynamicGemAndConditionalScopes(t *testing.T) {
	s := portfolioManifestScanner()
	warnings, err := s.inspectPortfolioManifest(inputFile{"Gemfile", []byte("if ENV['ENABLE_WORKER']\n  group :development do\n    gem 'sidekiq', '~> 7', git: ENV['REPO']\n  end\nend\n")})
	if err != nil || len(warnings) == 0 {
		t.Fatalf("missing dynamic coverage warning: %v %v", warnings, err)
	}
	got := s.found["sidekiq"]
	if got == nil || !strings.Contains(got.Evidence[0].Detail, "[restricted]") {
		t.Fatalf("conditional dependency lost restriction: %#v", got)
	}
}

func TestPortfolioManifestDoesNotInventNamesFromExpressions(t *testing.T) {
	for _, tt := range []struct{ name, input, phantom string }{
		{"Gemfile", "gem 'rails' + suffix\n", "rails"},
		{"pyproject.toml", "[tool.poetry.dependencies]\nphantom = \"1\" + suffix\n", "pypi:phantom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := portfolioManifestScanner()
			warnings, err := s.inspectPortfolioManifest(inputFile{tt.name, []byte(tt.input)})
			if err != nil || len(warnings) == 0 || s.found[tt.phantom] != nil {
				t.Fatalf("expression misread: warnings=%v error=%v findings=%#v", warnings, err, s.found)
			}
		})
	}
}

func TestPortfolioManifestSkipsRubyBlockComments(t *testing.T) {
	s := portfolioManifestScanner()
	warnings, err := s.inspectPortfolioManifest(inputFile{"Gemfile", []byte("=begin\ngem 'phantom'\n=end\ngem 'rails'\n")})
	if err != nil || len(warnings) != 0 || s.found["gem:phantom"] != nil || s.found["rails"] == nil {
		t.Fatalf("block comment became declaration: warnings=%v error=%v findings=%#v", warnings, err, s.found)
	}
}
