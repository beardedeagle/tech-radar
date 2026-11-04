package main

import (
	"strings"
	"testing"
)

func TestCheckCommandPrereleaseConstraintsAndStrictPins(t *testing.T) {
	for _, tt := range []struct {
		name, constraint, pin string
		exit                  int
	}{
		{"prerelease in AND range", ">=19.0.0-0 <20.0.0", "19.1.0-beta.1", 0},
		{"prerelease below stable upper bound", ">=19.0.0-0 <20.0.0", "20.0.0-beta.1", 0},
		{"stable outside AND range", ">=19.0.0-0 <20.0.0", "20.0.0", 2},
		{"prerelease in second OR branch", ">=18.0.0 <19.0.0 || >=20.0.0-0 <21.0.0", "20.1.0-beta.1", 0},
		{"stable in first OR branch", ">=18.0.0 <19.0.0 || >=20.0.0-0 <21.0.0", "18.1.0", 0},
		{"OR prerelease permission stays in its branch", ">=18.0.0 <19.0.0 || >=20.0.0-0 <21.0.0", "18.1.0-beta.1", 2},
		{"stable range excludes prerelease", ">=19.0.0 <20.0.0", "19.1.0-beta.1", 2},
		{"malformed numeric component", ">=19.0.0-0 <20.0.0", "019.1.0", 3},
		{"malformed prerelease component", ">=19.0.0-0 <20.0.0", "19.1.0-01", 3},
		{"malformed build metadata", ">=19.0.0-0 <20.0.0", "19.1.0+", 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, work := t.TempDir(), t.TempDir()
			write(t, repo, "package.json", `{"dependencies":{"react":"`+tt.pin+`"}}`)
			constrained := strings.Replace(commandPolicy, `"effect":"allow"`, `"effect":"allow","versions":"`+tt.constraint+`"`, 1)
			policy := write(t, work, "policy.json", constrained)
			call(t, tt.exit, "check", "--repo", repo, "--policy", policy, "--as-of", "2026-11-01")
		})
	}
}
