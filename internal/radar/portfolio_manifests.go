package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// portfolioCatalog contains the additional manifest and profile technologies.
func portfolioCatalog() []Technology {
	return []Technology{
		{ID: "jvm", Name: "JVM", Quadrant: "Languages & Frameworks"},
		{ID: "dotnet", Name: ".NET", Quadrant: "Languages & Frameworks"},
		{ID: "language:rust", Name: "Rust", Quadrant: "Languages & Frameworks"},
		{ID: "commanded", Name: "Commanded", Quadrant: "Languages & Frameworks", Hex: []string{"commanded"}},
		{ID: "eventstore", Name: "EventStore", Quadrant: "Languages & Frameworks", Hex: []string{"eventstore"}},
		{ID: "apollo-graphql", Name: "Apollo GraphQL", Quadrant: "Languages & Frameworks", NPM: []string{"@apollo/server", "apollo-server", "apollo-server-lambda"}},
		{ID: "pothos", Name: "Pothos", Quadrant: "Languages & Frameworks", NPM: []string{"@pothos/core"}},
		{ID: "prisma", Name: "Prisma", Quadrant: "Languages & Frameworks", NPM: []string{"prisma", "@prisma/client"}},
		{ID: "jose", Name: "jose", Quadrant: "Languages & Frameworks", NPM: []string{"jose"}},
		{ID: "react-native", Name: "React Native", Quadrant: "Languages & Frameworks", NPM: []string{"react-native"}},
		{ID: "fastapi", Name: "FastAPI", Quadrant: "Languages & Frameworks"},
		{ID: "pydantic", Name: "Pydantic", Quadrant: "Languages & Frameworks"},
		{ID: "langgraph", Name: "LangGraph", Quadrant: "Languages & Frameworks"},
		{ID: "model-sdk", Name: "Model provider SDKs", Quadrant: "Languages & Frameworks"},
		{ID: "celery", Name: "Celery", Quadrant: "Languages & Frameworks"},
		{ID: "rails", Name: "Ruby on Rails", Quadrant: "Languages & Frameworks"},
		{ID: "activerecord", Name: "Active Record", Quadrant: "Languages & Frameworks"},
		{ID: "sidekiq", Name: "Sidekiq", Quadrant: "Languages & Frameworks"},
		{ID: "delayed-job", Name: "Delayed Job", Quadrant: "Languages & Frameworks"},
		{ID: "redis", Name: "Redis", Quadrant: "Platforms", Hex: []string{"redix"}, NPM: []string{"redis", "ioredis"}},
		{ID: "laravel", Name: "Laravel", Quadrant: "Languages & Frameworks"},
		{ID: "language:php", Name: "PHP", Quadrant: "Languages & Frameworks"},
		{ID: "plugin-artifact", Name: "Plugin and skill artifacts", Quadrant: "Tools"},
		{ID: "argocd", Name: "Argo CD", Quadrant: "Tools"},
	}
}

// inspectPortfolioManifest reads supported literal declarations.
// It doesn't evaluate Ruby or follow includes or dependency URLs.
// Unsupported syntax produces a warning.
func (s *scanner) inspectPortfolioManifest(f inputFile) (warnings []string, err error) {
	base := path.Base(f.name)
	switch base {
	case "pyproject.toml", "requirements.txt", "requirements-dev.txt":
		s.add("python", f, 1, "declaration", "Python dependency manifest")
		if base == "pyproject.toml" {
			warnings = s.portfolioPyproject(f)
		} else {
			scope := "runtime"
			if base == "requirements-dev.txt" {
				scope = "development"
			}
			for i, raw := range strings.Split(string(f.data), "\n") {
				line := strings.TrimSpace(portfolioUncomment(raw))
				if line != "" && !s.portfolioPythonDependency(f, line, i+1, scope) {
					warnings = append(warnings, portfolioCoverage(f, i+1, "nonliteral requirement or include"))
				}
			}
		}
	case "pom.xml":
		warnings, err = s.xmlManifest(f)
	case "build.gradle", "build.gradle.kts":
		warnings = s.gradleManifest(f)
	case "Cargo.toml":
		warnings = s.cargoManifest(f)
	case "Gemfile":
		s.add("ruby", f, 1, "declaration", "Ruby dependency manifest")
		warnings = s.portfolioGemfile(f)
	case "composer.json":
		var obj struct {
			Require map[string]string `json:"require"`
			Dev     map[string]string `json:"require-dev"`
		}
		if err := json.Unmarshal(f.data, &obj); err != nil {
			return nil, fmt.Errorf("%s: invalid composer.json: %w", f.name, err)
		}
		s.add("language:php", f, 1, "declaration", "PHP dependency manifest")
		locations := portfolioJSONSections(f.data)
		for _, section := range []struct {
			deps        map[string]string
			scope, name string
		}{{obj.Require, "runtime", "require"}, {obj.Dev, "development", "require-dev"}} {
			for _, name := range sortedKeys(section.deps) {
				if name == "php" {
					continue
				}
				location := locations[section.name]
				line := location.line + jsonKeyLine(location.data, name) - 1
				if !portfolioComposerName.MatchString(name) {
					warnings = append(warnings, portfolioCoverage(f, line, "unsupported Composer package name"))
					continue
				}
				s.portfolioDependency(f, "composer", name, section.deps[name], line, section.scope)
			}
		}
	case "plugin.json", ".mcp.json":
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(f.data, &obj); err != nil {
			return nil, fmt.Errorf("%s: invalid %s: %w", f.name, base, err)
		}
		s.add("plugin-artifact", f, 1, "configuration", "Plugin manifest boundary; metadata omitted")
	case "SKILL.md":
		s.add("plugin-artifact", f, 1, "configuration", "Skill artifact boundary; content omitted")
	default:
		if contains([]string{".csproj", ".fsproj", ".vbproj"}, path.Ext(f.name)) {
			warnings, err = s.xmlManifest(f)
		} else {
			return nil, nil
		}
	}
	return warnings, err
}

type portfolioJSONSection struct {
	data []byte
	line int
}

// portfolioJSONSections keeps section offsets so a package declared in two
// scopes retains both locations. The caller must validate the JSON first.
func portfolioJSONSections(data []byte) map[string]portfolioJSONSection {
	sections := map[string]portfolioJSONSection{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if _, err := decoder.Token(); err != nil {
		return sections
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return sections
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return sections
		}
		if name, ok := key.(string); ok {
			start := int(decoder.InputOffset()) - len(raw)
			sections[name] = portfolioJSONSection{data: raw, line: bytes.Count(data[:start], []byte("\n")) + 1}
		}
	}
	return sections
}

var portfolioPythonName = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(?:\[[A-Za-z0-9,._ -]+\])?(.*)$`)
var portfolioPythonNormalize = regexp.MustCompile(`[-_.]+`)
var portfolioPythonKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var portfolioVersion = regexp.MustCompile(`^[0-9A-Za-zvVxX*!^~<>=|, .+_-]+$`)
var portfolioComposerName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)?$`)

// portfolioDependency resolves Python, Ruby and PHP package names.
// Unknown packages use the shared scanner's normal dependency handling.
func (s *scanner) portfolioDependency(f inputFile, ecosystem, name, value string, line int, scope string) {
	alias := map[string]string{
		"pypi:fastapi": "fastapi", "pypi:pydantic": "pydantic", "pypi:langgraph": "langgraph", "pypi:langchain": "langchain",
		"pypi:openai": "model-sdk", "pypi:anthropic": "model-sdk", "pypi:celery": "celery", "pypi:redis": "redis",
		"gem:rails": "rails", "gem:activerecord": "activerecord", "gem:sidekiq": "sidekiq", "gem:delayed_job": "delayed-job", "gem:redis": "redis",
		"composer:laravel/framework": "laravel",
	}[ecosystem+":"+name]
	if alias != "" {
		s.aliases[ecosystem+":"+name] = alias
	}
	s.dependency(f, ecosystem, name, value, "declaration", line, scope)
}

func portfolioCoverage(f inputFile, line int, reason string) string {
	return fmt.Sprintf("%s:%d: dependency coverage limited: %s omitted.", f.name, line, reason)
}

// portfolioUncomment removes comments while keeping hashes inside quotes.
func portfolioUncomment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == '#' {
			return line[:i]
		}
	}
	return line
}

func (s *scanner) portfolioPythonDependency(f inputFile, literal string, line int, scope string) bool {
	m := portfolioPythonName.FindStringSubmatch(strings.TrimSpace(literal))
	if m == nil {
		return false
	}
	name := portfolioPythonNormalize.ReplaceAllString(strings.ToLower(m[1]), "-")
	value := strings.TrimSpace(m[2])
	if i := strings.IndexByte(value, ';'); i >= 0 {
		value = strings.TrimSpace(value[:i])
		scope = "restricted"
	}
	if value == "" {
		value = "*"
	} else if strings.HasPrefix(value, "@") {
		// Keep the package name and omit its source URL.
		value = "[source omitted]"
	} else if !strings.ContainsAny(value[:1], "<>=!~") || !portfolioVersion.MatchString(value) {
		return false
	}
	s.portfolioDependency(f, "pypi", name, value, line, scope)
	return true
}

// portfolioQuoted reads a single-line quoted string.
// Escapes and multiline strings aren't supported.
func portfolioQuoted(text string) (value, rest string, ok bool) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || (text[0] != '\'' && text[0] != '"') {
		return "", text, false
	}
	quote := text[0]
	for i := 1; i < len(text); i++ {
		if text[i] == '\\' || text[i] == '\n' {
			return "", text, false
		}
		if text[i] == quote {
			return text[1:i], strings.TrimSpace(text[i+1:]), true
		}
	}
	return "", text, false
}

func (s *scanner) portfolioPyproject(f inputFile) []string {
	var warnings []string
	section, arrayScope, multiline := "", "", ""
	for i, raw := range strings.Split(string(f.data), "\n") {
		if multiline != "" {
			if strings.Contains(raw, multiline) {
				multiline = ""
			}
			continue
		}
		line := strings.TrimSpace(portfolioUncomment(raw))
		if line == "" {
			continue
		}
		if arrayScope != "" {
			var done bool
			warnings, done = s.portfolioPythonArray(f, line, i+1, arrayScope, warnings)
			if done {
				arrayScope = ""
			}
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.TrimSpace(line)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
			if !strings.Contains(value[3:], value[:3]) {
				multiline = value[:3]
			}
			if key == "dependencies" || strings.Contains(section, "dependencies]") {
				warnings = append(warnings, portfolioCoverage(f, i+1, "multiline dependency literal"))
			}
			continue
		}
		if section == "[project]" && key == "dynamic" && strings.Contains(value, "dependencies") {
			warnings = append(warnings, portfolioCoverage(f, i+1, "dynamic project dependencies"))
		}
		scope := ""
		if section == "[project]" && key == "dependencies" {
			scope = "runtime"
		}
		if section == "[project.optional-dependencies]" {
			scope = "restricted"
		}
		if scope != "" {
			if !strings.HasPrefix(value, "[") {
				warnings = append(warnings, portfolioCoverage(f, i+1, "nonliteral Python dependency array"))
				continue
			}
			var done bool
			warnings, done = s.portfolioPythonArray(f, strings.TrimSpace(value[1:]), i+1, scope, warnings)
			if !done {
				arrayScope = scope
			}
			continue
		}
		poetry := section == "[tool.poetry.dependencies]" || section == "[tool.poetry.dev-dependencies]" || (strings.HasPrefix(section, "[tool.poetry.group.") && strings.HasSuffix(section, ".dependencies]"))
		if !poetry {
			continue
		}
		if v, rest, ok := portfolioQuoted(key); ok && rest == "" {
			key = v
		}
		if !portfolioPythonKey.MatchString(key) {
			warnings = append(warnings, portfolioCoverage(f, i+1, "nonliteral Poetry dependency"))
			continue
		}
		if key == "python" {
			continue
		}
		scope = "runtime"
		if section == "[tool.poetry.dev-dependencies]" || section == "[tool.poetry.group.dev.dependencies]" || section == "[tool.poetry.group.test.dependencies]" {
			scope = "development"
		} else if section != "[tool.poetry.dependencies]" {
			scope = "restricted"
		}
		v, rest, literal := portfolioQuoted(value)
		if !literal || rest != "" {
			literal = false
			if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
				version := portfolioPoetryVersion.FindStringSubmatch(value)
				if version != nil {
					v = version[1]
					literal = true
				}
				if strings.Contains(value, "optional") || strings.Contains(value, "markers") || strings.Contains(value, "platform") || strings.Contains(value, "python") {
					scope = "restricted"
				}
				if strings.Contains(value, "git") || strings.Contains(value, "path") || strings.Contains(value, "url") {
					warnings = append(warnings, portfolioCoverage(f, i+1, "Poetry source reference"))
				}
			}
			if !literal {
				warnings = append(warnings, portfolioCoverage(f, i+1, "nonliteral Poetry source"))
				continue
			}
		}
		name := portfolioPythonNormalize.ReplaceAllString(strings.ToLower(key), "-")
		s.portfolioDependency(f, "pypi", name, v, i+1, scope)
	}
	if arrayScope != "" {
		warnings = append(warnings, portfolioCoverage(f, len(strings.Split(string(f.data), "\n")), "unterminated Python dependency array"))
	}
	return warnings
}

var portfolioPoetryVersion = regexp.MustCompile(`(?:\{|,)\s*version\s*=\s*["']([^"'\\]+)["']`)

func (s *scanner) portfolioPythonArray(f inputFile, text string, line int, scope string, warnings []string) ([]string, bool) {
	for text != "" {
		if strings.HasPrefix(text, "]") {
			return warnings, true
		}
		literal, rest, ok := portfolioQuoted(text)
		if !ok {
			return append(warnings, portfolioCoverage(f, line, "nonliteral Python dependency array entry")), true
		}
		if !s.portfolioPythonDependency(f, literal, line, scope) {
			warnings = append(warnings, portfolioCoverage(f, line, "nonliteral Python dependency"))
		}
		if strings.HasPrefix(rest, ",") {
			text = strings.TrimSpace(rest[1:])
			continue
		}
		if rest != "" && !strings.HasPrefix(rest, "]") {
			return append(warnings, portfolioCoverage(f, line, "unsupported Python array syntax")), true
		}
		text = rest
	}
	return warnings, false
}

var portfolioGemStart = regexp.MustCompile(`^gem\b\s*\(?\s*`)
var portfolioGemName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var portfolioRubyGroup = regexp.MustCompile(`:(development|test)\b`)

func (s *scanner) portfolioGemfile(f inputFile) []string {
	var warnings, scopes []string
	blockComment := false
	for i, raw := range strings.Split(string(f.data), "\n") {
		if strings.HasPrefix(raw, "=begin") {
			blockComment = true
			continue
		}
		if blockComment {
			if strings.HasPrefix(raw, "=end") {
				blockComment = false
			}
			continue
		}
		line := strings.TrimSpace(portfolioUncomment(raw))
		if line == "" {
			continue
		}
		if line == "end" {
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		}
		if strings.HasSuffix(line, " do") || strings.HasPrefix(line, "if ") || strings.HasPrefix(line, "unless ") {
			scope := "restricted"
			if strings.HasPrefix(line, "group ") && portfolioRubyGroup.MatchString(line) {
				scope = "development"
			}
			if strings.HasPrefix(line, "source ") {
				scope = "runtime"
			}
			scopes = append(scopes, scope)
			continue
		}
		prefix := portfolioGemStart.FindString(line)
		if prefix == "" {
			if strings.HasPrefix(line, "eval_gemfile") || strings.HasPrefix(line, "gemspec") {
				warnings = append(warnings, portfolioCoverage(f, i+1, "dynamic Gemfile declarations"))
			}
			continue
		}
		name, rest, ok := portfolioQuoted(strings.TrimPrefix(line, prefix))
		if !ok || !portfolioGemName.MatchString(name) {
			warnings = append(warnings, portfolioCoverage(f, i+1, "nonliteral gem name"))
			continue
		}
		if rest != "" && !strings.HasPrefix(rest, ",") && !strings.HasPrefix(rest, ")") && !strings.HasPrefix(rest, "if ") && !strings.HasPrefix(rest, "unless ") {
			warnings = append(warnings, portfolioCoverage(f, i+1, "dynamic gem name expression"))
			continue
		}
		scope := "runtime"
		for _, bounded := range scopes {
			if bounded == "restricted" {
				scope = "restricted"
				break
			}
			if bounded == "development" {
				scope = "development"
			}
		}
		if scope != "restricted" && (strings.Contains(rest, "group:") || strings.Contains(rest, "groups:")) {
			scope = "restricted"
			if portfolioRubyGroup.MatchString(rest) {
				scope = "development"
			}
		}
		if strings.Contains(rest, "platform") || strings.HasPrefix(rest, "if ") || strings.HasPrefix(rest, "unless ") || strings.Contains(rest, " if ") || strings.Contains(rest, " unless ") {
			scope = "restricted"
		}
		if strings.Contains(rest, "git:") || strings.Contains(rest, "path:") || strings.Contains(rest, "github:") {
			warnings = append(warnings, portfolioCoverage(f, i+1, "gem source reference"))
		}
		version := "*"
		if strings.HasPrefix(rest, ",") {
			var versions []string
			tail := strings.TrimSpace(rest[1:])
			for {
				v, next, literal := portfolioQuoted(tail)
				if !literal {
					break
				}
				versions = append(versions, v)
				tail = strings.TrimSpace(next)
				if !strings.HasPrefix(tail, ",") {
					break
				}
				tail = strings.TrimSpace(tail[1:])
			}
			if len(versions) > 0 {
				version = strings.Join(versions, ", ")
			} else if tail != "" && tail != ")" && !strings.HasPrefix(tail, "group:") && !strings.HasPrefix(tail, "groups:") && !strings.HasPrefix(tail, "require:") {
				version = "[source omitted]"
				warnings = append(warnings, portfolioCoverage(f, i+1, "dynamic gem version or source"))
			}
		}
		s.portfolioDependency(f, "gem", name, version, i+1, scope)
	}
	return warnings
}
