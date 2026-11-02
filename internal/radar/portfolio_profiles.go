package radar

import (
	"path"
	"sort"
	"strings"
)

var profileBindings = map[string][]string{
	"jvm_project":         {"jvm", "language:java", "language:kotlin", "language:scala"},
	"dotnet_project":      {"dotnet", "language:csharp", "language:fsharp"},
	"rust_project":        {"language:rust"},
	"beam_cqrs_events":    {"elixir", "erlang-otp", "commanded", "eventstore", "ecto"},
	"node_graphql_api":    {"javascript", "typescript", "nodejs", "apollo-graphql", "pothos", "prisma", "jose"},
	"python_ai_api":       {"python", "fastapi", "pydantic", "langgraph", "model-sdk"},
	"event_driven_worker": {"broadway", "oban", "celery", "sidekiq", "delayed-job"},
	"ruby_rails":          {"ruby", "rails", "activerecord", "sidekiq", "delayed-job"},
	"php_application":     {"language:php", "laravel"},
	"go_service":          {"go"},
	"mobile_client":       {"react-native", "react", "javascript", "typescript"},
}

func profileCatalog() []ContextProfile {
	return []ContextProfile{
		{ID: "jvm_project", Name: "JVM project", Ecosystem: "JVM", Role: "module; runtime role unverified", Concerns: []string{"framework and runtime boundaries", "dependency scope", "thread and resource lifecycle"}, Boundaries: []string{"persistence", "external contracts"}},
		{ID: "dotnet_project", Name: ".NET project", Ecosystem: ".NET", Role: "project; runtime role unverified", Concerns: []string{"hosting and dependency injection", "async cancellation", "data access lifetime"}, Boundaries: []string{"request authority", "persistence"}},
		{ID: "rust_project", Name: "Rust package", Ecosystem: "Rust", Role: "package; runtime role unverified", Concerns: []string{"ownership and error handling", "async runtime", "unsafe and FFI boundaries"}, Boundaries: []string{"resource lifecycle", "external contracts"}},
		{ID: "beam_web_data", Name: "BEAM web/data", Ecosystem: "BEAM", Role: "web/data application", Concerns: []string{"OTP supervision and process ownership", "Phoenix/LiveView boundaries", "Ash/Ecto domain and persistence", "durable jobs and queue backpressure"}, Boundaries: []string{"authorization/tenancy", "transactions", "worker ownership", "external contracts"}},
		{ID: "beam_cqrs_events", Name: "BEAM CQRS/events", Ecosystem: "BEAM", Role: "event-driven domain", Concerns: []string{"commands/aggregates/events", "projection consistency", "event compatibility", "replay and recovery"}, Boundaries: []string{"source of truth", "idempotent handlers", "event/projection ownership"}},
		{ID: "react_typescript_web", Name: "React web", Ecosystem: "JavaScript/TypeScript", Role: "UI application", Concerns: []string{"routing/rendering", "client/server split", "state/data caching", "form/schema validation"}, Boundaries: []string{"identity/tenant data", "browser credentials", "API contracts"}},
		{ID: "node_graphql_api", Name: "Node GraphQL API", Ecosystem: "JavaScript/TypeScript", Role: "API service candidate", Concerns: []string{"schema/resolver authorization", "data access/pooling", "token verification", "execution lifecycle"}, Boundaries: []string{"data ownership", "API compatibility", "identity"}},
		{ID: "python_ai_api", Name: "Python AI/API", Ecosystem: "Python", Role: "API/model workflow candidate", Concerns: []string{"API/worker separation", "async/retry behavior", "typed contracts", "model/tool/evaluation provenance"}, Boundaries: []string{"data handling", "cost/side effects", "provider contracts"}},
		{ID: "event_driven_worker", Name: "Event-driven work", Ecosystem: "composable", Role: "durable work candidate", Concerns: []string{"idempotency", "ack/retry/dead-letter behavior", "backpressure/scaling", "reconciliation"}, Boundaries: []string{"queue/worker ownership", "external writes", "result lifecycle"}},
		{ID: "ruby_rails", Name: "Rails", Ecosystem: "Ruby", Role: "Rails application candidate", Concerns: []string{"routes/controllers/models", "transactions/migrations", "job lifecycle", "retained API compatibility"}, Boundaries: []string{"data/schema ownership", "identity/storage", "job effects"}},
		{ID: "php_application", Name: "PHP application", Ecosystem: "PHP", Role: "application candidate", Concerns: []string{"actual framework/router", "authorization middleware", "transactions", "deployment lifecycle"}, Boundaries: []string{"request authority", "persistence", "external APIs"}},
		{ID: "infrastructure_gitops", Name: "Infrastructure/GitOps", Ecosystem: "infrastructure", Role: "infrastructure candidate", Concerns: []string{"state/resource ownership", "declarative delivery", "rollout/rollback", "workload identity/secrets"}, Boundaries: []string{"shared-platform impact", "privilege/trust", "recovery"}},
		{ID: "plugin_artifact", Name: "Plugin artifacts", Ecosystem: "artifact", Role: "plugin/tool contract", Concerns: []string{"capability manifests", "authorization scopes", "host contracts", "packaging reproducibility"}, Boundaries: []string{"tool authority", "data exposure", "host compatibility"}},
		{ID: "go_service", Name: "Go module/service", Ecosystem: "Go", Role: "Go module; service role unverified", Concerns: []string{"establish actual HTTP/RPC boundary", "cancellation/timeouts", "errors/concurrency", "external-process ownership"}, Boundaries: []string{"resource lifecycle", "data handling", "deployment compatibility"}},
		{ID: "mobile_client", Name: "Mobile client", Ecosystem: "JavaScript/native", Role: "mobile client candidate", Concerns: []string{"native bridges", "state/offline behavior", "identity", "release channels"}, Boundaries: []string{"device/backend contract", "credential storage", "retained-client compatibility"}},
	}
}

func knownProfile(id string) bool {
	for _, p := range profileCatalog() {
		if p.ID == id {
			return true
		}
	}
	return false
}

func portfolioComponents(findings []Finding, selected map[string]bool) []Component {
	byDir := map[string]map[string]bool{}
	for _, f := range findings {
		for _, e := range f.Evidence {
			if e.Kind != "declaration" && e.Kind != "configuration" && e.Kind != "documentation" {
				continue
			}
			if restrictedDeclaration(e) {
				continue
			}
			dir := path.Dir(e.Path)
			if byDir[dir] == nil {
				byDir[dir] = map[string]bool{}
			}
			// Dev and peer dependencies don't establish a browser/API runtime.
			if path.Base(e.Path) == "package.json" && e.Kind == "declaration" && !strings.HasSuffix(e.Detail, "[dependencies]") && !strings.HasSuffix(e.Detail, "[optionalDependencies]") {
				continue
			}
			byDir[dir][f.ID] = true
		}
	}
	var result []Component
	for _, dir := range sortedKeys(byDir) {
		ids := byDir[dir]
		has := func(values ...string) bool {
			for _, id := range values {
				if ids[id] {
					return true
				}
			}
			return false
		}
		matches := map[string]bool{
			"jvm_project":           ids["jvm"],
			"dotnet_project":        ids["dotnet"],
			"rust_project":          ids["language:rust"],
			"beam_web_data":         ids["elixir"] && has("phoenix", "ash", "ecto"),
			"beam_cqrs_events":      ids["elixir"] && has("commanded", "eventstore"),
			"react_typescript_web":  ids["react"] && !ids["react-native"],
			"node_graphql_api":      has("apollo-graphql", "pothos"),
			"python_ai_api":         ids["python"] && ids["fastapi"] && has("langgraph", "model-sdk"),
			"event_driven_worker":   has("broadway", "oban", "celery", "sidekiq", "delayed-job"),
			"ruby_rails":            ids["ruby"] && ids["rails"],
			"php_application":       ids["language:php"],
			"infrastructure_gitops": has("terraform", "helm", "argocd"),
			"plugin_artifact":       ids["plugin-artifact"],
			"go_service":            ids["go"],
			"mobile_client":         ids["react-native"],
		}
		for _, p := range profileCatalog() {
			if matches[p.ID] && (selected == nil || selected[p.ID]) {
				result = append(result, Component{Path: dir, Ecosystem: p.Ecosystem, Role: p.Role, Profile: p.ID, Reason: reasonPortfolioComponent})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Profile < result[j].Profile
	})
	return result
}

func assessPortfolio(f Finding, components []Component) Assessment {
	return assessPortfolioVersion(f, components, PortfolioVersion)
}

// Old contexts rebuild with the assessment rules used to create them.
func assessPortfolioVersion(f Finding, components []Component, version string) Assessment {
	legacy := version == legacyPortfolioVersion
	if !legacy {
		f.ID = canonicalFindingID(f.ID)
	}
	profiles := map[string]bool{}
	for _, e := range f.Evidence {
		nearest := -1
		local := map[string]bool{}
		for _, c := range components {
			if c.Path == "." || strings.HasPrefix(e.Path, c.Path+"/") {
				depth := len(c.Path)
				// Earlier contexts retain the root/one-character tie for exact rebuilds.
				if c.Path == "." && version != legacyPortfolioVersion && version != "portfolio-2" {
					depth = 0
				}
				if depth > nearest {
					nearest = depth
					local = map[string]bool{}
				}
				if depth == nearest {
					local[c.Profile] = true
				}
			}
		}
		for id := range local {
			profiles[id] = true
		}
	}
	a := assessWithProfiles(f, profiles)

	for _, id := range sortedKeys(profiles) {
		if contains(profileBindings[id], f.ID) {
			a.Significance = "system_defining"
			a.Role = "native execution/domain/data boundary"
			a.Scope = "component"
			a.Reason = reasonProfilePrefix + id + reasonProfileSuffix
			break
		}
	}
	// A profile match doesn't make a restricted/dev dependency part of the runtime.
	scoped, runtime := false, false
	for _, e := range f.Evidence {
		if !legacy && f.ID == "typescript" && profiles["react_typescript_web"] && authoredTypeScript(e) {
			runtime = true
		}
		if e.Kind == "declaration" {
			if restrictedDeclaration(e) {
				scoped = true
			} else {
				runtime = true
			}
		}
	}
	if scoped && !runtime && a.Significance == "system_defining" && a.Confidence != "static wiring" {
		a.Significance, a.Role, a.Scope, a.Reason = "implementation_support", "non-runtime dependency", "development", reasonRestrictedPortfolio
	}
	return a
}

func restrictedDeclaration(e Evidence) bool {
	if e.Kind != "declaration" {
		return false
	}
	for _, scope := range []string{"[development]", "[restricted]", "[restricted Mix environment]", "[devDependencies]", "[peerDependencies]"} {
		if strings.HasSuffix(e.Detail, scope) {
			return true
		}
	}
	return false
}
