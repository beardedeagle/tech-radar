package radar

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/boyter/scc/v4/processor"
)

var counterOnce sync.Once
var counterMu sync.Mutex

func initCounter() {
	counterOnce.Do(func() {
		processor.ProcessConstants()
		// SCC gets extension candidates from a map. Sort ties for repeatable detection.
		for _, candidates := range processor.ExtensionToLanguage {
			sort.Strings(candidates)
		}
	})
}

func counterSupports(name string) bool {
	initCounter()
	possible, _ := processor.DetectLanguage(path.Base(name))
	return len(possible) > 0 && possible[0] != processor.SheBang
}

type lineClassifier struct{ types []processor.LineType }

func (c *lineClassifier) ProcessLine(_ *processor.FileJob, _ int64, kind processor.LineType) bool {
	c.types = append(c.types, kind)
	return true
}

type analysis struct {
	stat      LanguageStat
	codeLines []processor.LineType
}

func analyze(f inputFile) analysis {
	initCounter()
	counterMu.Lock()
	defer counterMu.Unlock()
	possible, ext := processor.DetectLanguage(path.Base(f.name))
	if len(possible) == 0 || possible[0] == processor.SheBang {
		return analysis{}
	}
	possible = append([]string{}, possible...)
	sort.Strings(possible)
	language := processor.DetermineLanguage(path.Base(f.name), "", possible, f.data)
	if language == "" || language == processor.SheBang {
		return analysis{}
	}
	classifier := &lineClassifier{}
	job := &processor.FileJob{Filename: path.Base(f.name), Location: f.name, Language: language, Extension: ext, PossibleLanguages: possible, Content: f.data, Bytes: int64(len(f.data)), Callback: classifier}
	processor.CountStats(job)
	if job.Binary {
		return analysis{}
	}
	return analysis{LanguageStat{language, 1, job.Lines, job.Code, job.Comment, job.Blank}, classifier.types}
}

func (a analysis) isCodeLine(i int) bool {
	return len(a.codeLines) == 0 || i >= len(a.codeLines) || a.codeLines[i] == processor.LINE_CODE
}

var languageSlug = regexp.MustCompile(`[^a-z0-9]+`)

func languageID(language string) string {
	if id := map[string]string{"Elixir": "elixir", "Erlang": "erlang-otp", "JavaScript": "javascript", "TypeScript": "typescript", "Go": "go", "Python": "python", "Shell": "shell", "BASH": "shell", "Bourne Shell": "shell", "Ruby": "ruby", "C++": "language:cpp", "C#": "language:csharp"}[language]; id != "" {
		return id
	}
	switch language {
	case "JSON", "YAML", "TOML", "Markdown", "XML", "SVG", "Plain Text", "Text", "Dockerfile", "Makefile", "Gitignore", "gitignore", "License", "CSV", "INI", "Properties File":
		return ""
	}
	return "language:" + strings.Trim(languageSlug.ReplaceAllString(strings.ToLower(language), "-"), "-")
}
