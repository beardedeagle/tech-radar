package radar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var toolVersionIDs = map[string]string{"elixir": "elixir", "erlang": "erlang-otp", "nodejs": "nodejs", "golang": "go", "python": "python", "ruby": "ruby"}

func kindOrder(kind string) int {
	switch kind {
	case "declaration":
		return 0
	case "lock resolution":
		return 1
	case "tool pin":
		return 2
	case "configuration":
		return 3
	case "source reference":
		return 4
	case "documentation":
		return 5
	}
	return 6
}

func (s *scanner) add(id string, f inputFile, line int, kind, detail string) {
	if alias, ok := s.aliases[id]; ok {
		id = alias
	}
	finding := s.found[id]
	if finding == nil {
		tech, ok := s.known[id]
		if !ok {
			tech = Technology{ID: id, Name: id}
		}
		finding = &Finding{ID: id, Name: tech.Name, Quadrant: tech.Quadrant, Evidence: []Evidence{}, Presence: "detected", ReviewRequired: true}
		s.found[id] = finding
	}
	e := Evidence{f.name, line, kind, detail}
	if s.seen[id] == nil {
		s.seen[id] = map[Evidence]bool{}
	}
	if s.seen[id][e] {
		return
	}
	s.seen[id][e] = true
	finding.Evidence = append(finding.Evidence, e)
}

var safeVersion = regexp.MustCompile(`^[0-9vVxX*^~<>=|, .+_-]+$`)

func versionValue(value string) string {
	if len(value) <= 120 {
		if _, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v")); err == nil {
			return value
		}
	}
	if len(value) <= 120 && (safeVersion.MatchString(value) || value == "latest" || value == "workspace:*" || value == "catalog:") {
		return value
	}
	return "[non-version source reference omitted]"
}

func (s *scanner) dependency(f inputFile, ecosystem, name, value, kind string, line int, scope string) {
	if kind == "declaration" {
		key := ecosystem + ":" + path.Dir(f.name)
		if s.declared[key] == nil {
			s.declared[key] = map[string]bool{}
		}
		s.declared[key][name] = true
	}
	detail := ecosystem + " package " + name + ": " + versionValue(value)
	if scope != "" {
		detail += " [" + scope + "]"
	}
	s.add(ecosystem+":"+name, f, line, kind, detail)
}

var mixDep = regexp.MustCompile(`^\s*\{\s*:([a-zA-Z0-9_]+)\s*,(?:\s*"([^"]*)")?`)
var mixLocked = regexp.MustCompile(`^\s*"([^"]+)":\s*\{:hex,\s*:[^,]+,\s*"([^"]+)"`)
var goRequire = regexp.MustCompile(`^\s*(?:require\s+)?([\w.\-]+(?:/[\w.\-]+)+)\s+(v[^\s]+)`)

func (s *scanner) inspect(f inputFile, a analysis) error {
	base := path.Base(f.name)
	switch base {
	case "package.json":
		if err := s.npmManifest(f); err != nil {
			return err
		}
		// Package declarations handle built-in detection here.
		// Apply user-supplied rules separately.
		if len(s.patterns) > len(patterns) {
			s.references(f, a, s.patterns[len(patterns):])
		}
		return nil
	case "mix.exs":
		s.add("elixir", f, 1, "declaration", "Mix project manifest")
		for i, line := range strings.Split(string(f.data), "\n") {
			if m := mixDep.FindStringSubmatch(line); m != nil {
				value := m[2]
				if value == "" {
					value = "[non-version source reference omitted]"
				}
				scope := ""
				if strings.Contains(line, "only:") {
					scope = "restricted Mix environment"
				}
				s.dependency(f, "hex", m[1], value, "declaration", i+1, scope)
			}
		}
	case "go.mod":
		s.add("go", f, 1, "declaration", "Go module manifest")
		for i, line := range strings.Split(string(f.data), "\n") {
			if strings.Contains(line, "// indirect") {
				continue
			}
			if m := goRequire.FindStringSubmatch(line); m != nil {
				s.dependency(f, "go", m[1], m[2], "declaration", i+1, "")
			}
		}
	case ".tool-versions":
		for i, line := range strings.Split(string(f.data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			id := toolVersionIDs[fields[0]]
			if id != "" {
				s.add(id, f, i+1, "tool pin", fields[0]+" pin "+versionValue(fields[1]))
			}
		}
	case "Dockerfile":
		s.add("docker", f, 1, "configuration", "Container build definition")
	case "Chart.yaml":
		s.add("helm", f, 1, "configuration", "Helm chart metadata")
	}
	if strings.HasSuffix(base, ".Dockerfile") {
		s.add("docker", f, 1, "configuration", "Container build definition")
	}
	if strings.HasPrefix(f.name, ".github/workflows/") {
		s.add("github-actions", f, 1, "configuration", "GitHub Actions workflow definition")
	}
	if strings.HasSuffix(f.name, "ecs-task-definition.json") {
		s.add("amazon-ecs", f, 1, "configuration", "ECS task-definition file")
	}
	if path.Ext(f.name) == ".tf" {
		s.add("terraform", f, 1, "configuration", "Terraform configuration file")
	}
	if id := languageID(a.stat.Language); a.stat.Language != "" && id != "" {
		s.add(id, f, 1, "source reference", "SCC language classification: "+a.stat.Language)
		if strings.HasPrefix(id, "language:") {
			s.found[id].Name = a.stat.Language
			s.found[id].Quadrant = "Languages & Frameworks"
		}
	}
	s.references(f, a, s.patterns)
	return nil
}

func (s *scanner) references(f inputFile, a analysis, rules []sourcePattern) {
	base := path.Base(f.name)
	doc := path.Ext(f.name) == ".md"
	builtin := f.data
	if path.Ext(f.name) == ".go" {
		builtin = goReferenceText(f.data)
	}
	builtinLines := strings.Split(string(builtin), "\n")
	for i, raw := range strings.Split(string(f.data), "\n") {
		line := strings.TrimSpace(raw)
		if !doc && !a.isCodeLine(i) {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		for _, rule := range rules {
			if rule.doc != doc || (len(rule.languages) > 0 && !contains(rule.languages, a.stat.Language)) {
				continue
			}
			text := line
			if !rule.custom {
				text = builtinLines[i]
			}
			if rule.re.MatchString(text) {
				kind := "source reference"
				if doc {
					kind = "documentation"
				} else if path.Ext(f.name) == ".yml" || path.Ext(f.name) == ".yaml" || path.Ext(f.name) == ".json" || strings.HasPrefix(f.name, "config/") || base == "Dockerfile" {
					kind = "configuration"
				}
				s.add(rule.id, f, i+1, kind, rule.label)
			}
		}
	}
}

func (s *scanner) npmManifest(f inputFile) error {
	var obj struct {
		Dependencies map[string]string `json:"dependencies"`
		Dev          map[string]string `json:"devDependencies"`
		Optional     map[string]string `json:"optionalDependencies"`
		Peer         map[string]string `json:"peerDependencies"`
		Engines      map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(f.data, &obj); err != nil {
		return fmt.Errorf("invalid package.json: %w", err)
	}
	for _, section := range []struct {
		name string
		deps map[string]string
	}{{"dependencies", obj.Dependencies}, {"devDependencies", obj.Dev}, {"optionalDependencies", obj.Optional}, {"peerDependencies", obj.Peer}} {
		for name, value := range section.deps {
			s.dependency(f, "npm", name, value, "declaration", jsonKeyLine(f.data, name), section.name)
		}
	}
	if version, ok := obj.Engines["node"]; ok {
		s.add("nodejs", f, jsonKeyLine(f.data, "node"), "declaration", "Node engine constraint "+versionValue(version))
	}
	return nil
}

func jsonKeyLine(data []byte, key string) int {
	quoted, _ := json.Marshal(key)
	index := bytes.Index(data, quoted)
	if index < 0 {
		return 1
	}
	return bytes.Count(data[:index], []byte("\n")) + 1
}

func (s *scanner) npmLock(f inputFile) error {
	var obj struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(f.data, &obj); err != nil {
		return fmt.Errorf("invalid package-lock.json: %w", err)
	}
	for name := range s.declared["npm:"+path.Dir(f.name)] {
		entry, ok := obj.Packages["node_modules/"+name]
		version := entry.Version
		if !ok {
			version = obj.Dependencies[name].Version
		}
		if version != "" {
			s.dependency(f, "npm", name, version, "lock resolution", jsonKeyLine(f.data, "node_modules/"+name), "")
		}
	}
	return nil
}

func (s *scanner) mixLock(f inputFile) {
	for i, line := range strings.Split(string(f.data), "\n") {
		if m := mixLocked.FindStringSubmatch(line); m != nil && s.declared["hex:"+path.Dir(f.name)][m[1]] {
			s.dependency(f, "hex", m[1], m[2], "lock resolution", i+1, "")
		}
	}
}

type sourcePattern struct {
	id, label string
	re        *regexp.Regexp
	doc       bool
	languages []string
	custom    bool
}

func pattern(id, label, expression string) sourcePattern {
	return sourcePattern{id: id, label: label, re: regexp.MustCompile(expression)}
}

func syntaxPattern(id, label, expression string, languages ...string) sourcePattern {
	p := pattern(id, label, expression)
	p.languages = languages
	return p
}

var patterns = []sourcePattern{
	syntaxPattern("phoenix", "Phoenix endpoint/router runtime wiring", `\buse\s+Phoenix\.(?:Endpoint|Router|Controller)\b`, "Elixir"),
	syntaxPattern("phoenix-liveview", "LiveView runtime wiring", `\buse\s+Phoenix\.Live(?:View|Component)\b`, "Elixir"),
	syntaxPattern("ecto", "Ecto repository runtime wiring", `\buse\s+Ecto\.Repo\b`, "Elixir"),
	syntaxPattern("oban", "Oban job runtime wiring", `\buse\s+Oban\.Worker\b|\bOban\.(?:insert|insert!|start_link)\b`, "Elixir"),
	syntaxPattern("broadway", "Broadway consumer runtime wiring", `\buse\s+Broadway\b|\bBroadway\.start_link\b`, "Elixir"),
	syntaxPattern("otp-supervision", "OTP supervision runtime wiring", `\buse\s+(?:Application|Supervisor|DynamicSupervisor)\b|\bSupervisor\.start_link\b`, "Elixir"),
	syntaxPattern("react", "React module runtime wiring", `\bfrom\s+["']react["']|\brequire\(["']react["']\)`, "JavaScript", "JSX", "TypeScript", "Vue", "Svelte", "Astro", "MDX"),
	syntaxPattern("tailwind", "Tailwind style pipeline wiring", `@tailwind\s+(?:base|components|utilities)\b|@import\s+["']tailwindcss["']`, "CSS", "SCSS", "Sass", "LESS", "HTML", "Vue", "Svelte", "Astro"),
	syntaxPattern("ash", "Ash resource/domain reference", `\b(?:use|extensions:)\s+Ash\.`, "Elixir"),
	syntaxPattern("req", "Req HTTP/test reference", `\bReq\.(?:new|get|post|put|delete|Test)\b`, "Elixir"),
	pattern("hubspot", "HubSpot integration configuration/reference", `\bHUBSPOT_[A-Z_]+\b`),
	pattern("postmark", "Postmark integration configuration/reference", `\bPOSTMARK_[A-Z_]+\b|Swoosh\.Adapters\.Postmark`),
	pattern("myutilities", "MyUtilities integration configuration/reference", `\bMYUTILITIES_[A-Z_]+\b`),
	pattern("bindable", "Bindable integration configuration/reference", `\bBINDABLE_[A-Z_]+\b`),
	pattern("local-logic", "Local Logic integration reference", `\bLOCAL_LOGIC_[A-Z_]+\b`),
	pattern("mapbox", "Mapbox integration configuration/reference", `\bMAPBOX_[A-Z_]+\b`),
	pattern("smarty", "Smarty integration configuration/reference", `\bSMARTY_[A-Z_]+\b`),
	pattern("litellm", "LiteLLM gateway configuration/reference", `\bLITELLM_[A-Z_]+\b`),
	pattern("honeycomb", "Honeycomb exporter configuration/reference", `\bHONEYCOMB_[A-Z_]+\b|api\.honeycomb\.io`),
	pattern("opentelemetry", "OpenTelemetry instrumentation reference", `\b(?:OpenTelemetry|OpentelemetryPhoenix|OpentelemetryEcto)\.`),
	pattern("amazon-s3", "S3 API or permission reference", `\bExAws\.S3\b|\bs3:(?:GetObject|PutObject|ListBucket)\b`),
	pattern("amazon-sqs", "SQS consumer/queue reference", `\bBroadwaySQS\b|\bSQS_QUEUE_URL\b`),
	pattern("aws-kms", "KMS API/signer reference", `\bExAws\.KMS\b`),
	pattern("aws-secrets", "AWS secret-store configuration/reference", `secretsmanager:|ssm:|kind:\s*ExternalSecret\b`),
	pattern("amazon-ecs", "ECS delivery configuration/reference", `aws ecs\b|aws-actions/amazon-ecs|requiresCompatibilities`),
	pattern("amazon-eks", "EKS-specific configuration/reference", `eks\.amazonaws\.com|aws eks\b`),
	pattern("kubernetes", "Kubernetes workload/resource reference", `kind:\s*(?:Deployment|StatefulSet|DaemonSet|Pod)\b|\bkubectl\s`),
	pattern("postgresql", "PostgreSQL image declaration", `\bpostgres:(?:[0-9]+|latest)\b`),
	pattern("minio", "MinIO service declaration", `minio/minio|minio server`),
	pattern("nodejs", "Node image/setup reference", `\bnode:[0-9]+|nodesource\.com/setup_[0-9]+|actions/setup-node@`),
	pattern("erlang-otp", "OTP image version default", `\bARG\s+OTP_VERSION=`),
	pattern("k6", "k6 execution reference", `\bk6\s+run\b`),
	{id: "ash-first", label: "Documented Ash-first development guidance", re: regexp.MustCompile(`(?i)ash[- ]first|ash patterns, not raw ecto`), doc: true},
	{id: "traced-http", label: "Documented traced HTTP-client convention", re: regexp.MustCompile(`(?i)include OpenTelemetry tracing|always add OpenTelemetry tracing`), doc: true},
}
