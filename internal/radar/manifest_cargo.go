package radar

import "strings"

// cargoManifest reads registry declarations, including renamed crates and
// dependency subtables. Workspace inheritance and source references aren't resolved.
func (s *scanner) cargoManifest(f inputFile) []string {
	var warnings []string
	section, subname, subscope := "", "", ""
	fields := map[string]string{}
	subline := 1
	emit := func(name string, attrs map[string]string, version string, line int, scope string) {
		warn := func(reason string) { warnings = append(warnings, portfolioCoverage(f, line, reason)) }
		if attrs["workspace"] != "" {
			warn("Cargo workspace dependency inheritance")
			return
		}
		if v, exists := attrs["package"]; exists {
			literal, rest, ok := portfolioQuoted(v)
			if !ok || rest != "" {
				warn("nonliteral Cargo package rename")
				return
			}
			name = literal
		}
		if !manifestPackagePart.MatchString(name) {
			warn("unsupported Cargo package name")
			return
		}
		if attrs["optional"] == "true" {
			scope = "restricted"
		}
		if attrs["git"] != "" || attrs["path"] != "" || attrs["registry"] != "" {
			warn("Cargo dependency source reference")
			version = "[source omitted]"
			scope = "restricted"
		}
		if version == "" {
			version = "*"
			if v, exists := attrs["version"]; exists {
				literal, rest, ok := portfolioQuoted(v)
				if !ok || rest != "" {
					warn("nonliteral Cargo version")
					return
				}
				version = literal
			}
		}
		s.dependency(f, "cargo", name, version, "declaration", line, scope)
	}
	flush := func() {
		if subname != "" {
			emit(subname, fields, "", subline, subscope)
		}
		subname = ""
		fields = map[string]string{}
	}
	multiline := ""
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
		if strings.HasPrefix(line, "[") {
			flush()
			section = line
			if section == "[package]" {
				s.add("language:rust", f, i+1, "declaration", "Cargo package manifest; runtime role unverified")
			}
			for _, kind := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
				prefix := "[" + kind + "."
				if strings.HasPrefix(section, prefix) && strings.HasSuffix(section, "]") {
					subname = strings.TrimSuffix(strings.TrimPrefix(section, prefix), "]")
					subscope = "runtime"
					if kind != "dependencies" {
						subscope = "development"
					}
					subline = i + 1
				}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
			if !strings.Contains(value[3:], value[:3]) {
				multiline = value[:3]
			}
			continue
		}
		if subname != "" {
			fields[key] = value
			continue
		}
		scope := "runtime"
		switch section {
		case "[dependencies]":
		case "[dev-dependencies]", "[build-dependencies]":
			scope = "development"
		default:
			if strings.HasPrefix(section, "[target.") && strings.HasSuffix(section, ".dependencies]") {
				scope = "restricted"
			} else {
				continue
			}
		}
		if name, rest, literal := portfolioQuoted(key); literal && rest == "" {
			key = name
		}
		if v, rest, literal := portfolioQuoted(value); literal && rest == "" {
			emit(key, nil, v, i+1, scope)
			continue
		}
		attrs, valid := cargoInlineTable(value)
		if !valid {
			warnings = append(warnings, portfolioCoverage(f, i+1, "unsupported Cargo dependency literal"))
			continue
		}
		emit(key, attrs, "", i+1, scope)
	}
	flush()
	return warnings
}

func cargoInlineTable(text string) (map[string]string, bool) {
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return nil, false
	}
	attrs := map[string]string{}
	text = text[1 : len(text)-1]
	start, depth := 0, 0
	var quote byte
	for i := 0; i <= len(text); i++ {
		if i < len(text) {
			c := text[i]
			if quote != 0 {
				if c == '\\' {
					i++
					continue
				}
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
				continue
			}
			if c == '[' {
				depth++
				continue
			}
			if c == ']' {
				depth--
				if depth < 0 {
					return nil, false
				}
				continue
			}
			if c != ',' || depth > 0 {
				continue
			}
		}
		field := strings.TrimSpace(text[start:i])
		start = i + 1
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" || value == "" || attrs[key] != "" {
			return nil, false
		}
		attrs[key] = value
	}
	return attrs, quote == 0 && depth == 0
}
