package radar

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

// manifestElement is a bounded XML tree. No DTDs or external entities are read.
type manifestElement struct {
	name     string
	text     strings.Builder
	attrs    map[string]string
	children []*manifestElement
	line     int
}

func readManifestXML(data []byte) (*manifestElement, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []*manifestElement
	var root *manifestElement
	count := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid XML manifest")
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, fmt.Errorf("XML directives are unsupported")
		case xml.StartElement:
			count++
			if len(stack) >= 64 || count > 65536 {
				return nil, fmt.Errorf("XML manifest exceeds depth or element limit")
			}
			line, _ := decoder.InputPos()
			n := &manifestElement{name: t.Name.Local, attrs: map[string]string{}, line: line}
			for _, a := range t.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple XML roots")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, fmt.Errorf("content outside XML root")
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("missing XML root")
	}
	return root, nil
}

func (n *manifestElement) value(name string) string {
	for _, child := range n.children {
		if child.name == name {
			return strings.TrimSpace(child.text.String())
		}
	}
	return ""
}

var manifestPackagePart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func (s *scanner) xmlManifest(f inputFile) ([]string, error) {
	root, err := readManifestXML(f.data)
	if err != nil {
		return nil, err
	}
	var warnings []string
	warn := func(n *manifestElement, reason string) {
		warnings = append(warnings, portfolioCoverage(f, n.line, reason))
	}
	if path.Base(f.name) == "pom.xml" {
		if root.name != "project" {
			return nil, fmt.Errorf("expected Maven project root")
		}
		s.add("jvm", f, root.line, "declaration", "Maven project manifest; runtime role unverified")
		var visit func(*manifestElement, string, bool)
		visit = func(n *manifestElement, parent string, conditional bool) {
			// Management, plugins and extensions don't establish application dependencies.
			if !contains([]string{"project", "profiles", "profile", "dependencies", "dependency"}, n.name) {
				return
			}
			conditional = conditional || n.name == "profile"
			if n.name == "dependency" && parent == "dependencies" {
				group, artifact := n.value("groupId"), n.value("artifactId")
				if !manifestPackagePart.MatchString(group) || !manifestPackagePart.MatchString(artifact) {
					warn(n, "nonliteral Maven dependency identity")
					return
				}
				scope := "runtime"
				switch n.value("scope") {
				case "", "compile", "runtime":
				case "test":
					scope = "development"
				case "provided", "system", "import":
					scope = "restricted"
				default:
					scope = "restricted"
					warn(n, "unresolved or unsupported Maven dependency scope")
				}
				optional := n.value("optional")
				if optional != "" && optional != "true" && optional != "false" {
					scope = "restricted"
					warn(n, "unresolved Maven optional dependency")
				}
				if conditional || optional == "true" {
					scope = "restricted"
				}
				version := n.value("version")
				if version == "" {
					version = "*"
				}
				if strings.Contains(version, "${") {
					warn(n, "Maven version property")
					version = "[source omitted]"
				}
				s.dependency(f, "maven", group+":"+artifact, version, "declaration", n.line, scope)
				return
			}
			for _, child := range n.children {
				visit(child, n.name, conditional)
			}
		}
		visit(root, "", false)
	} else {
		if root.name != "Project" {
			return nil, fmt.Errorf("expected MSBuild Project root")
		}
		s.add("dotnet", f, root.line, "declaration", ".NET project manifest; runtime role unverified")
		if root.attrs["Sdk"] == "Microsoft.NET.Sdk.Web" {
			s.add("dotnet:microsoft.aspnetcore.app", f, root.line, "declaration", "ASP.NET Core shared framework [runtime]")
		}
		var visit func(*manifestElement, bool)
		visit = func(n *manifestElement, conditional bool) {
			if !contains([]string{"Project", "ItemGroup", "Choose", "When", "Otherwise", "Target", "PackageReference", "FrameworkReference"}, n.name) {
				return
			}
			conditional = conditional || n.attrs["Condition"] != "" || n.name == "When" || n.name == "Otherwise" || n.name == "Target"
			if n.name == "PackageReference" || n.name == "FrameworkReference" {
				name := n.attrs["Include"]
				if !manifestPackagePart.MatchString(name) {
					warn(n, "nonliteral or update-only .NET dependency")
					return
				}
				scope := "runtime"
				if conditional {
					scope = "restricted"
				}
				version := n.attrs["Version"]
				if version == "" {
					version = n.value("Version")
				}
				if version == "" {
					version = "*"
				}
				if strings.Contains(version, "$(") {
					warn(n, "MSBuild version property")
					version = "[source omitted]"
				}
				ecosystem := "nuget"
				if n.name == "FrameworkReference" {
					ecosystem = "dotnet"
				}
				s.dependency(f, ecosystem, strings.ToLower(name), version, "declaration", n.line, scope)
				return
			}
			for _, child := range n.children {
				visit(child, conditional)
			}
		}
		visit(root, false)
	}
	return warnings, nil
}
