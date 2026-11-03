package radar

import (
	"fmt"
	"sort"
	"strings"
)

type tableGroup struct {
	Quadrant string
	Findings []Finding
}

func decisionGroups(r Report, concise bool) []tableGroup {
	rows := append([]Finding(nil), r.Highlights...)
	if !concise {
		included := map[string]bool{}
		for _, f := range rows {
			included[f.ID] = true
			for _, id := range f.Members {
				included[id] = true
			}
		}
		for _, f := range r.Findings {
			if !included[f.ID] {
				rows = append(rows, f)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].FamilyID != rows[j].FamilyID {
			return rows[i].FamilyID < rows[j].FamilyID
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Number < rows[j].Number
	})
	var groups []tableGroup
	for _, q := range append(append([]string{}, Quadrants...), "Unclassified") {
		group := tableGroup{Quadrant: q}
		for _, f := range rows {
			if quadrant(f) == q {
				group.Findings = append(group.Findings, f)
			}
		}
		if len(group.Findings) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func tableMembers(r Report, f Finding) []Finding {
	ids := f.Members
	if len(ids) == 0 {
		ids = []string{f.ID}
	}
	byID := make(map[string]Finding, len(r.Findings))
	for _, member := range r.Findings {
		byID[member.ID] = member
	}
	members := make([]Finding, 0, len(ids))
	for _, id := range ids {
		members = append(members, byID[id])
	}
	return members
}

func stateClass(model Model, f Finding) string {
	switch model.ring(f) {
	case "Adopt":
		return "state-adopt"
	case "Experiment", "Trial":
		return "state-trial"
	case "Hold", "Assess":
		return "state-hold"
	case "Retire", "Caution":
		return "state-retire"
	default:
		return "state-pending"
	}
}

func placementNote(f Finding) string {
	if c := f.PlacementChange; c != nil {
		switch c.Kind {
		case "new":
			return "New entry"
		case "unchanged":
			return "Placement unchanged"
		case "moved-in":
			return "Moved inward from " + c.PreviousRing
		case "moved-out":
			return "Moved outward from " + c.PreviousRing
		default:
			return "Previously " + c.PreviousQuadrant + " / " + c.PreviousRing
		}
	}
	return "No comparable previous placement"
}

// Table cells treat supplied text as text, including Markdown link syntax.
func tableText(s string) string {
	s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "&#124;", "`", "&#96;", "\\", "&#92;", "*", "&#42;", "_", "&#95;", "[", "&#91;", "]", "&#93;", "\r", " ", "\n", " ").Replace(s)
	if s == "" {
		return "Not supplied"
	}
	return s
}

func markdownDecisionTable(b *strings.Builder, r Report, concise bool) {
	b.WriteString("\n## Decisions\n\n")
	for _, group := range decisionGroups(r, concise) {
		fmt.Fprintf(b, "### %s\n\n| Technology / family | State | Scope | Owner | Decision reference | Reviewed | Changes / review | Evidence / members |\n|---|---|---|---|---|---|---|---|\n", tableText(group.Quadrant))
		for _, f := range group.Findings {
			d := displayDecision(f)
			name := fmt.Sprintf("%d. %s", f.Number, tableText(f.Name))
			if f.FamilyID != "" {
				name += "<br>Family: " + tableText(f.FamilyID)
			}
			if f.GroupComponent != "" && f.GroupComponent != "." {
				name += "<br>Component: " + tableText(f.GroupComponent)
			}
			if d.Rationale != "" {
				name += "<br>Rationale: " + tableText(d.Rationale)
			}
			if r.Model != LifecycleModel && f.Decision.Lifecycle != "" {
				name += "<br>Lifecycle: " + tableText(f.Decision.Lifecycle)
			}
			var evidence []string
			for _, member := range tableMembers(r, f) {
				note := tableText(member.ID)
				if !concise {
					for _, e := range member.Evidence {
						note += fmt.Sprintf("<br>%s:%d, %s: %s", tableText(e.Path), e.Line, tableText(e.Kind), tableText(e.Detail))
					}
					if member.EvidenceCount > len(member.Evidence) {
						note += fmt.Sprintf("<br>Showing %d of %d references", len(member.Evidence), member.EvidenceCount)
					} else if len(member.Evidence) == 0 {
						note += "<br>No repository evidence available"
					}
				}
				evidence = append(evidence, note)
			}
			if concise {
				evidence = append(evidence, "See linked evidence file")
			}
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s<br>%s | %s |\n", name, tableText(r.Model.ring(f)), tableText(d.Scope), tableText(f.Decision.Owner), tableText(f.Decision.Reference), tableText(f.Decision.Reviewed), tableText(status(f)), tableText(placementNote(f)), strings.Join(evidence, "<br>"))
		}
		b.WriteString("\n")
	}
}

const decisionTableStyles = `
.decision-table{min-width:1000px;table-layout:fixed}.decision-table caption{text-align:left;font-size:22px;font-weight:650;padding:12px 0}.decision-table th,.decision-table td{vertical-align:top;overflow-wrap:anywhere}.decision-table th:first-child{width:25%}.decision-table th:last-child{width:19%}.decision-table .technology{font-size:16px}.decision-table .state-badge{display:inline-block;font-weight:700;padding:3px 9px;border:1px solid currentColor;border-radius:6px;max-width:100%;white-space:normal;background:var(--control-bg)}.state-adopt{color:var(--techniques)}.state-trial{color:var(--platforms)}.state-hold{color:var(--tools)}.state-retire{color:var(--languages)}.state-pending{color:var(--warning)}.decision-table p{margin:8px 0;font-size:13px;font-weight:400}.decision-table details{margin-top:10px;font-weight:400;font-size:13px}.decision-table summary{cursor:pointer;color:var(--link)}.decision-table ul{padding-left:18px}.decision-tables .table-wrap:focus-visible,.decision-table summary:focus-visible{outline:3px solid var(--focus);outline-offset:3px}@media print{.decision-table{min-width:0;table-layout:auto;font-size:10px}.decision-table .state-badge{white-space:normal}.decision-table .technology{font-size:12px}}
`

const decisionTableHTML = `
<section class="decision-tables" aria-labelledby="decisions-title"><h2 id="decisions-title">Decisions</h2><p class="muted">State and approval are recorded separately. Expand a technology for its rationale, members and {{if .Concise}}evidence summary{{else}}source evidence{{end}}. Scroll tables sideways on narrow screens.</p>
{{range .Groups}}<div class="table-wrap" role="region" tabindex="0" aria-label="{{.Quadrant}} decision table"><table class="decision-table"><caption>{{.Quadrant}}</caption><thead><tr><th scope="col">Technology / family</th><th scope="col">State</th><th scope="col">Scope</th><th scope="col">Owner</th><th scope="col">Decision reference</th><th scope="col">Reviewed</th><th scope="col">Changes / review</th></tr></thead><tbody>
{{range .Findings}}<tr id="entry-{{.Number}}"><th scope="row"><span class="technology">{{.Number}}. {{.Name}}</span>{{if .FamilyID}}<p>Family: {{.FamilyID}}{{if and .GroupComponent (ne .GroupComponent ".")}}<br>Component: {{.GroupComponent}}{{end}}</p>{{end}}<details><summary>Rationale, members and evidence</summary><p><code>{{.ID}}</code></p>{{with decision .}}{{if .Rationale}}<p><strong>Rationale:</strong> {{.Rationale}}</p>{{end}}{{end}}{{if and (ne $.Report.Model "lifecycle") .Decision.Lifecycle}}<p>Lifecycle: {{.Decision.Lifecycle}}</p>{{end}}{{with .Assessment}}<p>{{significance .Significance}} · confidence: {{.Confidence}}<br>{{reason .Reason}}</p>{{end}}{{range members .}}<p><strong>{{.Name}}</strong> <code>{{.ID}}</code> · {{.EvidenceCount}} references{{if eq .Presence "not detected"}} · not detected; evidence is from the previous report{{end}}</p>{{if not $.Concise}}<ul>{{range .Evidence}}<li><code>{{.Path}}:{{.Line}}</code><br>{{.Kind}}: {{.Detail}}</li>{{else}}<li>No repository evidence available.</li>{{end}}</ul>{{if gt .EvidenceCount (len .Evidence)}}<p>Showing {{len .Evidence}} of {{.EvidenceCount}} unique evidence references.</p>{{end}}{{end}}{{end}}{{if $.Concise}}<p><a href="{{$.EvidenceFile}}">Full evidence and decisions</a></p>{{end}}</details></th><td><span class="state-badge {{stateClass .}}">{{ring .}}</span></td><td>{{with decision .}}{{if .Scope}}{{.Scope}}{{else}}Not supplied{{end}}{{end}}</td><td>{{if .Decision.Owner}}{{.Decision.Owner}}{{else}}Not supplied{{end}}</td><td>{{if .Decision.Reference}}{{.Decision.Reference}}{{else}}Not supplied{{end}}</td><td>{{if .Decision.Reviewed}}{{.Decision.Reviewed}}{{else}}Not supplied{{end}}</td><td>{{status .}}<p class="muted">{{placementNote .}}</p></td></tr>{{end}}
</tbody></table></div>{{else}}<p>No included technologies.</p>{{end}}</section>
`
