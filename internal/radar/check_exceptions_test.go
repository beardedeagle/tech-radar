package radar

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCheckExceptionsRequireApprovalExpiryAndExactPolicy(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"deny-react","technologies":["react"],"effect":"deny"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	data := exceptionData{Kind: "tech-radar-exceptions", SchemaVersion: 1, PolicyFingerprint: policy.fingerprint, Exceptions: []CheckException{{CheckRule: CheckRule{ID: "migration", Technologies: []string{"react"}, Effect: "allow", Repositories: []string{"local:example"}, Versions: ">=19.0.0 <20.0.0"}, Approval: CheckApproval{Status: "approved", Owner: "Engineering", Reference: "EX-1", Reviewed: "2026-11-01"}, Expires: "2026-11-02"}}}
	encoded, _ := json.Marshal(data)
	exceptions, err := ReadCheckExceptions(encoded, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		date string
		exit int
	}{{"2026-11-01", 0}, {"2026-11-02", 0}, {"2026-11-03", 2}} {
		result, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: tt.date, Exceptions: exceptions})
		if err != nil || result.ExitCode() != tt.exit {
			t.Fatalf("expiry %s: %v %+v", tt.date, err, result)
		}
	}
	data.PolicyFingerprint = "bad"
	encoded, _ = json.Marshal(data)
	if _, err := ReadCheckExceptions(encoded, policy); err == nil {
		t.Fatal("unbound exception accepted")
	}
}

func TestCheckOutputRoundTripsAndRejectsEdits(t *testing.T) {
	policy, snapshot := checkFixture(t, `[{"id":"react","technologies":["react"],"effect":"allow"}]`, `{"dependencies":{"react":"19.0.0"}}`)
	r, err := Check([]RepositorySnapshot{snapshot}, policy, CheckOptions{AsOf: "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadCheckResult(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := decoded.Bytes()
	if !bytes.Equal(data, again) {
		t.Fatal("check replay changed")
	}
	edited := bytes.Replace(data, []byte(`"complete": true`), []byte(`"complete": false`), 1)
	if _, err := ReadCheckResult(edited); err == nil {
		t.Fatal("edited check accepted")
	}
}
