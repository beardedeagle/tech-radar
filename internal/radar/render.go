package radar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
	"tech-radar/internal/jsonutil"
)

const snapshotStart = "\n<!-- tech-radar:snapshot\n"
const snapshotEnd = "\ntech-radar:snapshot -->\n"

type snapshot struct {
	BodySHA256 string `json:"body_sha256"`
	Report     Report `json:"report"`
}

func Render(report Report, format string) ([]byte, error) {
	if err := validateReport(report); err != nil {
		return nil, err
	}
	if format == "json" {
		b, err := json.MarshalIndent(report, "", "  ")
		return append(b, '\n'), err
	}
	var body []byte
	var err error
	switch format {
	case "markdown":
		body = renderMarkdown(report)
	case "html":
		body, err = renderHTML(report)
	default:
		return nil, fmt.Errorf("unknown output format %q", format)
	}
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	state, err := json.MarshalIndent(snapshot{hex.EncodeToString(digest[:]), report}, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, []byte(snapshotStart)...)
	body = append(body, state...)
	body = append(body, []byte(snapshotEnd)...)
	return body, nil
}

func ReadReport(data []byte) (Report, error) {
	var report Report
	if index := bytes.Index(data, []byte(snapshotStart)); index >= 0 {
		remaining := data[index+len(snapshotStart):]
		end := bytes.Index(remaining, []byte(snapshotEnd))
		if end < 0 || len(bytes.TrimSpace(remaining[end+len(snapshotEnd):])) > 0 {
			return report, fmt.Errorf("incomplete snapshot or additional content after snapshot")
		}
		var state snapshot
		if err := jsonutil.DecodeStrict(remaining[:end], &state); err != nil {
			return report, fmt.Errorf("snapshot: %w", err)
		}
		digest := sha256.Sum256(data[:index])
		if hex.EncodeToString(digest[:]) != state.BodySHA256 {
			return report, fmt.Errorf("generated document body was edited; preserve those edits and express decisions in policy before regenerating")
		}
		report = state.Report
	} else if err := jsonutil.DecodeStrict(data, &report); err != nil {
		return report, fmt.Errorf("not a tech-radar JSON report or self-contained export: %w", err)
	}
	return report, validateReport(report)
}

func validateReport(r Report) error {
	if (r.SchemaVersion != 1 && r.SchemaVersion != 2) || r.GeneratorVersion == "" {
		return fmt.Errorf("unsupported or missing report schema/version")
	}
	if len(r.Source.Fingerprint) != 64 {
		return fmt.Errorf("invalid source fingerprint")
	}
	if _, err := hex.DecodeString(r.Source.Fingerprint); err != nil {
		return fmt.Errorf("invalid source fingerprint")
	}
	if r.Source.Files < 0 {
		return fmt.Errorf("invalid source file count")
	}
	if r.Source.ContextFingerprint != "" && (!validDigest(r.Source.ContextFingerprint) || !supportedPortfolioVersion(r.Source.CorpusVersion)) {
		return fmt.Errorf("invalid portfolio context provenance")
	}
	if r.Source.RulesFingerprint != "" {
		if b, err := hex.DecodeString(r.Source.RulesFingerprint); err != nil || len(b) != 32 {
			return fmt.Errorf("invalid run rules fingerprint")
		}
	}
	if (r.Source.GroupingFingerprint != "" && !validDigest(r.Source.GroupingFingerprint)) || (r.Source.GroupingCatalog != "" && !validDigest(r.Source.GroupingCatalog)) {
		return fmt.Errorf("invalid grouping provenance")
	}
	if r.EvidenceFile != "" || r.EvidenceSHA256 != "" {
		if !fs.ValidPath(r.EvidenceFile) || path.Base(r.EvidenceFile) != r.EvidenceFile || path.Ext(r.EvidenceFile) != ".json" {
			return fmt.Errorf("invalid evidence companion filename")
		}
		if b, err := hex.DecodeString(r.EvidenceSHA256); err != nil || len(b) != 32 {
			return fmt.Errorf("invalid evidence companion digest")
		}
	}
	if err := validateModel(r.Model); err != nil {
		return err
	}
	if r.Baseline && r.Model != LifecycleModel {
		return fmt.Errorf("baseline marker requires lifecycle model")
	}
	if err := validateSavedDecisions(r); err != nil {
		return err
	}
	ids := map[string]bool{}
	raw := map[string]Finding{}
	numbers := map[int]bool{}
	for _, f := range r.Findings {
		if !validID.MatchString(f.ID) || ids[f.ID] {
			return fmt.Errorf("invalid or duplicate finding id %q", f.ID)
		}
		ids[f.ID] = true
		raw[f.ID] = f
		if f.Number < 1 || numbers[f.Number] {
			return fmt.Errorf("invalid or duplicate display number %d", f.Number)
		}
		numbers[f.Number] = true
		if f.Quadrant != "" && !contains(Quadrants, f.Quadrant) {
			return fmt.Errorf("%s: invalid finding quadrant", f.ID)
		}
		if f.Presence != "detected" && f.Presence != "not detected" {
			return fmt.Errorf("%s: invalid presence", f.ID)
		}
		if r.Model == LifecycleModel && (f.MissingDecisionMaterial != (!r.Baseline && strings.TrimSpace(f.Decision.Reference) == "") || (f.MissingDecisionMaterial && !f.ReviewRequired)) {
			return fmt.Errorf("%s: inconsistent missing decision material review state", f.ID)
		}
		if f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("%s: finding and policy decisions disagree", f.ID)
		}
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		if f.EvidenceCount < len(f.Evidence) {
			return fmt.Errorf("%s: invalid evidence count", f.ID)
		}
		for _, e := range f.Evidence {
			if !fs.ValidPath(e.Path) || e.Path == "." || e.Line < 1 {
				return fmt.Errorf("%s: invalid relative evidence location", f.ID)
			}
		}
	}
	for _, c := range r.Components {
		if !fs.ValidPath(c.Path) || !knownProfile(c.Profile) {
			return fmt.Errorf("invalid component profile/path")
		}
	}
	for _, f := range r.ExcludedFindings {
		if ids[f.ID] || !validID.MatchString(f.ID) || !contains(r.Excluded, f.ID) || !r.Decisions[f.ID].Exclude || f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("invalid excluded evidence %s", f.ID)
		}
		ids[f.ID] = true
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		for _, e := range f.Evidence {
			if !fs.ValidPath(e.Path) || e.Path == "." || e.Line < 1 {
				return fmt.Errorf("invalid excluded evidence path")
			}
		}
	}
	grouped := map[string]bool{}
	familyIDs := map[string]bool{}
	for _, f := range r.Highlights {
		if f.FamilyID != "" {
			key := familyIdentity(f)
			if familyIDs[key] {
				return fmt.Errorf("duplicate family/component highlight %s", f.FamilyID)
			}
			familyIDs[key] = true
		}
		original, ok := raw[f.ID]
		if !ok || f.Number != original.Number || f.Decision != r.Decisions[f.ID] {
			return fmt.Errorf("highlight does not match source finding %s", f.ID)
		}
		if err := validateInference(f, r.Model); err != nil {
			return err
		}
		if r.Model == LifecycleModel && (f.MissingDecisionMaterial != original.MissingDecisionMaterial || (f.MissingDecisionMaterial && !f.ReviewRequired)) {
			return fmt.Errorf("%s: highlight lost missing decision material review state", f.ID)
		}
		if !contains(f.Members, f.ID) {
			return fmt.Errorf("highlight omits its root %s", f.ID)
		}
		for _, id := range f.Members {
			if _, exists := raw[id]; !exists || grouped[id] {
				return fmt.Errorf("invalid/duplicate grouped member %s", id)
			}
			if id != f.ID && !groupableDecisions(r.Model, f.Decision, r.Decisions[id]) {
				return fmt.Errorf("group hides explicit decision %s", id)
			}
			grouped[id] = true
		}
	}
	return nil
}

func validateInference(f Finding, model Model) error {
	if f.MissingDecisionMaterial && model != LifecycleModel {
		return fmt.Errorf("%s: material gap requires lifecycle model", f.ID)
	}
	if model == LifecycleModel && f.Proposal != nil {
		return fmt.Errorf("%s: lifecycle placement requires a supplied decision", f.ID)
	}
	if (f.FamilyID != "" && (!validID.MatchString(f.FamilyID) || len(f.FamilyID) > 128 || !fs.ValidPath(f.GroupComponent))) || (f.FamilyID == "" && f.GroupComponent != "") {
		return fmt.Errorf("%s: invalid family identity or component", f.ID)
	}
	if c := f.PlacementChange; c != nil {
		if !contains([]string{"new", "unchanged", "moved", "moved-in", "moved-out"}, c.Kind) || (c.PreviousRing != "" && !contains(append(append([]string{}, model.rings()...), model.pending()), c.PreviousRing)) || (c.PreviousQuadrant != "" && !contains(Quadrants, c.PreviousQuadrant)) {
			return fmt.Errorf("%s: invalid placement comparison", f.ID)
		}
		if (c.Kind == "new" && (c.PreviousRing != "" || c.PreviousQuadrant != "")) || (c.Kind != "new" && c.PreviousRing == "") {
			return fmt.Errorf("%s: incomplete placement comparison", f.ID)
		}
	}
	if f.Assessment != nil {
		if !contains([]string{"unknown", "system_defining", "workflow_enabling", "implementation_support"}, f.Assessment.Significance) || !contains([]string{"unknown", "documentation", "reference", "declaration", "configuration", "static wiring"}, f.Assessment.Confidence) {
			return fmt.Errorf("%s: invalid assessment", f.ID)
		}
	}
	if f.Proposal != nil {
		if f.Proposal.Status != "proposed" || f.Proposal.Exclude || f.Proposal.Owner != "" || f.Proposal.Reference != "" || f.Proposal.Reviewed != "" {
			return fmt.Errorf("%s: inference cannot claim approval", f.ID)
		}
		if err := validateRules(map[string]Decision{f.ID: *f.Proposal}); err != nil {
			return err
		}
	}
	return nil
}

func ring(f Finding) string {
	if f.Decision.Ring == "" {
		if f.Proposal != nil {
			return f.Proposal.Ring
		}
		return "Needs review"
	}
	return f.Decision.Ring
}
func quadrant(f Finding) string {
	if f.Quadrant == "" {
		return "Unclassified"
	}
	return f.Quadrant
}
func status(f Finding) string {
	label := f.Decision.Status
	if label == "" {
		label = "unreviewed"
		if f.Proposal != nil {
			label = "suggested"
		}
	}
	if f.MissingDecisionMaterial {
		label += " - missing decision material requires review"
	}
	if f.Presence != "detected" {
		return label + "; not detected - review required"
	}
	if f.EvidenceChanged {
		return label + "; evidence changed - review required"
	}
	if f.ReviewRequired && !f.MissingDecisionMaterial {
		return label + "; review required"
	}
	return label
}
func md(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "\\|", "`", "&#96;", "\r", " ", "\n", " ").Replace(s)
}

const coverage = "Reads manifests, direct dependency locks, tool pins, language counts and selected source, configuration and documentation references. Unknown dependencies stay unclassified. Dynamic code and ambiguous references need review. Runtime behavior, deployed versions and vendor accounts aren't checked."
const boundaries = "Skips dependency/build/generated directories, tests, credentials, symlinks, binary files and files over 2 MiB. Warnings list size and binary skips. Counts cover scanned files. Repo scripts, builds and hooks aren't run. Git HEAD is read when available."

func renderMarkdown(r Report) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n# %s Technology Radar\n\n", GeneratedMarker, md(r.Repository))
	fmt.Fprintf(&b, "Generated by tech-radar %s. %d entries across %d scanned files.\n\n", Version, len(r.Findings), r.Source.Files)
	if r.Model == LifecycleModel {
		b.WriteString("Lifecycle placements come from supplied decisions. **Needs decision** lists candidates without a lifecycle decision.\n\n")
	} else {
		b.WriteString("Review suggested placements with your team. Saved decisions take priority. **Needs review** lists findings without a placement. Lifecycle is recorded separately.\n\n")
	}
	b.WriteString(modelReviewNote(r) + "\n\n")
	fmt.Fprintf(&b, "Source fingerprint: `%s`\n\n", r.Source.Fingerprint)
	if r.Source.GitHead != "" {
		fmt.Fprintf(&b, "Git HEAD: `%s`. The fingerprint identifies the files read.\n\n", md(r.Source.GitHead))
	}
	if r.Source.Counter != "" {
		fmt.Fprintf(&b, "Language counter: %s.\n\n", md(r.Source.Counter))
	}
	fmt.Fprintf(&b, "## Radar\n\n| Quadrant | %s | %s |\n|---|---:|---:|---:|---:|---:|\n", strings.Join(r.Model.rings(), " | "), r.Model.pending())
	for _, q := range append(append([]string{}, Quadrants...), "Unclassified") {
		counts := map[string]int{}
		for _, f := range r.Findings {
			if quadrant(f) == q {
				counts[r.Model.ring(f)]++
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d |\n", md(q), counts[r.Model.rings()[0]], counts[r.Model.rings()[1]], counts[r.Model.rings()[2]], counts[r.Model.rings()[3]], counts[r.Model.pending()])
	}
	b.WriteString("\n" + modelRingExplanation(r.Model) + "\n\n## Register\n\n")
	for _, f := range r.Findings {
		d := displayDecision(f)
		fmt.Fprintf(&b, "### %d. %s\n\n- **ID:** `%s`\n- **Quadrant:** %s\n- **Ring:** %s\n- **Decision:** %s\n", f.Number, md(f.Name), md(f.ID), md(quadrant(f)), md(r.Model.ring(f)), md(status(f)))
		for _, v := range []struct{ label, value string }{{"Scope", d.Scope}, {"Rationale", d.Rationale}, {"Owner", f.Decision.Owner}, {"Decision reference", f.Decision.Reference}, {"Reviewed", f.Decision.Reviewed}, {"Lifecycle", f.Decision.Lifecycle}} {
			if v.value != "" {
				fmt.Fprintf(&b, "- **%s:** %s\n", v.label, md(v.value))
			}
		}
		if f.Presence != "detected" {
			b.WriteString("\nNot found in this scan. The evidence below is from the previous report. Check whether the technology was removed or is still relevant.\n")
		}
		if len(f.Evidence) == 0 {
			b.WriteString("\nAdded by policy; no repository evidence was found.\n")
		} else {
			b.WriteString("\n| Evidence location | Kind | Finding |\n|---|---|---|\n")
			for _, e := range f.Evidence {
				fmt.Fprintf(&b, "| `%s:%d` | %s | %s |\n", md(e.Path), e.Line, md(e.Kind), md(e.Detail))
			}
			if f.EvidenceCount > len(f.Evidence) {
				fmt.Fprintf(&b, "\nShowing %d of %d unique evidence references.\n", len(f.Evidence), f.EvidenceCount)
			}
		}
		b.WriteString("\n")
	}
	if len(r.Languages) > 0 {
		b.WriteString("## Language statistics\n\n| Language | Files | Code | Comments | Blank | Total lines |\n|---|---:|---:|---:|---:|---:|\n")
		for _, s := range r.Languages {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d |\n", md(s.Language), s.Files, s.Code, s.Comments, s.Blank, s.Lines)
		}
		b.WriteString("\nCounts cover the scanned files. Languages excluded from the radar still appear here.\n\n")
	}
	if len(r.Excluded) > 0 {
		b.WriteString("## Explicit policy exclusions\n\n")
		for _, id := range r.Excluded {
			fmt.Fprintf(&b, "- `%s`: %s\n", md(id), md(r.Decisions[id].Rationale))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Coverage and maintenance\n\n" + coverage + "\n\n" + boundaries + "\n\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "- **Scan warning:** %s\n", md(w))
	}
	b.WriteString("\nEdit policy and regenerate with --previous. Direct edits to this report stop reimport so they aren't discarded. Changed evidence keeps the saved decision and requests review; update the decision reference or review date after reviewing it.\n\n")
	if r.Model == LifecycleModel {
		b.WriteString(lifecycleMethod + "\n")
	} else {
		b.WriteString("Method: [Thoughtworks Technology Radar](https://www.thoughtworks.com/en-us/radar), Volume 34. Team recommendations are independent of Thoughtworks.\n")
	}
	return []byte(b.String())
}

type chartPoint struct {
	X, Y                float64
	Number              int
	Anchor, Name, Color string
	Change, Tooltip     string
	SymbolColor         string
}
type htmlView struct {
	Report                              Report
	Points                              []chartPoint
	Pending, Overflow                   int
	Coverage, Boundaries                string
	Concise                             bool
	EvidenceFile                        string
	RawCount, Unknown, Changed, Missing int
}

func renderHTML(r Report) ([]byte, error) {
	return renderHTMLView(r, false, "")
}

func renderHTMLView(r Report, concise bool, evidenceFile string) ([]byte, error) {
	v := htmlView{Report: r, Coverage: coverage, Boundaries: boundaries, Concise: concise, EvidenceFile: evidenceFile, RawCount: len(r.Findings)}
	for _, f := range r.Findings {
		if f.Assessment != nil && f.Assessment.Significance == "unknown" {
			v.Unknown++
		}
		if f.EvidenceChanged {
			v.Changed++
		}
		if f.Presence == "not detected" {
			v.Missing++
		}
	}
	if concise {
		r.Findings = r.Highlights
		v.Report = r
	}
	colors := []string{"#00635e", "#2853a5", "#965117", "#7d4190"}
	symbolColors := []string{"var(--techniques)", "var(--platforms)", "var(--tools)", "var(--languages)"}
	signs := [][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}}
	bands := [][2]float64{{0, 215}, {215, 295}, {295, 365}, {365, 430}}
	for _, f := range r.Findings {
		if f.Quadrant == "" || r.Model.ring(f) == r.Model.pending() {
			v.Pending++
		}
	}
	for qi, q := range Quadrants {
		for ri, ringName := range r.Model.rings() {
			var group []Finding
			for _, f := range r.Findings {
				if f.Quadrant == q && r.Model.ring(f) == ringName {
					group = append(group, f)
				}
			}
			var slots [][2]float64
			// Concentric rows use narrow rings more effectively than a square
			// grid. Chord/radial spacing keeps 30px markers at least 38px apart.
			for radius := bands[ri][0] + 20; radius <= bands[ri][1]-20; radius += 38 {
				if radius < math.Hypot(38, 38) {
					continue
				}
				start := math.Asin(38 / radius)
				span := math.Pi/2 - 2*start
				steps := int(math.Floor(span / (2 * math.Asin(19/radius))))
				for i := 0; i <= steps; i++ {
					angle := math.Pi / 4
					if steps > 0 {
						angle = start + float64(i)*span/float64(steps)
					}
					slots = append(slots, [2]float64{radius * math.Cos(angle), radius * math.Sin(angle)})
				}
			}
			sort.Slice(slots, func(i, j int) bool {
				a, b := slots[i], slots[j]
				angleA, angleB := math.Atan2(a[1], a[0]), math.Atan2(b[1], b[0])
				if angleA != angleB {
					return angleA < angleB
				}
				return math.Hypot(a[0], a[1]) < math.Hypot(b[0], b[1])
			})
			for i, f := range group {
				if i >= len(slots) {
					v.Overflow++
					continue
				}
				slot := slots[i]
				change := ""
				if f.PlacementChange != nil {
					change = f.PlacementChange.Kind
				}
				v.Points = append(v.Points, chartPoint{
					X: 500 + signs[qi][0]*slot[0], Y: 500 + signs[qi][1]*slot[1],
					Number: f.Number, Anchor: fmt.Sprintf("entry-%d", f.Number),
					Name: f.Name, Color: colors[qi], SymbolColor: symbolColors[qi],
					Change: change, Tooltip: chartTooltip(r.Model, f),
				})
			}
		}
	}
	t, err := template.New("radar").Funcs(template.FuncMap{"ring": r.Model.ring, "rings": Model.rings, "modelReview": modelReviewNote, "ringExplanation": modelRingExplanation, "quadrant": quadrant, "status": status, "decision": displayDecision, "reason": displayReason, "significance": significanceLabel, "profile": profileName, "role": displayRole}).Parse(htmlDocument)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, v)
	return b.Bytes(), err
}

const htmlDocument = `<!-- tech-radar:generated -->
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Report.Repository}} Technology Radar</title>` + themeScript + chartScript + `<style>` + themeStyles + chartStyles + `
*{box-sizing:border-box}body{margin:0;color:var(--text);font:16px/1.6 system-ui,sans-serif;background:var(--bg)}main{max-width:1160px;margin:auto;padding:24px 28px 40px}h1{font-size:clamp(30px,5vw,46px);line-height:1.15;letter-spacing:-1px}h2{margin-top:38px}h3{margin:0 0 14px;font-size:23px}.meta,.muted{color:var(--muted);font-size:14px}.notice{border-left:4px solid var(--accent);padding:14px 18px;background:var(--notice-bg)}a{color:var(--link);text-underline-offset:3px}a:focus-visible{outline:3px solid var(--focus);outline-offset:3px}.chart-scroll{overflow:auto}.chart{width:100%;min-width:650px;max-height:820px}.chart text{font-family:inherit}.chart .label{font-size:21px;font-weight:650}.chart .ring{font-size:15px;fill:var(--text)}.chart .point{font-size:15px;fill:white;font-weight:700;text-anchor:middle}.chart a:hover circle{stroke:var(--text);stroke-width:3}.register{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:28px}.entry{border-top:3px solid var(--border);padding-top:18px;scroll-margin-top:20px;overflow-wrap:anywhere}.entry p,.entry li{font-size:14px}.entry ul{padding-left:20px}.entry li{margin-bottom:8px}.decision{font-weight:650}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;font-size:14px}th,td{text-align:left;padding:9px;border-bottom:1px solid var(--border)}code{font-size:12px;overflow-wrap:anywhere}footer{border-top:1px solid var(--border);margin-top:38px;padding-top:20px;font-size:13px}.warning{color:var(--warning)}@media(max-width:700px){main{padding:22px 18px}.register{grid-template-columns:1fr}}@media print{main{max-width:none;padding:0}.chart{min-width:0;height:170mm}.entry{break-inside:avoid}.register{display:block}.entry{margin-bottom:24px}}
</style></head><body><main>
<div class="theme-control" id="theme-control" hidden><label for="theme">Theme</label><select id="theme"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></div>
<h1>{{.Report.Repository}} Technology Radar</h1>
<p class="meta">tech-radar {{.Report.GeneratorVersion}} · {{len .Report.Findings}} entries · {{.Report.Source.Files}} eligible files · {{.Report.Source.Counter}}</p>
<p class="notice">{{if eq .Report.Model "lifecycle"}}Lifecycle placements come from supplied decisions. Unplaced candidates appear as Needs decision.{{else}}Review suggested placements with your team. Saved decisions take priority. {{.Pending}} entries have no placement and are listed under “Needs review”.{{end}}</p>
<p class="meta">{{modelReview .Report}}</p>
{{if .Concise}}<p><a href="{{.EvidenceFile}}">Full evidence and decisions</a> · {{.RawCount}} findings · {{.Unknown}} need investigation · {{.Changed}} changed · {{.Missing}} no longer detected.</p><h2>Architecture</h2><ul>{{range .Report.Components}}<li>{{if ne .Path "."}}<strong>{{.Path}}</strong>: {{end}}{{profile .Profile}} ({{role .Role}}). {{reason .Reason}}</li>{{else}}<li>No application profile matched. Detected tools, integrations and team decisions are still included.</li>{{end}}</ul>{{end}}
<section aria-label="Technology radar"><div class="chart-scroll"><svg class="chart" viewBox="0 0 1000 1000" role="group" aria-labelledby="chart-title chart-description"><title id="chart-title">Technology radar by quadrant and ring</title><desc id="chart-description">Numbered links lead to register entries. Unplaced findings are listed in the register. Position within a ring has no ranking.</desc>
<circle cx="500" cy="500" r="430" fill="var(--radar-outer)" stroke="var(--radar-border)"/><circle cx="500" cy="500" r="365" fill="var(--radar-assess)" stroke="var(--radar-border)"/><circle cx="500" cy="500" r="295" fill="var(--radar-trial)" stroke="var(--radar-border)"/><circle cx="500" cy="500" r="215" fill="var(--radar-adopt)" stroke="var(--radar-border)"/><path d="M500 65V935M65 500H935" stroke="var(--bg)" stroke-width="36"/>
<text x="55" y="36" class="label" fill="var(--techniques)">Techniques</text><text x="690" y="36" class="label" fill="var(--platforms)">Platforms</text><text x="55" y="972" class="label" fill="var(--tools)">Tools</text><text x="590" y="972" class="label" fill="var(--languages)">Languages &amp; Frameworks</text>
<g class="ring" text-anchor="middle"><text x="100" y="506">{{index (rings .Report.Model) 3}}</text><text x="170" y="506">{{index (rings .Report.Model) 2}}</text><text x="245" y="506">{{index (rings .Report.Model) 1}}</text><text x="385" y="506">Adopt</text><text x="615" y="506">Adopt</text><text x="755" y="506">{{index (rings .Report.Model) 1}}</text><text x="830" y="506">{{index (rings .Report.Model) 2}}</text><text x="900" y="506">{{index (rings .Report.Model) 3}}</text></g>
{{range .Points}}<a href="#{{.Anchor}}" class="blip" data-change="{{.Change}}" data-tooltip="{{.Tooltip}}" aria-label="{{.Number}}. {{.Name}}"><circle cx="{{.X}}" cy="{{.Y}}" r="15" fill="{{.Color}}" stroke="var(--point-outline)" stroke-width="2"/><title>{{.Tooltip}}</title><text x="{{.X}}" y="{{.Y}}" dy="5" class="point">{{.Number}}</text>{{if eq .Change "new"}}<circle cx="{{.X}}" cy="{{.Y}}" r="18" fill="none" stroke="{{.SymbolColor}}" stroke-width="2"/>{{else if or (eq .Change "moved") (eq .Change "moved-in") (eq .Change "moved-out")}}<path transform="translate({{.X}} {{.Y}})" d="M-18 0 A18 18 0 0 1 18 0" fill="none" stroke="{{.SymbolColor}}" stroke-width="2" stroke-linecap="round"/>{{end}}</a>{{end}}
</svg></div>
<div class="radar-legend" aria-label="Change symbols"><span><svg viewBox="-21 -21 42 42" aria-hidden="true"><circle r="12" fill="currentColor"/><circle r="18" fill="none" stroke="currentColor" stroke-width="2"/></svg>New entry</span><span><svg viewBox="-21 -21 42 42" aria-hidden="true"><circle r="12" fill="currentColor"/><path d="M-18 0 A18 18 0 0 1 18 0" fill="none" stroke="currentColor" stroke-width="2"/></svg>Moved in/out or changed quadrant</span><span><svg viewBox="-21 -21 42 42" aria-hidden="true"><circle r="12" fill="currentColor"/></svg>{{if eq .Report.Model "lifecycle"}}Unchanged or unavailable comparison{{else}}No change in placement{{end}}</span></div>
<p class="muted">The first report marks every entry as new to this radar. Later versions compare placements within the same model. New does not mean recently adopted. Hover or focus a marker for details.</p>
<p class="muted">Select a numbered point to inspect its evidence. The register contains all entries. Scroll sideways on narrow screens.{{if .Overflow}} {{.Overflow}} placed entries don't fit on the chart; see the register below.{{end}}</p></section><div id="radar-tooltip" role="tooltip" hidden></div>
<p>{{ringExplanation .Report.Model}}</p>
<h2>{{if .Concise}}Main findings{{else}}Register{{end}}</h2><div class="register">{{range .Report.Findings}}<section class="entry" id="entry-{{.Number}}"><h3>{{.Number}}. {{.Name}}</h3><p class="decision">{{quadrant .}} · {{ring .}}<br>{{status .}}</p><p><code>{{.ID}}</code>{{if and .GroupComponent (ne .GroupComponent ".")}} · {{.GroupComponent}}{{end}}</p>{{with decision .}}{{if .Scope}}<p><strong>Scope:</strong> {{.Scope}}</p>{{end}}{{if .Rationale}}<p><strong>Rationale:</strong> {{.Rationale}}</p>{{end}}{{end}}{{if .Decision.Owner}}<p><strong>Owner:</strong> {{.Decision.Owner}}</p>{{end}}{{if .Decision.Reference}}<p><strong>Decision:</strong> {{.Decision.Reference}} · {{.Decision.Reviewed}}</p>{{end}}{{if .Decision.Lifecycle}}<p><strong>Lifecycle:</strong> {{.Decision.Lifecycle}}</p>{{end}}{{if eq .Presence "not detected"}}<p class="warning">Not detected in the current scan. Any listed evidence is from the prior snapshot.</p>{{end}}{{if $.Concise}}{{with .Assessment}}<p><strong>{{significance .Significance}}</strong> · confidence: {{.Confidence}}<br>{{reason .Reason}}</p>{{end}}{{if gt (len .Members) 1}}<p>Grouped members: {{range .Members}}<code>{{.}}</code> {{end}}</p>{{end}}{{else}}<ul>{{range .Evidence}}<li><code>{{.Path}}:{{.Line}}</code><br>{{.Kind}}: {{.Detail}}</li>{{else}}<li>Added by policy; no repository evidence was found.</li>{{end}}</ul>{{end}}</section>{{end}}</div>
<h2>Language statistics</h2><p class="muted">Counts cover the scanned files. Languages excluded from the radar still appear here.</p><div class="table-wrap"><table><thead><tr><th>Language</th><th>Files</th><th>Code</th><th>Comments</th><th>Blank</th><th>Lines</th></tr></thead><tbody>{{range .Report.Languages}}<tr><td>{{.Language}}</td><td>{{.Files}}</td><td>{{.Code}}</td><td>{{.Comments}}</td><td>{{.Blank}}</td><td>{{.Lines}}</td></tr>{{end}}</tbody></table></div>
<h2>Policy exclusions</h2><ul>{{range .Report.Excluded}}<li><code>{{.}}</code></li>{{else}}<li>No detected findings excluded by policy.</li>{{end}}</ul>
<h2>Evidence and coverage</h2><p>{{.Coverage}}</p><p>{{.Boundaries}}</p>{{range .Report.Warnings}}<p class="warning">{{.}}</p>{{end}}<p class="meta">On-disk source fingerprint: <code>{{.Report.Source.Fingerprint}}</code><br>Git HEAD at scan: <code>{{.Report.Source.GitHead}}</code></p>
<footer>Edit policy and regenerate with --previous. {{if .Concise}}Commit the linked evidence file to keep the full history.{{else}}This file includes the full saved report.{{end}} Direct edits to the document stop reimport so they aren't discarded.<br>{{if eq .Report.Model "lifecycle"}}Lifecycle method adapted from Andrew Harmel-Law's <a href="https://martinfowler.com/articles/scaling-architecture-conversationally.html">Scaling the Practice of Architecture, Conversationally</a>.{{else}}Method: <a href="https://www.thoughtworks.com/en-us/radar">Thoughtworks Technology Radar</a>, Volume 34.{{end}}</footer>
</main></body></html>
`
