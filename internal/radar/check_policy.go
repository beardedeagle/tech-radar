package radar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"tech-radar/internal/jsonutil"
)

// CheckApproval records the approval supplied by the policy owner, not proof of
// that person's authority. Proposed radar placements cannot supply permission.
type CheckApproval struct {
	Status    string `json:"status"`
	Owner     string `json:"owner"`
	Reference string `json:"reference"`
	Reviewed  string `json:"reviewed"`
}

// CheckRule defines permission for exact observed IDs within optional repo/path
// boundaries. Versions is a SemVer constraint, evaluated only on concrete pins.
type CheckRule struct {
	ID           string   `json:"id"`
	Technologies []string `json:"technologies"`
	Effect       string   `json:"effect"`
	Repositories []string `json:"repositories,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	Versions     string   `json:"versions,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	constraint   *semver.Constraints
}

type checkPolicyData struct {
	Kind          string        `json:"kind"`
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	Version       string        `json:"version"`
	Default       string        `json:"default"`
	Approval      CheckApproval `json:"approval"`
	Rules         []CheckRule   `json:"rules"`
}

// CheckPolicy is an immutable validated enforcement policy. Its zero value is
// invalid. It is separate from the presentation policy used by generate.
type CheckPolicy struct {
	data        checkPolicyData
	fingerprint string
}

// ReadCheckPolicy validates and canonicalizes an explicit approved policy.
// Invalid metadata, unknown fields, duplicate IDs and oversized inputs fail.
func ReadCheckPolicy(data []byte) (CheckPolicy, error) {
	var p checkPolicyData
	if len(data) > MaxFileBytes {
		return CheckPolicy{}, fmt.Errorf("enforcement policy exceeds 2 MiB")
	}
	if err := jsonutil.DecodeStrict(data, &p); err != nil {
		return CheckPolicy{}, fmt.Errorf("enforcement policy: %w", err)
	}
	if p.Kind != "tech-radar-policy" || p.SchemaVersion != 1 || !validID.MatchString(p.ID) || len(p.ID) > 128 || strings.TrimSpace(p.Version) == "" || len(p.Version) > 128 || !contains([]string{"allow", "deny"}, p.Default) || p.Rules == nil || len(p.Rules) > 1024 {
		return CheckPolicy{}, fmt.Errorf("enforcement policy requires kind, schema 1, id, version, explicit default and up to 1024 rules")
	}
	if err := validateCheckApproval(p.Approval); err != nil {
		return CheckPolicy{}, err
	}
	ids := map[string]bool{}
	for i := range p.Rules {
		if err := validateCheckRule(&p.Rules[i], ids); err != nil {
			return CheckPolicy{}, err
		}
	}
	sort.Slice(p.Rules, func(i, j int) bool { return p.Rules[i].ID < p.Rules[j].ID })
	canonical, err := json.Marshal(p)
	if err != nil {
		return CheckPolicy{}, err
	}
	digest := sha256.Sum256(canonical)
	return CheckPolicy{data: p, fingerprint: hex.EncodeToString(digest[:])}, nil
}

func validateCheckApproval(a CheckApproval) error {
	if a.Status != "approved" || strings.TrimSpace(a.Owner) == "" || strings.TrimSpace(a.Reference) == "" || len(a.Owner) > 240 || len(a.Reference) > 1024 {
		return fmt.Errorf("enforcement approval requires approved status, owner, reference and reviewed date")
	}
	if _, err := time.Parse(time.DateOnly, a.Reviewed); err != nil {
		return fmt.Errorf("approval reviewed must be YYYY-MM-DD")
	}
	return nil
}

func validateCheckRule(r *CheckRule, ids map[string]bool) error {
	if !validID.MatchString(r.ID) || len(r.ID) > 128 || ids[r.ID] || !contains([]string{"allow", "deny", "existing-only"}, r.Effect) || len(r.Technologies) == 0 || len(r.Technologies) > 512 || len(r.Repositories) > 1024 || len(r.Paths) > 256 || len(r.Reason) > 1024 {
		return fmt.Errorf("invalid/duplicate enforcement rule %q", r.ID)
	}
	ids[r.ID] = true
	for _, id := range r.Technologies {
		if !validID.MatchString(id) || len(id) > 256 {
			return fmt.Errorf("%s: invalid technology ID", r.ID)
		}
	}
	for _, repo := range r.Repositories {
		if strings.TrimSpace(repo) == "" || len(repo) > 256 || strings.ContainsAny(repo, "\n\r\x00") {
			return fmt.Errorf("%s: invalid repository identity", r.ID)
		}
	}
	for _, p := range r.Paths {
		if !fs.ValidPath(p) || strings.Contains(p, "\\") || len(p) > 4096 {
			return fmt.Errorf("%s: paths must be relative files or directory prefixes", r.ID)
		}
	}
	for _, values := range [][]string{r.Technologies, r.Repositories, r.Paths} {
		sort.Strings(values)
		if len(slices.Compact(slices.Clone(values))) != len(values) {
			return fmt.Errorf("%s: duplicate selector", r.ID)
		}
	}
	if len(r.Versions) > 512 || (r.Effect == "deny" && r.Versions != "") {
		return fmt.Errorf("%s: version constraints belong on allow/existing-only rules and are limited to 512 bytes", r.ID)
	}
	if r.Versions != "" {
		c, err := semver.NewConstraint(r.Versions)
		if err != nil {
			return fmt.Errorf("%s: invalid SemVer constraint", r.ID)
		}
		r.constraint = c
	}
	return nil
}

// CheckException is an explicitly approved, expiring permission override.
// Repositories must be bounded so a local exception cannot silently apply org-wide.
type CheckException struct {
	CheckRule
	Approval CheckApproval `json:"approval"`
	Expires  string        `json:"expires"`
}

type exceptionData struct {
	Kind              string           `json:"kind"`
	SchemaVersion     int              `json:"schema_version"`
	PolicyFingerprint string           `json:"policy_fingerprint"`
	Exceptions        []CheckException `json:"exceptions"`
}

// CheckExceptions holds validated overrides bound to one exact policy digest.
type CheckExceptions struct {
	data        exceptionData
	fingerprint string
}

// ReadCheckExceptions validates policy-bound exceptions without applying expiry.
// The explicit evaluation date in Check controls whether each exception is active.
func ReadCheckExceptions(data []byte, policy CheckPolicy) (CheckExceptions, error) {
	var pack exceptionData
	if len(data) > MaxFileBytes {
		return CheckExceptions{}, fmt.Errorf("exceptions exceed 2 MiB")
	}
	if err := jsonutil.DecodeStrict(data, &pack); err != nil {
		return CheckExceptions{}, fmt.Errorf("exceptions: %w", err)
	}
	if policy.fingerprint == "" || pack.Kind != "tech-radar-exceptions" || pack.SchemaVersion != 1 || pack.PolicyFingerprint != policy.fingerprint || pack.Exceptions == nil || len(pack.Exceptions) > 256 {
		return CheckExceptions{}, fmt.Errorf("exceptions require schema 1 and the current policy_fingerprint")
	}
	ids := map[string]bool{}
	for i := range pack.Exceptions {
		e := &pack.Exceptions[i]
		if err := validateCheckRule(&e.CheckRule, ids); err != nil {
			return CheckExceptions{}, err
		}
		if e.Effect != "allow" || len(e.Repositories) == 0 {
			return CheckExceptions{}, fmt.Errorf("%s: exception must allow use within explicit repository identities", e.ID)
		}
		if err := validateCheckApproval(e.Approval); err != nil {
			return CheckExceptions{}, err
		}
		if _, err := time.Parse(time.DateOnly, e.Expires); err != nil || e.Expires < e.Approval.Reviewed {
			return CheckExceptions{}, fmt.Errorf("%s: invalid exception expiry", e.ID)
		}
	}
	sort.Slice(pack.Exceptions, func(i, j int) bool { return pack.Exceptions[i].ID < pack.Exceptions[j].ID })
	canonical, err := json.Marshal(pack)
	if err != nil {
		return CheckExceptions{}, err
	}
	digest := sha256.Sum256(canonical)
	return CheckExceptions{data: pack, fingerprint: hex.EncodeToString(digest[:])}, nil
}
