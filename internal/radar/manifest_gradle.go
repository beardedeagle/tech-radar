package radar

import (
	"regexp"
	"strings"
)

var gradleDeclaration = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)\s*(.*)$`)

// gradleManifest reads direct literal coordinates in dependencies blocks.
// Version catalogs, plugins, includes and arbitrary build logic aren't evaluated.
func (s *scanner) gradleManifest(f inputFile) []string {
	s.add("jvm", f, 1, "declaration", "Gradle project manifest; runtime role unverified")
	var warnings, stack []string
	consume := func(text string, line int) {
		text = strings.TrimSpace(text)
		if text == "" || len(stack) == 0 || stack[len(stack)-1] != "dependencies" {
			return
		}
		warn := func() {
			warnings = append(warnings, portfolioCoverage(f, line, "nonliteral or unsupported Gradle dependency"))
		}
		m := gradleDeclaration.FindStringSubmatch(text)
		if m == nil {
			warn()
			return
		}
		scope := "runtime"
		switch m[1] {
		case "implementation", "api", "runtimeOnly", "compile", "runtime":
		case "testImplementation", "testCompile", "testRuntimeOnly", "androidTestImplementation", "testApi":
			scope = "development"
		case "compileOnly", "annotationProcessor", "kapt", "ksp":
			scope = "restricted"
		default:
			warn()
			return
		}
		if len(stack) > 1 {
			scope = "restricted"
		}
		expr := strings.TrimSpace(m[2])
		parens := strings.HasPrefix(expr, "(")
		if parens {
			expr = strings.TrimSpace(expr[1:])
		}
		value, rest, ok := portfolioQuoted(expr)
		if !ok || (parens && rest != ")") || (!parens && rest != "") || strings.ContainsAny(value, "$\\") {
			warn()
			return
		}
		parts := strings.Split(value, ":")
		if len(parts) < 2 || len(parts) > 3 || !manifestPackagePart.MatchString(parts[0]) || !manifestPackagePart.MatchString(parts[1]) {
			warn()
			return
		}
		version := "*"
		if len(parts) == 3 {
			version = parts[2]
		}
		s.dependency(f, "maven", parts[0]+":"+parts[1], version, "declaration", line, scope)
	}
	// Split statements only outside strings/comments. Triple-quoted build strings
	// are skipped so embedded examples cannot become dependency declarations.
	data := string(f.data)
	var text strings.Builder
	line, start := 1, 1
	var quote byte
	blockComment := false
	triple := ""
	flush := func() { consume(text.String(), start); text.Reset(); start = line }
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c == '\n' {
			line++
		}
		if triple != "" {
			if strings.HasPrefix(data[i:], triple) {
				i += 2
				triple = ""
			}
			continue
		}
		if blockComment {
			if c == '*' && i+1 < len(data) && data[i+1] == '/' {
				blockComment = false
				i++
				text.WriteByte(' ')
			}
			continue
		}
		if quote != 0 {
			text.WriteByte(c)
			if c == '\\' && i+1 < len(data) {
				i++
				text.WriteByte(data[i])
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if strings.HasPrefix(data[i:], `"""`) || strings.HasPrefix(data[i:], "'''") {
			triple = data[i : i+3]
			i += 2
			text.WriteString(" [unsupported string] ")
			continue
		}
		if c == '/' && i+1 < len(data) {
			if data[i+1] == '*' {
				blockComment = true
				i++
				continue
			}
			if data[i+1] == '/' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					line++
					flush()
				}
				continue
			}
		}
		if c == '\'' || c == '"' {
			quote = c
			text.WriteByte(c)
			continue
		}
		switch c {
		case '{':
			label := strings.TrimSpace(text.String())
			text.Reset()
			if len(stack) >= 64 {
				warnings = append(warnings, portfolioCoverage(f, line, "Gradle nesting limit"))
				return warnings
			}
			stack = append(stack, label)
			start = line
		case '}':
			flush()
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			start = line
		case '\n', ';':
			flush()
		default:
			if text.Len() == 0 && c != ' ' && c != '\t' {
				start = line
			}
			text.WriteByte(c)
		}
	}
	flush()
	if len(stack) > 0 || quote != 0 || triple != "" || blockComment {
		warnings = append(warnings, portfolioCoverage(f, line, "unterminated Gradle syntax"))
	}
	return warnings
}
