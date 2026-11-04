package radar

import (
	"path"
	"slices"
	"sort"
	"strings"
)

const ruleVersion = "context-4"

// These explanations are saved in reports and context fingerprints.
// Changing their values requires a compatible saved-state migration.
const (
	reasonAssess              = "Investigate this declared/referenced architectural choice and confirm its scope, ownership and current use before recommending commitment."
	reasonTrial               = "Proposed bounded evaluation of this statically wired/configured boundary; validate behavior and operational fit with the team. Source wiring is not approval."
	reasonAdopt               = "Source evidence shows this technology in use in this repository. Confirm its scope and ownership with the team. This is a suggested placement, not approval."
	reasonUnknown             = "No applicable architectural rule; retain for investigation."
	reasonLocalHelper         = "Local development helper; presence alone does not define the application or a material delivery boundary."
	reasonWorkflow            = "Enables a material delivery, operational, quality or observability workflow; assess its actual use and owner."
	reasonSupporting          = "Presence does not establish a primary runtime or architectural boundary in the detected component; retain evidence without promoting it."
	reasonRestricted          = "Only a restricted/development/peer declaration supports this dependency; no runtime wiring establishes an architectural role."
	reasonRestrictedPortfolio = "Only restricted/development declarations support this dependency; no runtime wiring establishes an architectural role."
	reasonPlatform            = "Owns a potential data, execution, identity or external-system boundary; live enablement and business criticality require review."
	reasonFrontend            = "Native UI framework, language or component-system choice in a manifest-backed React component."
	reasonBeamComponent       = "Mix declares Phoenix, Ash or Ecto; inspect OTP, domain, persistence and job boundaries."
	reasonReactComponent      = "React is a runtime dependency in this component's package manifest; inspect UI/data and client/server boundaries."
	reasonPortfolioComponent  = "Manifest/configuration signals select a candidate native checklist; deployment, implementation quality and ownership remain unverified."
	reasonBeamPrefix          = "Native BEAM/web architectural role: "
	reasonBeamSuffix          = ". Confirm scope and operational ownership with the team."
	reasonProfilePrefix       = "The locally applicable "
	reasonProfileSuffix       = " profile identifies this role; portfolio frequency does not determine significance."
)

var beamRoles = map[string]string{"elixir": "primary BEAM language", "erlang-otp": "BEAM runtime", "phoenix": "web execution", "phoenix-liveview": "server-driven UI", "ash": "domain/resource model", "ecto": "persistence boundary", "oban": "durable jobs", "broadway": "queue consumption and backpressure", "ash-authentication": "identity boundary", "graphql": "API schema boundary", "bandit": "HTTP execution", "otp-supervision": "process ownership and failure isolation", "ash-first": "domain modeling convention", "tailwind": "UI styling system", "daisyui": "UI component family", "langchain": "model orchestration"}

func assessWithProfiles(f Finding, profiles map[string]bool) Assessment {
	a := Assessment{Significance: "unknown", Confidence: "unknown", Role: "unresolved", Scope: "repository", Reason: reasonUnknown}
	for _, e := range f.Evidence {
		confidence := map[string]string{"declaration": "declaration", "tool pin": "declaration", "lock resolution": "declaration", "configuration": "configuration", "source reference": "reference", "documentation": "documentation"}[e.Kind]
		if strings.Contains(e.Detail, "runtime wiring") || strings.Contains(e.Detail, "pipeline wiring") || (f.ID == "ash" && e.Kind == "source reference") {
			confidence = "static wiring"
		}
		if confidenceRank(confidence) > confidenceRank(a.Confidence) {
			a.Confidence = confidence
		}
	}
	if len(profiles) > 0 {
		a.Profiles = sortedKeys(profiles)
	}
	beam := profiles["beam_web_data"]
	ui := profiles["react_typescript_web"]
	helperOnly := len(f.Evidence) > 0
	for _, e := range f.Evidence {
		if !strings.HasPrefix(e.Path, "bin/") && !strings.HasPrefix(e.Path, "scripts/") && !strings.HasPrefix(e.Path, "tools/") {
			helperOnly = false
		}
	}
	system := func(role, reason string) {
		a.Significance, a.Role, a.Scope, a.Reason = "system_defining", role, "runtime", reason
	}
	if beam && beamRoles[f.ID] != "" {
		system(beamRoles[f.ID], reasonBeamPrefix+beamRoles[f.ID]+reasonBeamSuffix)
	} else if ui && contains([]string{"react", "nextjs", "javascript", "typescript", "tailwind", "daisyui"}, f.ID) {
		system("UI application boundary", reasonFrontend)
	} else if f.Quadrant == "Platforms" && f.ID != "honeycomb" {
		system("platform/integration boundary", reasonPlatform)
	} else if f.ID == "tidewave" {
		a.Significance, a.Role, a.Scope, a.Reason = "implementation_support", "developer convenience", "development", reasonLocalHelper
	} else if f.Quadrant == "Tools" || contains([]string{"honeycomb", "feature-flags", "traced-http"}, f.ID) {
		a.Significance, a.Role, a.Scope, a.Reason = "workflow_enabling", "delivery/operation/development workflow", "workflow", reasonWorkflow
	} else if (contains([]string{"python", "shell", "nodejs", "javascript", "typescript", "ruby"}, f.ID) && (beam || ui || helperOnly)) || (ui && strings.HasPrefix(f.ID, "npm:@radix-ui/")) {
		a.Significance, a.Role, a.Scope, a.Reason = "implementation_support", "supporting code/tooling", "unknown", reasonSupporting
	}
	// Dev dependencies alone don't establish a runtime framework.
	// Tools and source wiring can still show a separate role.
	if a.Significance == "system_defining" && a.Confidence != "static wiring" {
		runtime, scoped := false, false
		for _, e := range f.Evidence {
			if ui && f.ID == "typescript" && authoredTypeScript(e) {
				runtime = true
			}
			if e.Kind != "declaration" {
				continue
			}
			if path.Base(e.Path) == "package.json" {
				scoped = true
				runtime = runtime || strings.HasSuffix(e.Detail, "[dependencies]") || strings.HasSuffix(e.Detail, "[optionalDependencies]")
			}
			if strings.Contains(e.Detail, "[restricted Mix environment]") {
				scoped = true
			} else if path.Base(e.Path) == "mix.exs" {
				runtime = true
			}
		}
		if scoped && !runtime {
			a.Significance, a.Role, a.Scope, a.Reason = "implementation_support", "non-runtime dependency", "development", reasonRestricted
		}
	}
	return a
}

func confidenceRank(value string) int {
	return map[string]int{"unknown": 0, "documentation": 1, "reference": 2, "declaration": 3, "configuration": 4, "static wiring": 5}[value]
}

func authoredTypeScript(e Evidence) bool {
	return e.Kind == "source reference" && (path.Ext(e.Path) == ".tsx" || strings.HasPrefix(e.Path, "src/") || strings.Contains(e.Path, "/src/"))
}

// establishedUse distinguishes implementation from declarations and settings.
// It describes repository use; it cannot establish live deployment or approval.
func establishedUse(f Finding) bool {
	declared, referenced := false, false
	manifests := map[string]bool{}
	for _, e := range f.Evidence {
		if e.Kind == "declaration" {
			manifests[e.Path] = true
			declared = declared || !restrictedDeclaration(e)
		}
	}
	for _, e := range f.Evidence {
		if e.Kind != "source reference" || manifests[e.Path] {
			continue
		}
		helper := false
		for _, segment := range strings.Split(e.Path, "/") {
			if strings.HasPrefix(segment, ".") || contains([]string{"bin", "scripts", "tools"}, segment) {
				helper = true
				break
			}
		}
		if helper {
			continue
		}
		if strings.Contains(e.Detail, "runtime wiring") || strings.Contains(e.Detail, "pipeline wiring") || (f.ID == "ash" && strings.Contains(e.Detail, "Ash resource/domain reference")) {
			return true
		}
		referenced = true
	}
	return declared && referenced
}

// synthesizeWithGrouping assesses and groups findings for the report.
// Decisions, evidence and technology IDs stay unchanged.
func synthesizeWithGrouping(r *Report, grouping GroupingRules) {
	r.RuleVersion = ruleVersion
	r.Highlights = nil
	byID := map[string]Finding{}
	for i := range r.Findings {
		f := &r.Findings[i]
		a := assessPortfolio(*f, r.Components)
		f.Assessment, f.Proposal, f.Members = &a, nil, nil
		f.FamilyID, f.GroupComponent = "", ""
		if r.Model != LifecycleModel {
			f.Proposal = suggestPlacement(*f, a)
		}
		byID[f.ID] = *f
	}
	roots, families, scopes := assignFamilies(byID, grouping)
	groups := map[string][]string{}
	for _, f := range r.Findings {
		root := f.ID
		if f.Decision == (Decision{}) || r.Model == LifecycleModel {
			if family, ok := roots[f.ID]; ok {
				root = family
			}
		}
		parent, ok := byID[root]
		if !ok || parent.Presence != "detected" || (parent.Quadrant == "" && families[root].Quadrant == "") || (root != f.ID && (f.Presence != "detected" || !groupableDecisions(r.Model, parent.Decision, f.Decision))) {
			root = f.ID
		}
		groups[root] = append(groups[root], f.ID)
	}
	for _, id := range sortedKeys(groups) {
		f := byID[id]
		f.Members = groups[id]
		sort.Strings(f.Members)
		assessment := *f.Assessment
		f.Assessment = &assessment
		family, recognized := families[id]
		if recognized && (f.Decision == (Decision{}) || r.Model == LifecycleModel) {
			if roots[id] == id {
				f.FamilyID, f.GroupComponent = family.ID, scopes[id]
			}
			if f.Quadrant == "" {
				f.Quadrant = family.Quadrant
			}
			if f.Decision.Name == "" && (len(f.Members) > 1 || (f.Name == f.ID && roots[id] == id)) {
				f.Name = family.Name
			}
			if assessment.Significance == "unknown" {
				assessment = familyAssessment(f, family)
				if r.Model != LifecycleModel {
					f.Proposal = suggestPlacement(f, assessment)
				}
			}
		}
		for _, member := range f.Members {
			m := byID[member]
			if recognized && m.Assessment.Significance == "unknown" && m.Decision == (Decision{}) {
				m.Quadrant = family.Quadrant
				a := familyAssessment(m, family)
				m.Assessment = &a
				if r.Model != LifecycleModel {
					m.Proposal = suggestPlacement(m, a)
				}
			}
			if significanceRank(m.Assessment.Significance) > significanceRank(assessment.Significance) {
				assessment.Significance, assessment.Role, assessment.Scope, assessment.Reason = m.Assessment.Significance, m.Assessment.Role, m.Assessment.Scope, m.Assessment.Reason
			}
			if confidenceRank(m.Assessment.Confidence) > confidenceRank(assessment.Confidence) {
				assessment.Confidence = m.Assessment.Confidence
			}
			if m.Proposal != nil && (f.Proposal == nil || slices.Index(Rings, m.Proposal.Ring) < slices.Index(Rings, f.Proposal.Ring)) {
				f.Proposal = m.Proposal
			}
			if m.ReviewRequired {
				f.ReviewRequired = true
			}
			if m.MissingDecisionMaterial {
				f.MissingDecisionMaterial, f.ReviewRequired = true, true
			}
			if m.EvidenceChanged {
				f.EvidenceChanged, f.ReviewRequired = true, true
			}
		}
		if r.Model != LifecycleModel && f.Assessment.Significance != "system_defining" && f.Assessment.Significance != "workflow_enabling" && f.Decision == (Decision{}) {
			continue
		}
		// Reference original findings instead of duplicating their source evidence.
		f.Evidence = nil
		r.Highlights = append(r.Highlights, f)
	}
	// Importance sets display order, not the radar ring.
	sort.SliceStable(r.Highlights, func(i, j int) bool {
		a, b := significanceRank(r.Highlights[i].Assessment.Significance), significanceRank(r.Highlights[j].Assessment.Significance)
		if a != b {
			return a > b
		}
		return r.Highlights[i].ID < r.Highlights[j].ID
	})
}

func suggestPlacement(f Finding, a Assessment) *Decision {
	if f.Presence != "detected" || f.Quadrant == "" || (a.Significance != "system_defining" && a.Significance != "workflow_enabling") {
		return nil
	}
	phase, reason := "Assess", reasonAssess
	if a.Confidence == "configuration" || a.Confidence == "static wiring" {
		phase, reason = "Trial", reasonTrial
	}
	if establishedUse(f) {
		phase, reason = "Adopt", reasonAdopt
	}
	return &Decision{Ring: phase, Scope: a.Scope + " in this repository", Rationale: reason, Status: "proposed"}
}

func familyAssessment(f Finding, family groupingFamily) Assessment {
	a := *f.Assessment
	a.Significance, a.Role, a.Scope = family.Significance, strings.ReplaceAll(family.Relationship, "_", " "), "component"
	a.Reason = "Grouped as " + family.Name + ". Review each member's evidence before deciding."
	scoped, runtime := false, false
	for _, e := range f.Evidence {
		if e.Kind == "declaration" {
			scoped = scoped || restrictedDeclaration(e)
			runtime = runtime || !restrictedDeclaration(e)
		}
	}
	if scoped && !runtime && a.Significance == "system_defining" && a.Confidence != "static wiring" {
		a.Significance, a.Role, a.Scope, a.Reason = "implementation_support", "non-runtime dependency", "development", reasonRestrictedPortfolio
	}
	return a
}
