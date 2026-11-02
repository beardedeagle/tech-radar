package radar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// RenderDocument returns Markdown or HTML linked to the full JSON evidence.
// evidenceFile must be a basename; evidenceSHA256 is its checksum.
// The document keeps decision/review state if sent without the evidence file.
// Invalid reports, references or formats return an error. Use Render to embed
// all evidence in one file.
func RenderDocument(report Report, format, evidenceFile, evidenceSHA256 string) ([]byte, error) {
	if evidenceFile == "" || evidenceSHA256 == "" {
		return nil, fmt.Errorf("concise document requires an evidence filename and digest")
	}
	if err := validateReport(report); err != nil {
		return nil, err
	}
	state := report
	state.EvidenceFile, state.EvidenceSHA256 = evidenceFile, evidenceSHA256
	if err := validateReport(state); err != nil {
		return nil, err
	}
	var body []byte
	var err error
	switch format {
	case "markdown":
		body = conciseMarkdown(report, evidenceFile)
	case "html":
		body, err = renderHTMLView(report, true, evidenceFile)
	default:
		return nil, fmt.Errorf("concise document requires markdown or html")
	}
	if err != nil {
		return nil, err
	}
	state.Findings = compactFindings(report.Findings)
	state.ExcludedFindings = compactFindings(report.ExcludedFindings)
	state.Highlights = append([]Finding(nil), report.Highlights...)
	for i := range state.Highlights {
		state.Highlights[i].Evidence = nil
		state.Highlights[i].Assessment = nil
	}
	state.Components, state.Languages = nil, nil
	digest := sha256.Sum256(body)
	saved, err := json.Marshal(snapshot{hex.EncodeToString(digest[:]), state})
	if err != nil {
		return nil, err
	}
	body = append(body, snapshotStart...)
	body = append(body, saved...)
	body = append(body, snapshotEnd...)
	return body, nil
}

func compactFindings(findings []Finding) []Finding {
	if findings == nil {
		return nil
	}
	result := append([]Finding{}, findings...)
	for i := range result {
		result[i].Evidence = nil
		result[i].Assessment, result[i].Proposal, result[i].Members = nil, nil, nil
	}
	return result
}

func displayDecision(f Finding) Decision {
	if f.Decision.Ring != "" || f.Proposal == nil {
		return f.Decision
	}
	d := *f.Proposal
	d.Rationale = displayReason(d.Rationale)
	return d
}

// Saved reports include these explanations in their fingerprints and review
// checks. Change the displayed wording without rewriting that saved state.
func displayReason(reason string) string {
	switch reason {
	case reasonAssess:
		return "Check its use and owner before choosing a ring."
	case reasonTrial:
		return "Try it in a limited scope and review the results with the team."
	case reasonAdopt:
		return "Source evidence shows established use. Confirm its scope and owner."
	case reasonUnknown:
		return "No matching profile yet. Review this finding's role in the application."
	case reasonLocalHelper:
		return "Used for local development. Its presence doesn't make it a core application choice."
	case reasonWorkflow:
		return "Supports development, delivery or operations. Check how it's used and who maintains it."
	case reasonSupporting:
		return "Supporting code or tooling. The evidence doesn't establish a primary runtime role."
	case reasonRestricted, reasonRestrictedPortfolio:
		return "Declared only for a restricted or development scope. Runtime use hasn't been established."
	case reasonPlatform:
		return "May handle data, execution, identity or an external service. Check whether it's enabled and how the application depends on it."
	case reasonFrontend:
		return "A frontend framework, language or component choice in a React application."
	case reasonBeamComponent:
		return "Mix declares Phoenix, Ash or Ecto. Review process ownership, data access and background jobs."
	case reasonReactComponent:
		return "React is a runtime dependency here. Review data flow and the client/server split."
	case reasonPortfolioComponent:
		return "The manifests or configuration match this profile. Review the implementation, deployment and ownership with the team."
	}
	if role, ok := strings.CutPrefix(reason, reasonBeamPrefix); ok {
		return displayRole(strings.TrimSuffix(role, reasonBeamSuffix)) + ". Check its use and owner."
	}
	if profile, ok := strings.CutPrefix(reason, reasonProfilePrefix); ok {
		return "The component evidence matches " + profileName(strings.TrimSuffix(profile, reasonProfileSuffix)) + "."
	}
	return reason
}

// Display labels leave saved profile metadata and fingerprints unchanged.
func profileName(id string) string {
	switch id {
	case "beam_web_data":
		return "BEAM web application"
	case "beam_cqrs_events":
		return "BEAM events and CQRS"
	case "python_ai_api":
		return "Python API and model workflows"
	case "infrastructure_gitops":
		return "Infrastructure and GitOps"
	case "go_service":
		return "Go module"
	}
	for _, p := range profileCatalog() {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

func displayRole(role string) string {
	switch role {
	case "native execution/domain/data boundary":
		return "Runtime, domain model or data access"
	case "delivery/operation/development workflow":
		return "Development, delivery or operations"
	case "platform/integration boundary":
		return "Platform or external service"
	case "UI application boundary":
		return "Frontend application"
	case "supporting code/tooling":
		return "Supporting code or tools"
	case "primary BEAM language":
		return "Main BEAM language"
	case "domain/resource model":
		return "Domain and resource model"
	case "persistence boundary":
		return "Data access"
	case "API schema boundary":
		return "API schema"
	case "identity boundary":
		return "Authentication and authorization"
	case "Go module; service role unverified":
		return "Check whether this module runs a service"
	}
	if label, ok := strings.CutSuffix(role, "; runtime role unverified"); ok {
		return label + "; runtime use needs review"
	}
	return strings.ReplaceAll(role, "/", " and ")
}

func significanceLabel(value string) string {
	switch value {
	case "system_defining":
		return "Core architecture"
	case "workflow_enabling":
		return "Development and operations"
	case "implementation_support":
		return "Supporting code"
	case "unknown":
		return "Needs investigation"
	}
	return value
}

func conciseMarkdown(r Report, evidenceFile string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n# %s Technology Radar\n\n", GeneratedMarker, md(r.Repository))
	fmt.Fprintf(&b, "%d main findings from %d findings across %d scanned files.\n\n", len(r.Highlights), len(r.Findings), r.Source.Files)

	fmt.Fprintf(&b, "[Full evidence and decisions](%s). Source fingerprint: `%s`.\n\n", url.PathEscape(evidenceFile), r.Source.Fingerprint)
	b.WriteString("## Architecture\n\n")
	for _, c := range r.Components {
		b.WriteString("- ")
		if c.Path != "." {
			fmt.Fprintf(&b, "**%s**: ", md(c.Path))
		}
		fmt.Fprintf(&b, "%s (%s). %s\n", md(profileName(c.Profile)), md(displayRole(c.Role)), md(displayReason(c.Reason)))
	}
	if len(r.Components) == 0 {
		b.WriteString("No application profile matched. The report still includes detected tools, integrations and team decisions.\n")
	}
	b.WriteString("\n## Radar\n\n| Quadrant | Adopt | Trial | Assess | Caution | Needs review |\n|---|---|---|---|---|---|\n")
	for _, q := range append(append([]string{}, Quadrants...), "Unclassified") {
		fmt.Fprintf(&b, "| %s", md(q))
		for _, phase := range append(append([]string{}, Rings...), "Needs review") {
			var labels []string
			for _, f := range r.Highlights {
				if quadrant(f) == q && ring(f) == phase {
					labels = append(labels, fmt.Sprintf("%d. %s", f.Number, md(f.Name)))
				}
			}
			fmt.Fprintf(&b, " | %s", strings.Join(labels, "<br>"))
		}
		b.WriteString(" |\n")
	}
	b.WriteString("\n## Main findings\n\nSuggested Adopt reflects established source use; Trial reflects configuration; Assess calls for investigation. Groups follow their strongest use evidence. Suggestions need team review; saved decisions take priority.\n\n| Finding | Importance / role | Confidence | Decision / review |\n|---|---|---|---|\n")
	for _, f := range r.Highlights {
		d := displayDecision(f)
		significance, confidence := "unknown", "unknown"
		if f.Assessment != nil {
			significance = md(significanceLabel(f.Assessment.Significance)) + ": " + md(displayRole(f.Assessment.Role))
			confidence = md(f.Assessment.Confidence)
		}
		note := md(status(f))
		if f.Decision != (Decision{}) {
			note += ". " + md(d.Rationale) + " Scope: " + md(d.Scope)
		}
		if f.Decision.Owner != "" || f.Decision.Reference != "" {
			note += ". " + md(f.Decision.Owner) + "; " + md(f.Decision.Reference) + "; " + md(f.Decision.Reviewed)
		}
		if f.Decision.Lifecycle != "" {
			note += ". Lifecycle: " + md(f.Decision.Lifecycle)
		}
		if len(f.Members) > 1 {
			note += ". Members: " + md(strings.Join(f.Members, ", "))
		}
		fmt.Fprintf(&b, "| %d. %s | %s | %s | %s: %s |\n", f.Number, md(f.Name), significance, confidence, md(ring(f)), note)
	}
	b.WriteString("\n## Other observations and changes\n\n")
	unknown, changed, missing := 0, 0, 0
	for _, f := range r.Findings {
		if f.Assessment != nil && f.Assessment.Significance == "unknown" {
			unknown++
		}
		if f.EvidenceChanged {
			changed++
		}
		if f.Presence == "not detected" {
			missing++
		}
	}
	fmt.Fprintf(&b, "%d findings need investigation; %d have changed evidence; %d are no longer detected. The evidence file includes supporting findings, group members, language counts and source locations.\n\n", unknown, changed, missing)
	for _, id := range r.Excluded {
		fmt.Fprintf(&b, "- Excluded `%s`: %s\n", md(id), md(r.Decisions[id].Rationale))
	}
	b.WriteString("\n## Coverage and maintenance\n\n" + coverage + "\n\n" + boundaries + "\n\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "- **Warning:** %s\n", md(w))
	}
	b.WriteString("\nEdit policy and regenerate with --previous. Commit the linked evidence file to keep the full history. Direct edits to this document stop reimport so they aren't discarded.\n\nMethod: [Thoughtworks Technology Radar](https://www.thoughtworks.com/en-us/radar), Volume 34.\n")
	return []byte(b.String())
}
