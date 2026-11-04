package radar

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"sort"
)

type chartPoint struct {
	X, Y                float64
	Number              int
	Anchor, Name, Color string
	Change, Tooltip     string
	SymbolColor         string
}
type htmlView struct {
	Report                              Report
	Table                               bool
	Groups                              []tableGroup
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
	v := htmlView{Report: r, Table: r.selectedView() == TableView, Coverage: coverage, Boundaries: boundaries, Concise: concise, EvidenceFile: evidenceFile, RawCount: len(r.Findings)}
	var byID map[string]Finding
	if v.Table {
		v.Groups = decisionGroups(r)
		byID = findingsByID(r)
	}
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
	if concise || v.Table {
		r.Findings = r.Highlights
		v.Report = r
	}
	if !v.Table {
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
	}
	t, err := template.New("radar").Funcs(template.FuncMap{
		"ring":            r.Model.ring,
		"stateClass":      func(f Finding) string { return stateClass(r.Model, f) },
		"members":         func(f Finding) []Finding { return tableMembers(byID, f) },
		"placementNote":   placementNote,
		"rings":           Model.rings,
		"modelReview":     modelReviewNote,
		"ringExplanation": modelRingExplanation,
		"quadrant":        quadrant,
		"status":          r.Model.status,
		"decision":        r.Model.displayDecision,
		"historical":      r.Model.historicalDecision,
		"rationaleLabel":  rationaleLabel,
		"reason":          displayReason,
		"significance":    significanceLabel,
		"profile":         profileName,
		"role":            displayRole,
	}).Parse(htmlDocument)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, v)
	return b.Bytes(), err
}

const htmlDocument = `<!-- tech-radar:generated -->
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Report.Repository}} Technology Radar</title>` + themeScript + `{{if not .Table}}` + chartScript + `{{end}}<style>` + themeStyles + chartStyles + decisionTableStyles + `
* {
  box-sizing:border-box
}
body {
  margin:0;
  color:var(--text);
  font:16px/1.6 system-ui,sans-serif;
  background:var(--bg)
}
main {
  max-width:1160px;
  margin:auto;
  padding:24px 28px 40px
}
h1 {
  font-size:clamp(30px,5vw,46px);
  line-height:1.15;
  letter-spacing:-1px
}
h2 {
  margin-top:38px
}
h3 {
  margin:0 0 14px;
  font-size:23px
}
.meta,.muted {
  color:var(--muted);
  font-size:14px
}
.notice {
  border-left:4px solid var(--accent);
  padding:14px 18px;
  background:var(--notice-bg)
}
a {
  color:var(--link);
  text-underline-offset:3px
}
a:focus-visible {
  outline:3px solid var(--focus);
  outline-offset:3px
}
.chart-scroll {
  overflow:auto
}
.chart {
  width:100%;
  min-width:650px;
  max-height:820px
}
.chart text {
  font-family:inherit
}
.chart .label {
  font-size:21px;
  font-weight:650
}
.chart .ring {
  font-size:15px;
  fill:var(--text)
}
.chart .point {
  font-size:15px;
  fill:white;
  font-weight:700;
  text-anchor:middle
}
.chart a:hover circle {
  stroke:var(--text);
  stroke-width:3
}
.register {
  display:grid;
  grid-template-columns:repeat(2,minmax(0,1fr));
  gap:28px
}
.entry {
  border-top:3px solid var(--border);
  padding-top:18px;
  scroll-margin-top:20px;
  overflow-wrap:anywhere
}
.entry p,.entry li {
  font-size:14px
}
.entry ul {
  padding-left:20px
}
.entry li {
  margin-bottom:8px
}
.decision {
  font-weight:650
}
.table-wrap {
  overflow:auto
}
table {
  width:100%;
  border-collapse:collapse;
  font-size:14px
}
th,td {
  text-align:left;
  padding:9px;
  border-bottom:1px solid var(--border)
}
code {
  font-size:12px;
  overflow-wrap:anywhere
}
footer {
  border-top:1px solid var(--border);
  margin-top:38px;
  padding-top:20px;
  font-size:13px
}
.warning {
  color:var(--warning)
}
@media(max-width:700px) {
  main {
    padding:22px 18px
  }
  .register {
    grid-template-columns:1fr
  }
}
@media print {
  main {
    max-width:none;
    padding:0
  }
  .chart {
    min-width:0;
    height:170mm
  }
  .entry {
    break-inside:avoid
  }
  .register {
    display:block
  }
  .entry {
    margin-bottom:24px
  }
}
</style>
</head>
<body>
<main>
  <div class="theme-control" id="theme-control" hidden><label for="theme">Theme</label><select id="theme"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select>
  </div>
  <h1>{{.Report.Repository}} Technology Radar</h1>
  <p class="meta">tech-radar {{.Report.GeneratorVersion}} · {{len .Report.Findings}} entries · {{.Report.Source.Files}} eligible files · {{.Report.Source.Counter}}</p>
  <p class="notice">
  {{if eq .Report.Model "lifecycle"}}Lifecycle placements come from supplied decisions. Unplaced candidates appear as Needs decision.
  {{else}}Review suggested placements with your team. Saved decisions take priority.
    {{if .Table}}Unplaced candidates appear as Needs review.
    {{else}}{{.Pending}} entries have no placement and are listed under “Needs review”.
    {{end}}
  {{end}}</p>
  <p class="meta">{{modelReview .Report}}</p>
  {{if .Concise}}
    <p><a href="{{.EvidenceFile}}">Full evidence and decisions</a> · {{.RawCount}} findings · {{.Unknown}} need investigation · {{.Changed}} changed · {{.Missing}} no longer detected.</p>
    <h2>Architecture</h2>
    <ul>
      {{range .Report.Components}}<li>
        {{if ne .Path "."}}<strong>{{.Path}}</strong>:
        {{end}}{{profile .Profile}} ({{role .Role}}). {{reason .Reason}}</li>
      {{else}}<li>No application profile matched. Detected tools, integrations and team decisions are still included.</li>
      {{end}}
    </ul>
  {{end}}
  {{if .Table}}` + decisionTableHTML + `
  {{else}}
    <section aria-label="Technology radar">
      <div class="chart-scroll"><svg class="chart" viewBox="0 0 1000 1000" role="group" aria-labelledby="chart-title chart-description"><title id="chart-title">Technology radar by quadrant and ring</title><desc id="chart-description">Numbered links lead to register entries. Unplaced findings are listed in the register. Position within a ring has no ranking.</desc>
        <circle cx="500" cy="500" r="430" fill="var(--radar-outer)" stroke="var(--radar-border)"/>
        <circle cx="500" cy="500" r="365" fill="var(--radar-assess)" stroke="var(--radar-border)"/>
        <circle cx="500" cy="500" r="295" fill="var(--radar-trial)" stroke="var(--radar-border)"/>
        <circle cx="500" cy="500" r="215" fill="var(--radar-adopt)" stroke="var(--radar-border)"/>
        <path d="M500 65V935M65 500H935" stroke="var(--bg)" stroke-width="36"/>
        <text x="55" y="36" class="label" fill="var(--techniques)">Techniques</text>
        <text x="690" y="36" class="label" fill="var(--platforms)">Platforms</text>
        <text x="55" y="972" class="label" fill="var(--tools)">Tools</text>
        <text x="590" y="972" class="label" fill="var(--languages)">Languages &amp; Frameworks</text>
        <g class="ring" text-anchor="middle">
        <text x="100" y="506">{{index (rings .Report.Model) 3}}</text>
        <text x="170" y="506">{{index (rings .Report.Model) 2}}</text>
        <text x="245" y="506">{{index (rings .Report.Model) 1}}</text>
        <text x="385" y="506">Adopt</text>
        <text x="615" y="506">Adopt</text>
        <text x="755" y="506">{{index (rings .Report.Model) 1}}</text>
        <text x="830" y="506">{{index (rings .Report.Model) 2}}</text>
        <text x="900" y="506">{{index (rings .Report.Model) 3}}</text>
</g>
        {{range .Points}}<a href="#{{.Anchor}}" class="blip" data-change="{{.Change}}" data-tooltip="{{.Tooltip}}" aria-label="{{.Number}}. {{.Name}}"><circle cx="{{.X}}" cy="{{.Y}}" r="15" fill="{{.Color}}" stroke="var(--point-outline)" stroke-width="2"/><title>{{.Tooltip}}</title>
        <text x="{{.X}}" y="{{.Y}}" dy="5" class="point">{{.Number}}</text>
          {{if eq .Change "new"}}<circle cx="{{.X}}" cy="{{.Y}}" r="18" fill="none" stroke="{{.SymbolColor}}" stroke-width="2"/>
          {{else if or (eq .Change "moved") (eq .Change "moved-in") (eq .Change "moved-out")}}<path transform="translate({{.X}} {{.Y}})" d="M-18 0 A18 18 0 0 1 18 0" fill="none" stroke="{{.SymbolColor}}" stroke-width="2" stroke-linecap="round"/>
          {{end}}</a>
        {{end}}
        </svg>
      </div>
      <div class="radar-legend" aria-label="Change symbols">
      <span><svg viewBox="-21 -21 42 42" aria-hidden="true">
      <circle r="12" fill="currentColor"/>
      <circle r="18" fill="none" stroke="currentColor" stroke-width="2"/></svg>New entry</span>
      <span><svg viewBox="-21 -21 42 42" aria-hidden="true">
      <circle r="12" fill="currentColor"/>
      <path d="M-18 0 A18 18 0 0 1 18 0" fill="none" stroke="currentColor" stroke-width="2"/></svg>Moved in/out or changed quadrant</span>
      <span><svg viewBox="-21 -21 42 42" aria-hidden="true">
      <circle r="12" fill="currentColor"/></svg>
        {{if eq .Report.Model "lifecycle"}}Unchanged or unavailable comparison
        {{else}}No change in placement
        {{end}}</span>
      </div>
      <p class="muted">The first report marks every entry as new to this radar. Later versions compare placements within the same model. New does not mean recently adopted. Hover or focus a marker for details.</p>
      <p class="muted">Select a numbered point to inspect its evidence. The register contains all entries. Scroll sideways on narrow screens.
      {{if .Overflow}} {{.Overflow}} placed entries don't fit on the chart; see the register below.
      {{end}}</p>
    </section>
    <div id="radar-tooltip" role="tooltip" hidden>
    </div>
    <p>{{ringExplanation .Report.Model}}</p>
    <h2>
    {{if .Concise}}Main findings
    {{else}}Register
    {{end}}</h2>
    <div class="register">
      {{range .Report.Findings}}
        <section class="entry" id="entry-{{.Number}}">
          <h3>{{.Number}}. {{.Name}}</h3>
          <p class="decision">{{quadrant .}} · {{ring .}}<br>{{status .}}</p>
          <p><code>{{.ID}}</code>
          {{if and .GroupComponent (ne .GroupComponent ".")}} · {{.GroupComponent}}
          {{end}}</p>
          {{with decision .}}
            {{if .Scope}}
              <p><strong>Scope:</strong> {{.Scope}}</p>
            {{end}}
            {{if .Rationale}}
              <p><strong>{{rationaleLabel .}}:</strong> {{.Rationale}}</p>
            {{end}}
          {{end}}
          {{with decision .}}
            {{if .Owner}}<p><strong>Owner:</strong> {{.Owner}}</p>{{end}}
            {{if .Reference}}<p><strong>Decision:</strong> {{.Reference}}</p>{{end}}
            {{if .Reviewed}}<p><strong>Reviewed:</strong> {{.Reviewed}}</p>{{end}}
            {{if .Lifecycle}}<p><strong>Lifecycle:</strong> {{.Lifecycle}}</p>{{end}}
          {{end}}
          {{range historical .}}
            {{if .Value}}<p><strong>{{.Label}}:</strong> {{.Value}}</p>{{end}}
          {{end}}
          {{if eq .Presence "not detected"}}
            <p class="warning">Not detected in the current scan. Any listed evidence is from the prior snapshot.</p>
          {{end}}
          {{if $.Concise}}
            {{with .Assessment}}
              <p><strong>{{significance .Significance}}</strong> · confidence: {{.Confidence}}<br>{{reason .Reason}}</p>
            {{end}}
            {{if gt (len .Members) 1}}
              <p>Grouped members:
              {{range .Members}}<code>{{.}}</code>
              {{end}}</p>
            {{end}}
          {{else}}
            <ul>
              {{range .Evidence}}<li><code>{{.Path}}:{{.Line}}</code><br>{{.Kind}}: {{.Detail}}</li>
              {{else}}<li>Added by policy; no repository evidence was found.</li>
              {{end}}
            </ul>
          {{end}}
        </section>
      {{end}}
    </div>
  {{end}}
  <h2>Language statistics</h2>
  <p class="muted">Counts cover the scanned files. Languages excluded from the radar still appear here.</p>
  <div class="table-wrap">
    <table>
      <thead>
        <tr><th>Language</th><th>Files</th><th>Code</th><th>Comments</th><th>Blank</th><th>Lines</th>
        </tr>
      </thead>
      <tbody>
        {{range .Report.Languages}}
          <tr><td>{{.Language}}</td><td>{{.Files}}</td><td>{{.Code}}</td><td>{{.Comments}}</td><td>{{.Blank}}</td><td>{{.Lines}}</td>
          </tr>
        {{end}}
      </tbody>
    </table>
  </div>
  <h2>Policy exclusions</h2>
  <ul>
    {{range .Report.Excluded}}<li><code>{{.}}</code></li>
    {{else}}<li>No detected findings excluded by policy.</li>
    {{end}}
  </ul>
  <h2>Evidence and coverage</h2>
  <p>{{.Coverage}}</p>
  <p>{{.Boundaries}}</p>
  {{range .Report.Warnings}}
    <p class="warning">{{.}}</p>
  {{end}}
  <p class="meta">On-disk source fingerprint: <code>{{.Report.Source.Fingerprint}}</code><br>Git HEAD at scan: <code>{{.Report.Source.GitHead}}</code></p>
  <footer>Edit policy and regenerate with --previous.
    {{if .Concise}}Commit the linked evidence file to keep the full history.
    {{else}}This file includes the full saved report.
    {{end}} Direct edits to the document stop reimport so they aren't discarded.<br>
    {{if eq .Report.Model "lifecycle"}}Lifecycle method adapted from Andrew Harmel-Law's <a href="https://martinfowler.com/articles/scaling-architecture-conversationally.html">Scaling the Practice of Architecture, Conversationally</a>.
    {{else}}Method: <a href="https://www.thoughtworks.com/en-us/radar">Thoughtworks Technology Radar</a>, Volume 34.
    {{end}}
  </footer>
</main>
</body>
</html>
`
