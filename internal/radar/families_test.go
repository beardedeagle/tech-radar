package radar

import (
	"reflect"
	"strings"
	"testing"
)

func TestRelatedTechnologiesShareOneRadarEntryAndKeepTheirEvidence(t *testing.T) {
	root := beamFixture(t)
	fixture(t, root, "mix.exs", `defp deps do
[
{:phoenix, "1"},
{:ash, "3"},
{:ash_authentication, "4"},
{:ash_ai, "1"},
{:ex_aws_s3, "2"},
{:ex_aws_kms, "2"},
{:ex_aws, "2"},
{:ecto_sql, "3"}
]
end
`)
	fixture(t, root, ".tool-versions", "elixir 1.19\nerlang 28\n")
	fixture(t, root, "lib/app/application.ex", "use Application\nSupervisor.start_link(children, strategy: :one_for_one)\n")
	fixture(t, root, "lib/app/storage.ex", "ExAws.S3.get_object(bucket, key)\n")
	fixture(t, root, "config/runtime.exs", "ExAws.KMS.encrypt(key, data)\n")
	fixture(t, root, "CLAUDE.md", "Use Ash-first domain modeling.\n")
	inv := scanTestRepository(t, root, ScanOptions{})
	r := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
	want := map[string][]string{
		"Amazon Web Services":   {"amazon-s3", "aws-kms", "hex:ex_aws"},
		"Ash Framework":         {"ash", "ash-ai", "ash-authentication", "ash-first"},
		"Elixir / Erlang / OTP": {"elixir", "erlang-otp", "otp-supervision"},
	}
	seen := map[string]bool{}
	for _, h := range r.Highlights {
		if members, ok := want[h.Name]; ok {
			if !reflect.DeepEqual(h.Members, members) {
				t.Errorf("%s members: got %v, want %v", h.Name, h.Members, members)
			}
			if ring(h) != "Adopt" || h.Decision != (Decision{}) {
				t.Errorf("%s should suggest established use without claiming a team decision", h.Name)
			}
			seen[h.Name] = true
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("missing family %s", name)
		}
	}
	for _, members := range want {
		for _, id := range members {
			appearances := 0
			for _, h := range r.Highlights {
				if contains(h.Members, id) {
					appearances++
				}
			}
			if appearances != 1 {
				t.Errorf("%s appears in %d radar entries", id, appearances)
			}
			if len(finding(t, Inventory{Findings: r.Findings}, id).Evidence) == 0 {
				t.Errorf("grouping lost %s evidence", id)
			}
		}
	}
	if _, err := Render(r, "json"); err != nil {
		t.Fatal(err)
	}

	decision := Decision{Ring: "Caution", Scope: "Encryption", Rationale: "Review key ownership", Status: "approved", Owner: "Team", Reference: "PR #3", Reviewed: "2026-11-01"}
	policy := Policy{Rules: map[string]Decision{"aws-kms": decision, "ash-ai": {Exclude: true, Rationale: "Outside this radar's scope"}}}
	decided := buildTestReport(t, inv, policy, &r, "", BuildOptions{})
	data, err := RenderDocument(decided, "markdown", "evidence.json", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ReadReport(data)
	if err != nil {
		t.Fatal(err)
	}
	next := buildTestReport(t, inv, Policy{}, &saved, "", BuildOptions{})
	if next.Decisions["aws-kms"] != decision || !next.Decisions["ash-ai"].Exclude {
		t.Fatal("family replay lost a decision or exclusion")
	}
	separate := false
	for _, h := range next.Highlights {
		if contains(h.Members, "ash-ai") {
			t.Fatal("excluded technology returned as a family member")
		}
		if contains(h.Members, "aws-kms") {
			if h.ID != "aws-kms" || len(h.Members) != 1 || ring(h) != "Caution" || h.Decision != decision {
				t.Fatal("family hid or extended the encryption decision")
			}
			separate = true
		}
	}
	if !separate {
		t.Fatal("family hid the encryption decision")
	}
}

func TestSuggestedAdoptRequiresUseEvidence(t *testing.T) {
	declaration := Evidence{"mix.exs", 1, "declaration", "hex package req: 1"}
	use := Evidence{"lib/app/client.ex", 1, "source reference", "Req HTTP/test reference"}
	for _, tt := range []struct {
		name     string
		evidence []Evidence
		want     string
	}{
		{"declared and used", []Evidence{declaration, use}, "Adopt"},
		{"declared only", []Evidence{declaration}, "Assess"},
		{"configured only", []Evidence{{"config/runtime.exs", 1, "configuration", "Req HTTP/test reference"}}, "Trial"},
		{"documented only", []Evidence{{"README.md", 1, "documentation", "Req HTTP/test reference"}}, "Assess"},
		{"transitive and referenced", []Evidence{{"mix.lock", 1, "lock resolution", "hex package req: 1"}, use}, "Assess"},
		{"restricted dependency", []Evidence{{"mix.exs", 1, "declaration", "hex package req: 1 [restricted Mix environment]"}, use}, "Assess"},
		{"helper only", []Evidence{declaration, {"scripts/demo.exs", 1, "source reference", "Req HTTP/test reference"}}, "Assess"},
		{"nested helper only", []Evidence{declaration, {"apps/client/scripts/demo.exs", 1, "source reference", "Req HTTP/test reference"}}, "Assess"},
		{"manifest classified as source", []Evidence{declaration, {"mix.exs", 1, "source reference", "SCC language classification: Elixir"}}, "Assess"},
		{"tool configuration classified as source", []Evidence{declaration, {".credo.exs", 1, "source reference", "SCC language classification: Elixir"}}, "Assess"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inv := Inventory{Source: Source{Fingerprint: strings.Repeat("a", 64)}, Findings: []Finding{{ID: "req", Name: "Req", Quadrant: "Tools", Presence: "detected", Evidence: tt.evidence, EvidenceCount: len(tt.evidence)}}}
			r := buildTestReport(t, inv, Policy{}, nil, "Demo", BuildOptions{})
			f := r.Findings[0]
			if ring(f) != tt.want {
				t.Fatalf("got %s, want %s", ring(f), tt.want)
			}
			if f.Decision != (Decision{}) || len(r.Decisions) != 0 || f.Proposal.Status != "proposed" {
				t.Fatal("suggested placement became a team decision")
			}
		})
	}
}
