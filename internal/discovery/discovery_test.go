package discovery

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tech-radar/internal/radar"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

type archiveEntry struct {
	name string
	data string
	kind byte
	link string
}

func makeArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: 0600, Typeflag: kind, Linkname: e.link}
		if kind == tar.TypeReg {
			h.Size = int64(len(e.data))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jsonResponse(v any) []byte { b, _ := json.Marshal(v); return b }

func fakeFetch(responses map[string][]byte, failures map[string]error, calls *[]string) func(context.Context, string, io.Writer) error {
	return func(_ context.Context, endpoint string, out io.Writer) error {
		if calls != nil {
			*calls = append(*calls, endpoint)
		}
		if err := failures[endpoint]; err != nil {
			return err
		}
		b, ok := responses[endpoint]
		if !ok {
			return fmt.Errorf("unexpected endpoint %s", endpoint)
		}
		_, err := out.Write(b)
		return err
	}
}

func TestCollectPinsCommitAndNormalizesSnapshots(t *testing.T) {
	archive := makeArchive(t,
		archiveEntry{name: "archive-root/go.mod", data: "module example.org/service\n\ngo 1.26\n"},
		archiveEntry{name: "archive-root/main.go", data: "package main\nfunc main() {}\n"},
		archiveEntry{name: "archive-root/.env", data: "PRIVATE_VALUE=never retain"},
		archiveEntry{name: "archive-root/vendor/dependency.go", data: "package dependency"},
	)
	responses := map[string][]byte{}
	for _, name := range []string{"Example/Zeta", "Example/Alpha"} {
		responses["repos/"+name] = jsonResponse(map[string]any{"full_name": name, "default_branch": "release/stable", "archived": true, "fork": true})
		responses["repos/"+name+"/commits/release%2Fstable"] = jsonResponse(map[string]string{"sha": testSHA})
		responses["repos/"+name+"/tarball/"+testSHA] = archive
	}
	var calls []string
	c := collector{fetch: fakeFetch(responses, nil, &calls)}
	got, err := c.collect(context.Background(), "", []string{"Example/Zeta", "Example/Alpha"}, radar.DetectionRules{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "github:example/alpha" || got[1].ID != "github:example/zeta" {
		t.Fatalf("unordered snapshots: %+v", got)
	}
	for _, s := range got {
		if s.Origin != "github" || s.Revision != testSHA || s.Status != "captured" || !s.Archived || !s.Fork || s.Inventory.Source.GitHead != testSHA {
			t.Fatalf("missing pinned metadata: %+v", s)
		}
		var paths []string
		for _, f := range s.Files {
			paths = append(paths, f.Path)
			if len(f.SHA256) != 64 {
				t.Fatalf("invalid file digest: %+v", f)
			}
		}
		if !reflect.DeepEqual(paths, []string{"go.mod", "main.go"}) {
			t.Fatalf("captured paths: %v", paths)
		}
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"never retain", "module example.org/service", os.TempDir()} {
		if bytes.Contains(b, []byte(value)) {
			t.Fatalf("snapshot retained source or local path %q", value)
		}
	}
	other, err := c.collect(context.Background(), "", []string{"Example/Alpha", "Example/Zeta"}, radar.DetectionRules{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherJSON, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, otherJSON) {
		t.Fatal("repository input order or temporary path changed normalized snapshots")
	}
	for i, endpoint := range calls {
		if strings.Contains(endpoint, "/tarball/") && (i == 0 || !strings.Contains(calls[i-1], "/commits/")) {
			t.Fatalf("archive was not preceded by pin: %v", calls)
		}
	}
}

func TestOrganizationMembershipPagination(t *testing.T) {
	first := make([]repository, 100)
	for i := range first {
		first[i] = repository{FullName: fmt.Sprintf("Example/repo-%03d", i), DefaultBranch: "main"}
	}
	last := []repository{{FullName: "Example/final", DefaultBranch: "main", Archived: true, Fork: true}}
	var calls []string
	c := collector{fetch: fakeFetch(map[string][]byte{
		"orgs/Example/repos?type=all&sort=full_name&direction=asc&per_page=100&page=1": jsonResponse(first),
		"orgs/Example/repos?type=all&sort=full_name&direction=asc&per_page=100&page=2": jsonResponse(last),
	}, nil, &calls)}
	got, err := c.repositories(context.Background(), "Example", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 101 || !got[100].Archived || !got[100].Fork || len(calls) != 2 {
		t.Fatalf("incomplete membership: %d repositories, calls %v", len(got), calls)
	}
}

func TestCollectEmptyRequiresExplicitAPIProof(t *testing.T) {
	meta := map[string][]byte{"repos/Example/empty": []byte(`{"full_name":"Example/empty","default_branch":"main","size":0}`)}
	endpoint := "repos/Example/empty/commits/main"
	for _, tc := range []struct {
		name  string
		err   error
		empty bool
	}{
		{"empty", &apiError{status: 409, message: "Git Repository is empty."}, true},
		{"empty lowercase", &apiError{status: 409, message: "git repository is empty"}, true},
		{"empty punctuation", &apiError{status: 409, message: " GIT REPOSITORY IS EMPTY! "}, true},
		{"empty text wrong status", &apiError{status: 404, message: "Git Repository is empty."}, false},
		{"unavailable", &apiError{status: 409, message: "Repository is unavailable."}, false},
		{"inaccessible", &apiError{status: 404, message: "Not Found"}, false},
		{"network", errors.New("network unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := collector{fetch: fakeFetch(meta, map[string]error{endpoint: tc.err}, nil)}
			got, err := c.collect(context.Background(), "", []string{"Example/empty"}, radar.DetectionRules{}, nil)
			if !tc.empty {
				if err == nil || got != nil {
					t.Fatalf("failed request became success: %+v, %v", got, err)
				}
				return
			}
			if err != nil || len(got) != 1 || got[0].Status != "empty" || got[0].Revision != "" || len(got[0].Files) != 0 {
				t.Fatalf("empty capture: %+v, %v", got, err)
			}
		})
	}
}

func TestCollectRejectsInvalidSelectorsBeforeRequests(t *testing.T) {
	for _, tc := range []struct {
		org   string
		repos []string
	}{
		{}, {org: "Example", repos: []string{"Example/service"}}, {org: "../other"},
		{repos: []string{"https://example.net/Example/service"}}, {repos: []string{"Example/../service"}},
		{repos: []string{"Example/service", "example/SERVICE"}},
	} {
		c := collector{fetch: func(context.Context, string, io.Writer) error { t.Fatal("invalid selector requested API"); return nil }}
		if _, err := c.collect(context.Background(), tc.org, tc.repos, radar.DetectionRules{}, nil); err == nil {
			t.Fatalf("accepted invalid selector: %+v", tc)
		}
	}
}

func TestArchiveRejectsUnsafePathsAndDuplicates(t *testing.T) {
	for _, name := range []string{"../outside.go", "/absolute.go", "root/../../outside.go", `root\outside.go`, "root/./file.go", "root/../file.go", "C:/outside.go"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := extractArchive(bytes.NewReader(makeArchive(t, archiveEntry{name: name, data: "package main"})), root, defaultArchiveLimits); err == nil {
				t.Fatalf("accepted unsafe path %q", name)
			}
		})
	}
	for _, entries := range [][]archiveEntry{
		{{name: "root/main.go", data: "package main"}, {name: "root/main.go", data: "package other"}},
		{{name: "root/.env", data: "first"}, {name: "root/.env", data: "second"}},
		{{name: "first/main.go", data: "package main"}, {name: "second/other.go", data: "package other"}},
	} {
		if _, err := extractArchive(bytes.NewReader(makeArchive(t, entries...)), t.TempDir(), defaultArchiveLimits); err == nil {
			t.Fatal("accepted duplicate regular paths or multiple roots")
		}
	}
}

func TestArchiveSkipsLinksAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	archive := makeArchive(t,
		archiveEntry{name: "root/link", kind: tar.TypeSymlink, link: "/tmp"},
		archiveEntry{name: "root/hard.go", kind: tar.TypeLink, link: "root/main.go"},
		archiveEntry{name: "root/main.go", data: "package main\n"},
		archiveEntry{name: "root/large.go", data: strings.Repeat("x", (2<<20)+1)},
	)
	warnings, err := extractArchive(bytes.NewReader(archive), root, defaultArchiveLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "large.go") {
		t.Fatalf("missing size warning: %v", warnings)
	}
	for _, name := range []string{"link", "hard.go", "large.go"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("materialized skipped entry %s: %v", name, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "main.go")); err != nil || string(b) != "package main\n" {
		t.Fatalf("valid source not extracted: %q, %v", b, err)
	}
}

func TestArchiveBoundsAndCorruption(t *testing.T) {
	archive := makeArchive(t, archiveEntry{name: "root/main.go", data: strings.Repeat("x", 4096)}, archiveEntry{name: "root/other.go", data: "package main"})
	for _, limits := range []archiveLimits{
		{compressed: 20, expanded: 1 << 20, entries: 100},
		{compressed: 1 << 20, expanded: 1000, entries: 100},
		{compressed: 1 << 20, expanded: 1 << 20, entries: 1},
	} {
		if _, err := extractArchive(bytes.NewReader(archive), t.TempDir(), limits); err == nil {
			t.Fatalf("ignored archive bounds %+v", limits)
		}
	}
	for _, bad := range [][]byte{[]byte("not gzip"), archive[:len(archive)-8]} {
		if _, err := extractArchive(bytes.NewReader(bad), t.TempDir(), defaultArchiveLimits); err == nil {
			t.Fatal("accepted corrupt archive")
		}
	}
}

func TestArchiveCannotWriteThroughLinks(t *testing.T) {
	outside, root := t.TempDir(), t.TempDir()
	guard := filepath.Join(outside, "guard.go")
	if err := os.WriteFile(guard, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := makeArchive(t,
		archiveEntry{name: "root/link", kind: tar.TypeSymlink, link: outside},
		archiveEntry{name: "root/link/guard.go", data: "package overwritten\n"},
	)
	if _, err := extractArchive(bytes.NewReader(archive), root, defaultArchiveLimits); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(guard); err != nil || string(b) != "unchanged" {
		t.Fatalf("escaped private root: %q %v", b, err)
	}
	info, err := os.Lstat(filepath.Join(root, "link"))
	if err != nil || !info.IsDir() {
		t.Fatalf("safe replacement directory missing: %v %v", info, err)
	}
}

func TestCollectRemovesTemporarySourceOnSuccessAndFailure(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("TMPDIR", parent)
	for _, valid := range []bool{true, false} {
		archive := makeArchive(t, archiveEntry{name: "root/main.go", data: "package main\n"})
		if !valid {
			archive = makeArchive(t, archiveEntry{name: "root/../../outside.go", data: "package main\n"})
		}
		c := collector{fetch: fakeFetch(map[string][]byte{
			"repos/Example/service":                    []byte(`{"full_name":"Example/service","default_branch":"main"}`),
			"repos/Example/service/commits/main":       jsonResponse(map[string]string{"sha": testSHA}),
			"repos/Example/service/tarball/" + testSHA: archive,
		}, nil, nil)}
		_, err := c.collect(context.Background(), "", []string{"Example/service"}, radar.DetectionRules{}, nil)
		if (err == nil) != valid {
			t.Fatalf("valid=%v: %v", valid, err)
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Fatalf("retained temporary sources: %v %v", entries, err)
		}
	}
}

func TestCollectRejectsInvalidCommitAndArchiveFailures(t *testing.T) {
	for _, commit := range []string{"main", "abc", strings.Repeat("a", 39), strings.Repeat("g", 40)} {
		c := collector{fetch: fakeFetch(map[string][]byte{"repos/Example/service": []byte(`{"full_name":"Example/service","default_branch":"main"}`), "repos/Example/service/commits/main": jsonResponse(map[string]string{"sha": commit})}, nil, nil)}
		if got, err := c.collect(context.Background(), "", []string{"Example/service"}, radar.DetectionRules{}, nil); err == nil || got != nil {
			t.Fatalf("accepted unpinned revision %q", commit)
		}
	}
	c := collector{fetch: fakeFetch(map[string][]byte{"repos/Example/service": []byte(`{"full_name":"Example/service","default_branch":"main"}`), "repos/Example/service/commits/main": jsonResponse(map[string]string{"sha": testSHA})}, map[string]error{"repos/Example/service/tarball/" + testSHA: errors.New("download failed")}, nil)}
	if got, err := c.collect(context.Background(), "", []string{"Example/service"}, radar.DetectionRules{}, nil); err == nil || got != nil {
		t.Fatalf("archive failure became success: %+v %v", got, err)
	}
}

func TestCollectRejectsNullMembershipAndMetadataFailure(t *testing.T) {
	c := collector{fetch: fakeFetch(map[string][]byte{"orgs/Example/repos?type=all&sort=full_name&direction=asc&per_page=100&page=1": []byte("null")}, nil, nil)}
	if got, err := c.collect(context.Background(), "Example", nil, radar.DetectionRules{}, nil); err == nil || got != nil {
		t.Fatalf("null membership became empty success: %+v %v", got, err)
	}
	c = collector{fetch: fakeFetch(nil, map[string]error{"repos/Example/service": errors.New("denied")}, nil)}
	if got, err := c.collect(context.Background(), "", []string{"Example/service"}, radar.DetectionRules{}, nil); err == nil || got != nil {
		t.Fatalf("metadata failure became success: %+v %v", got, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeGitHubTransportUsesFixedHostAndExistingAuthentication(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-token")
	t.Setenv("GH_HOST", "other.example")
	t.Setenv("GH_DEBUG", "api")
	c, err := newGitHubCollector(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || req.Method != "GET" || req.URL.Path != "/repos/Example/service" {
			t.Fatalf("unexpected request: %v", req.URL)
		}
		if req.Header.Get("Authorization") != "token fixture-token" {
			t.Fatal("native gh authentication was not applied")
		}
		return &http.Response{StatusCode: 409, Header: http.Header{"Content-Type": []string{"application/json"}}, Request: req, Body: io.NopCloser(strings.NewReader(`{"message":"Git Repository is empty."}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = c.fetch(context.Background(), "repos/Example/service", &out)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.status != 409 || apiErr.message != "Git Repository is empty." {
		t.Fatalf("structured empty error not preserved: %#v %v", apiErr, err)
	}
	if strings.Contains(err.Error(), "fixture-token") || strings.Contains(err.Error(), "Git Repository is empty.") {
		t.Fatalf("sensitive response leaked into error: %v", err)
	}
}

func TestNativeGitHubTransportRejectsForeignRedirect(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-token")
	requests := 0
	c, err := newGitHubCollector(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.example/archive.tar.gz"}}, Request: req, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.fetch(context.Background(), "repos/Example/service/tarball/"+testSHA, io.Discard); err == nil || requests != 1 {
		t.Fatalf("followed foreign redirect: %v (%d requests)", err, requests)
	}
}

type observedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *observedBody) Close() error { b.closed = true; return nil }

func TestNativeGitHubTransportBoundsErrorBodiesAndClosesStreams(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-token")
	body := &observedBody{Reader: strings.NewReader(strings.Repeat("x", maxErrorBodyBytes*2))}
	c, err := newGitHubCollector(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{"Content-Type": []string{"application/json"}}, Request: req, Body: body}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.fetch(context.Background(), "repos/Example/service", io.Discard); err == nil {
		t.Fatal("API error became success")
	}
	if body.read > maxErrorBodyBytes || !body.closed {
		t.Fatalf("unbounded/unclosed error body: %d bytes, closed=%v", body.read, body.closed)
	}
	body = &observedBody{Reader: strings.NewReader("streamed source")}
	c, err = newGitHubCollector(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/octet-stream"}}, Request: req, Body: body}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := c.fetch(context.Background(), "repos/Example/service/tarball/"+testSHA, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "streamed source" || !body.closed {
		t.Fatalf("stream not copied and closed: %q, closed=%v", out.String(), body.closed)
	}
}

func TestCollectCanceledContextDoesNotRequestMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := collector{fetch: func(context.Context, string, io.Writer) error { t.Fatal("requested canceled collection"); return nil }}
	if _, err := c.collect(ctx, "", []string{"Example/service"}, radar.DetectionRules{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("missing context cancellation: %v", err)
	}
}
