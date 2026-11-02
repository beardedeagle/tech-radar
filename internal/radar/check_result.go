package radar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"tech-radar/internal/jsonutil"
)

// CheckIssue identifies an observed violation or undecidable policy check.
type CheckIssue struct {
	Technology string     `json:"technology,omitempty"`
	RuleID     string     `json:"rule_id,omitempty"`
	Code       string     `json:"code"`
	Message    string     `json:"message"`
	Evidence   []Evidence `json:"evidence,omitempty"`
}

// CheckDrift records observation changes independently of policy compliance.
type CheckDrift struct {
	Technology string   `json:"technology"`
	Kind       string   `json:"kind"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

// CheckedRepository keeps the frozen observations and their check results.
type CheckedRepository struct {
	Snapshot   RepositorySnapshot `json:"snapshot"`
	Violations []CheckIssue       `json:"violations"`
	Gaps       []CheckIssue       `json:"coverage_gaps"`
	Drift      []CheckDrift       `json:"drift"`
	Exceptions []string           `json:"applied_exceptions,omitempty"`
}

// CheckResult retains the observations used for checking and later drift runs.
// Its digest detects accidental edits; it is not a signature or approval proof.
type CheckResult struct {
	Kind                  string              `json:"kind"`
	SchemaVersion         int                 `json:"schema_version"`
	GeneratorVersion      string              `json:"generator_version"`
	PolicyID              string              `json:"policy_id"`
	PolicyVersion         string              `json:"policy_version"`
	PolicyFingerprint     string              `json:"policy_fingerprint"`
	ExceptionsFingerprint string              `json:"exceptions_fingerprint,omitempty"`
	AsOf                  string              `json:"as_of"`
	Complete              bool                `json:"complete"`
	Repositories          []CheckedRepository `json:"repositories"`
	RemovedRepositories   []string            `json:"repositories_not_in_current_scan,omitempty"`
	SHA256                string              `json:"sha256"`
}

// ExitCode returns 2 for observed violations, 3 for coverage gaps only, or 0.
// The CLI reserves 1 for failed input, collection and output operations.
func (r CheckResult) ExitCode() int {
	for _, repo := range r.Repositories {
		if len(repo.Violations) > 0 {
			return 2
		}
	}
	for _, repo := range r.Repositories {
		if len(repo.Gaps) > 0 {
			return 3
		}
	}
	return 0
}

// Bytes serializes a check result with an edit-detection digest and size limit.
func (r CheckResult) Bytes() ([]byte, error) {
	r.SHA256 = ""
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	r.SHA256 = hex.EncodeToString(sum[:])
	data, err = json.MarshalIndent(r, "", "  ")
	if len(data) > MaxContextBytes {
		return nil, fmt.Errorf("check result exceeds 128 MiB")
	}
	return append(data, '\n'), err
}

// ReadCheckResult validates generated check output before baseline use or forced
// replacement. The digest detects edits but does not establish authenticity.
func ReadCheckResult(data []byte) (CheckResult, error) {
	var r CheckResult
	if len(data) > MaxContextBytes {
		return r, fmt.Errorf("check result exceeds 128 MiB")
	}
	if err := jsonutil.DecodeStrict(data, &r); err != nil {
		return r, err
	}
	if r.Kind != "tech-radar-check" || r.SchemaVersion != 1 || r.GeneratorVersion == "" || !validDigest(r.PolicyFingerprint) || len(r.Repositories) == 0 {
		return r, fmt.Errorf("invalid check result")
	}
	digest := r.SHA256
	r.SHA256 = ""
	canonical, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	sum := sha256.Sum256(canonical)
	r.SHA256 = digest
	if hex.EncodeToString(sum[:]) != digest {
		return r, fmt.Errorf("check result was edited or is incomplete")
	}
	snapshots := make([]RepositorySnapshot, 0, len(r.Repositories))
	for _, repo := range r.Repositories {
		snapshots = append(snapshots, repo.Snapshot)
	}
	if _, err := BuildContext(snapshots); err != nil {
		return r, fmt.Errorf("invalid saved check observations: %w", err)
	}
	return r, nil
}
