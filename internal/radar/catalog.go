package radar

import (
	"strings"
	"sync"
)

type Technology struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Quadrant string   `json:"quadrant"`
	Hex      []string `json:"hex,omitempty"`
	NPM      []string `json:"npm,omitempty"`
}

// The catalog names technologies and quadrants. Policy supplies radar rings.
func Catalog() []Technology { return append(baseCatalog(), portfolioCatalog()...) }

func baseCatalog() []Technology {
	return []Technology{
		{"otp-supervision", "OTP supervision", "Techniques", nil, nil},
		{"ash-first", "Ash-first domain modeling", "Techniques", nil, nil},
		{"feature-flags", "Actor-scoped feature flags", "Techniques", nil, nil},
		{"traced-http", "Instrumented HTTP boundaries", "Techniques", nil, nil},
		{"postgresql", "PostgreSQL", "Platforms", nil, nil},
		{"amazon-ecs", "Amazon ECS / Fargate", "Platforms", nil, nil},
		{"amazon-eks", "Amazon EKS", "Platforms", nil, nil},
		{"kubernetes", "Kubernetes", "Platforms", nil, nil},
		{"amazon-s3", "Amazon S3", "Platforms", []string{"ex_aws_s3"}, nil},
		{"amazon-sqs", "Amazon SQS", "Platforms", []string{"broadway_sqs"}, nil},
		{"aws-kms", "AWS KMS", "Platforms", []string{"ex_aws_kms"}, nil},
		{"aws-secrets", "AWS SSM / Secrets Manager", "Platforms", nil, nil},
		{"honeycomb", "Honeycomb", "Platforms", nil, nil},
		{"hubspot", "HubSpot", "Platforms", nil, nil},
		{"postmark", "Postmark", "Platforms", nil, nil},
		{"myutilities", "MyUtilities", "Platforms", nil, nil},
		{"bindable", "Bindable", "Platforms", nil, nil},
		{"retool", "Retool", "Platforms", nil, nil},
		{"local-logic", "Local Logic", "Platforms", nil, []string{"@local-logic/sdks-js"}},
		{"mapbox", "Mapbox", "Platforms", nil, []string{"mapbox-gl"}},
		{"smarty", "Smarty", "Platforms", nil, nil},
		{"litellm", "LiteLLM gateway", "Platforms", nil, nil},
		{"docker", "Docker", "Tools", nil, nil},
		{"github-actions", "GitHub Actions", "Tools", nil, nil},
		{"helm", "Helm", "Tools", nil, nil},
		{"terraform", "Terraform", "Tools", nil, nil},
		{"req", "Req / Req.Test", "Tools", []string{"req"}, nil},
		{"opentelemetry", "OpenTelemetry", "Tools", []string{"opentelemetry", "opentelemetry_api", "opentelemetry_exporter", "opentelemetry_ash", "opentelemetry_bandit", "opentelemetry_ecto", "opentelemetry_phoenix"}, nil},
		{"sentry", "Sentry", "Tools", []string{"sentry"}, []string{"@sentry/browser", "@sentry/cli", "@sentry/node", "@sentry/react"}},
		{"fun-with-flags", "FunWithFlags", "Tools", []string{"fun_with_flags", "fun_with_flags_ui"}, nil},
		{"mox", "Mox", "Tools", []string{"mox"}, nil},
		{"excoveralls", "ExCoveralls", "Tools", []string{"excoveralls"}, nil},
		{"credo", "Credo", "Tools", []string{"credo"}, nil},
		{"dialyzer", "Dialyzer / Dialyxir", "Tools", []string{"dialyxir"}, nil},
		{"sobelow", "Sobelow", "Tools", []string{"sobelow"}, nil},
		{"mix-audit", "Mix Audit", "Tools", []string{"mix_audit"}, nil},
		{"playwright", "Playwright", "Tools", nil, []string{"@playwright/test", "playwright"}},
		{"esbuild", "esbuild", "Tools", []string{"esbuild"}, []string{"esbuild"}},
		{"eslint", "ESLint", "Tools", nil, []string{"eslint", "@eslint/js", "@eslint/css", "@eslint/markdown"}},
		{"minio", "MinIO", "Tools", nil, nil},
		{"tidewave", "Tidewave", "Tools", []string{"tidewave"}, nil},
		{"ash-ai", "Ash AI", "Tools", []string{"ash_ai"}, nil},
		{"hotjar", "Hotjar", "Tools", nil, []string{"@hotjar/browser"}},
		{"k6", "k6", "Tools", nil, []string{"k6"}},
		{"aws-copilot", "AWS Copilot CLI", "Tools", nil, nil},
		{"elixir", "Elixir", "Languages & Frameworks", nil, nil},
		{"erlang-otp", "Erlang / OTP", "Languages & Frameworks", nil, nil},
		{"phoenix", "Phoenix", "Languages & Frameworks", []string{"phoenix"}, nil},
		{"phoenix-liveview", "Phoenix LiveView", "Languages & Frameworks", []string{"phoenix_live_view"}, nil},
		{"ash", "Ash Framework", "Languages & Frameworks", []string{"ash", "ash_admin", "ash_phoenix", "ash_postgres", "ash_json_api", "ash_jason", "ash_paper_trail"}, nil},
		{"ecto", "Ecto / Postgrex", "Languages & Frameworks", []string{"ecto_sql", "ecto", "postgrex", "phoenix_ecto"}, nil},
		{"oban", "Oban / Oban Pro", "Languages & Frameworks", []string{"oban", "oban_pro", "oban_web"}, nil},
		{"broadway", "Broadway", "Languages & Frameworks", []string{"broadway"}, nil},
		{"ash-authentication", "Ash Authentication", "Languages & Frameworks", []string{"ash_authentication", "ash_authentication_phoenix"}, nil},
		{"graphql", "Absinthe / AshGraphql", "Languages & Frameworks", []string{"absinthe", "absinthe_phoenix", "ash_graphql"}, nil},
		{"bandit", "Bandit", "Languages & Frameworks", []string{"bandit"}, nil},
		{"javascript", "JavaScript", "Languages & Frameworks", nil, nil},
		{"typescript", "TypeScript", "Languages & Frameworks", nil, []string{"typescript"}},
		{"nodejs", "Node.js", "Languages & Frameworks", nil, nil},
		{"tailwind", "Tailwind CSS", "Languages & Frameworks", []string{"tailwind"}, []string{"tailwindcss", "@tailwindcss/cli"}},
		{"daisyui", "daisyUI", "Languages & Frameworks", []string{"daisy_ui_components"}, []string{"daisyui"}},
		{"langchain", "LangChain", "Languages & Frameworks", []string{"langchain"}, []string{"langchain", "@langchain/core"}},
		{"react", "React", "Languages & Frameworks", nil, []string{"react", "react-dom"}},
		{"nextjs", "Next.js", "Languages & Frameworks", nil, []string{"next"}},
		{"go", "Go", "Languages & Frameworks", nil, nil},
		{"python", "Python", "Languages & Frameworks", nil, nil},
		{"shell", "Shell scripts", "Languages & Frameworks", nil, nil},
		{"ruby", "Ruby", "Languages & Frameworks", nil, nil},
	}
}

var canonicalAliases = sync.OnceValue(func() map[string]string {
	aliases := map[string]string{}
	for _, tech := range Catalog() {
		for _, name := range tech.NPM {
			aliases["npm:"+name] = tech.ID
		}
		for _, name := range tech.Hex {
			aliases["hex:"+name] = tech.ID
		}
	}
	return aliases
})

// canonicalFindingID resolves maintained aliases without changing saved IDs.
func canonicalFindingID(id string) string {
	if canonical := canonicalAliases()[id]; canonical != "" {
		return canonical
	}
	return id
}

// retainSavedPackageIDs lets new recognition classify an existing finding
// without detaching its decision, exclusion or display number. Aliases of the
// same technology retain their own package declarations and common references.
func retainSavedPackageIDs(current, prior map[string]Finding, decisions map[string]Decision) {
	preserve := map[string]bool{}
	for id := range prior {
		preserve[id] = true
	}
	for id := range decisions {
		preserve[id] = true
	}
	consumed := map[string]map[Evidence]bool{}
	for _, id := range sortedKeys(preserve) {
		canonical := canonicalFindingID(id)
		if canonical == id {
			continue
		}
		detected, exists := current[canonical]
		if !exists {
			continue
		}
		ecosystem, name, _ := strings.Cut(id, ":")
		prefix := ecosystem + " package " + name + ":"
		var evidence []Evidence
		declared := false
		for _, e := range detected.Evidence {
			if e.Kind == "declaration" || e.Kind == "lock resolution" {
				if !strings.HasPrefix(e.Detail, prefix) {
					continue
				}
				declared = true
				if consumed[canonical] == nil {
					consumed[canonical] = map[Evidence]bool{}
				}
				consumed[canonical][e] = true
			}
			evidence = append(evidence, e)
		}
		if !declared {
			continue
		}
		detected.ID, detected.Evidence = id, evidence
		detected.EvidenceCount = len(evidence)
		detected.EvidenceFingerprint = fingerprintEvidence(evidence)
		current[id] = detected
	}
	for id, assigned := range consumed {
		if preserve[id] {
			continue
		}
		f := current[id]
		var remainder []Evidence
		hasPackage := false
		for _, e := range f.Evidence {
			if assigned[e] {
				continue
			}
			remainder = append(remainder, e)
			hasPackage = hasPackage || e.Kind == "declaration" || e.Kind == "lock resolution"
		}
		if !hasPackage {
			delete(current, id)
			continue
		}
		f.Evidence = remainder
		f.EvidenceCount = len(remainder)
		f.EvidenceFingerprint = fingerprintEvidence(remainder)
		current[id] = f
	}
}
