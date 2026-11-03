package radar

import (
	"bytes"
	"testing"
)

func TestContextPreservesSaved021Generator(t *testing.T) {
	snapshot := capturedFixture(t, "web", "package.json", `{"dependencies":{"react":"19.1.0"}}`)
	saved, err := buildContext([]RepositorySnapshot{snapshot}, PortfolioVersion, "0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := saved.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ReadContext(data)
	if err != nil {
		t.Fatalf("cannot read saved 0.2.1 context: %v", err)
	}
	replayed, err := restored.Bytes()
	if err != nil || !bytes.Equal(data, replayed) || restored.pack.GeneratorVersion != "0.2.1" {
		t.Fatal("replay changed the saved generator version or fingerprint")
	}
	edited := bytes.Replace(data, []byte(`"generator_version": "0.2.1"`), []byte(`"generator_version": "`+Version+`"`), 1)
	if _, err := ReadContext(edited); err == nil {
		t.Fatal("accepted edited saved generator metadata")
	}
}
