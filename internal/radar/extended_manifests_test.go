package radar

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtendedManifestDependencies(t *testing.T) {
	for _, tt := range []struct {
		name, content, profile, family string
		members, absent                []string
	}{
		{"pom.xml", `<project><dependencyManagement><dependencies><dependency><groupId>fake</groupId><artifactId>managed</artifactId></dependency></dependencies></dependencyManagement><dependencies><dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId><version>4.0</version></dependency><dependency><groupId>org.springframework</groupId><artifactId>spring-context</artifactId></dependency></dependencies></project>`, "jvm_project", "Spring", []string{"maven:org.springframework.boot:spring-boot-starter-web", "maven:org.springframework:spring-context"}, []string{"maven:fake:managed"}},
		{"build.gradle.kts", "dependencies {\n implementation(\"org.springframework.boot:spring-boot-starter-web:4.0\")\n implementation(\"org.springframework:spring-context:7\")\n testImplementation(libs.junit)\n}\n", "jvm_project", "Spring", []string{"maven:org.springframework.boot:spring-boot-starter-web", "maven:org.springframework:spring-context"}, nil},
		{"App.csproj", `<Project Sdk="Microsoft.NET.Sdk.Web"><ItemGroup><PackageReference Include="Microsoft.EntityFrameworkCore" Version="10.0"/><PackageReference Include="Microsoft.EntityFrameworkCore.Sqlite" Version="10.0"/><PackageVersion Include="Fake.Managed" Version="1"/></ItemGroup></Project>`, "dotnet_project", "Entity Framework Core", []string{"nuget:microsoft.entityframeworkcore", "nuget:microsoft.entityframeworkcore.sqlite"}, []string{"nuget:fake.managed"}},
		{"Cargo.toml", "[package]\nname = \"example\"\nversion = \"1.0.0\"\n[dependencies]\ntokio = { version = \"1\", features = [\"full\"] }\nstreams = { package = \"tokio-stream\", version = \"0.1\" }\n[workspace.dependencies]\nphantom = \"1\"\n", "rust_project", "Tokio", []string{"cargo:tokio", "cargo:tokio-stream"}, []string{"cargo:phantom", "cargo:streams"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fixture(t, root, tt.name, tt.content)
			inv := scanTestRepository(t, root, ScanOptions{})
			foundProfile := false
			for _, c := range inv.Components {
				foundProfile = foundProfile || c.Profile == tt.profile
			}
			if !foundProfile {
				t.Fatalf("no %s profile: %+v", tt.profile, inv.Components)
			}
			report := buildTestReport(t, inv, Policy{}, nil, "Example", BuildOptions{})
			var group *Finding
			for i := range report.Highlights {
				if report.Highlights[i].Name == tt.family {
					group = &report.Highlights[i]
				}
			}
			if group == nil {
				t.Fatalf("no %s group: %+v", tt.family, report.Highlights)
			}
			for _, id := range tt.members {
				if !contains(group.Members, id) || len(finding(t, inv, id).Evidence) == 0 {
					t.Fatalf("missing evidence/member %s", id)
				}
			}
			for _, id := range tt.absent {
				for _, f := range inv.Findings {
					if f.ID == id {
						t.Fatalf("invented dependency %s", id)
					}
				}
			}
		})
	}
}

func TestExtendedManifestRestrictionsAndSources(t *testing.T) {
	for _, tt := range []struct{ name, input, id string }{
		{"pom.xml", `<project><profiles><profile><dependencies><dependency><groupId>example</groupId><artifactId>library</artifactId><version>${private.secret}</version></dependency></dependencies></profile></profiles></project>`, "maven:example:library"},
		{"App.csproj", `<Project><ItemGroup Condition="'$(Secret)' != ''"><PackageReference Include="Example.Library" Version="$(Secret)"/></ItemGroup></Project>`, "nuget:example.library"},
		{"Cargo.toml", "[target.'cfg(unix)'.dependencies]\nexample = { version = \"1\", git = \"https://example.invalid/private-secret\" }\n[dependencies]\ndynamic = { workspace = true }\n", "cargo:example"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := portfolioManifestScanner()
			warnings, err := s.inspectPortfolioManifest(inputFile{tt.name, []byte(tt.input)})
			if err != nil || len(warnings) == 0 {
				t.Fatalf("missing warning: %v %v", warnings, err)
			}
			f := s.found[tt.id]
			if f == nil || !restrictedDeclaration(f.Evidence[0]) {
				t.Fatalf("lost condition: %+v", f)
			}
			data, _ := json.Marshal(s.found)
			if strings.Contains(string(data), "secret") || strings.Contains(string(data), "Secret") || strings.Contains(string(data), "https://") {
				t.Fatalf("raw source escaped: %s", data)
			}
		})
	}
	for _, input := range []string{`<!DOCTYPE Project [<!ENTITY x "secret">]><Project/>`, `<Project>`} {
		s := portfolioManifestScanner()
		if _, err := s.inspectPortfolioManifest(inputFile{"App.csproj", []byte(input)}); err == nil {
			t.Fatalf("accepted unsafe/invalid XML: %s", input)
		}
	}
}

func TestManifestExamplesAreNotDependencies(t *testing.T) {
	for _, tt := range []struct{ name, input string }{
		{"pom.xml", `<project><properties><example><dependencies><dependency><groupId>fake</groupId><artifactId>example</artifactId></dependency></dependencies></example></properties></project>`},
		{"App.csproj", `<Project><PropertyGroup><Example><PackageReference Include="Fake.Example" Version="1"/></Example></PropertyGroup></Project>`},
		{"build.gradle", "def docs = \"\"\"\ndependencies {\n implementation 'fake:example:1'\n}\n\"\"\"\n"},
	} {
		s := portfolioManifestScanner()
		_, err := s.inspectPortfolioManifest(inputFile{tt.name, []byte(tt.input)})
		if err != nil {
			t.Fatal(err)
		}
		for id := range s.found {
			if strings.Contains(id, "fake") {
				t.Fatalf("example became a dependency: %s", id)
			}
		}
	}
}

func TestMavenUnresolvedScopeIsRestricted(t *testing.T) {
	for _, field := range []string{`<scope>${dependency.scope}</scope>`, `<optional>${optionalFlag}</optional>`} {
		s := portfolioManifestScanner()
		warnings, err := s.inspectPortfolioManifest(inputFile{"pom.xml", []byte(`<project><dependencies><dependency><groupId>org.springframework</groupId><artifactId>spring-context</artifactId>` + field + `</dependency></dependencies></project>`)})
		if err != nil {
			t.Fatal(err)
		}
		f := s.found["maven:org.springframework:spring-context"]
		if f == nil || !restrictedDeclaration(f.Evidence[0]) || len(warnings) == 0 {
			t.Fatalf("unresolved condition became runtime: %+v %v", f, warnings)
		}
	}
}
