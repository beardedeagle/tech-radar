package radar

import (
	"io/fs"
	"path"
	"strings"
)

// PortfolioPathEligible applies shared path and filename exclusions to all scans.
// It allows supported source, configuration and manifest files.
// Callers must still reject links and non-regular archive entries.
func PortfolioPathEligible(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") {
		return false
	}
	parts := strings.Split(name, "/")
	for _, dir := range parts[:len(parts)-1] {
		if skipDirectory(dir) && dir != ".claude-plugin" {
			return false
		}
	}
	base := path.Base(name)
	lower := strings.ToLower(base)
	if strings.HasPrefix(base, ".env") || strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return false
	}
	for _, word := range []string{"secret", "credential", "id_rsa", "id_ed25519"} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(lower, ext) {
			return false
		}
	}
	if contains([]string{".csproj", ".fsproj", ".vbproj"}, path.Ext(name)) {
		return true
	}
	switch base {
	case "Cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts", "pyproject.toml", "requirements.txt", "requirements-dev.txt", "Gemfile", "composer.json", "plugin.json", ".mcp.json", "SKILL.md":
		return true
	}
	return eligible(name)
}
