// Package discovery reads GitHub repos at pinned commits using go-gh.
// It reuses existing github.com credentials; keychain access may need gh.
// Repos are read as archives, without cloning or running their code.
//
// Archives allow 512 MiB compressed, 2 GiB expanded and 100,000 entries.
// Eligible regular files up to 2 MiB are written to a private temporary directory.
// Paths allow 4,096 bytes, metadata 16 MiB and error responses 64 KiB.
// Listings allow 1,000 pages of 100 repos; requests time out after ten minutes.
// Exceeding a collection limit returns an error instead of partial results.
package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"tech-radar/internal/radar"
)

const maxMetadataBytes = 16 << 20
const maxRepositoryPages = 1000

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var commitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

type repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
}

type collector struct {
	ignored [][32]byte
	fetch   func(context.Context, string, io.Writer) error
}

// Collect reads either an organization or a list of owner/repo names.
// Supply exactly one. The org is listed once and each repo is pinned to a commit
// before its archive is read. Errors return no partial result.
// Snapshots contain findings and hashes, not source files.
// If progress is set, it receives the name of each repo as capture starts.
// ignored holds SHA-256 digests of explicit runtime inputs to omit from capture.
func Collect(ctx context.Context, org string, repos []string, rules radar.DetectionRules, progress io.Writer, ignored ...[32]byte) ([]radar.RepositorySnapshot, error) {
	if err := validateSelectors(org, repos); err != nil {
		return nil, err
	}
	c, err := newGitHubCollector(http.DefaultTransport)
	if err != nil {
		return nil, err
	}
	c.ignored = append([][32]byte(nil), ignored...)
	return c.collect(ctx, org, repos, rules, progress)
}

func (c collector) collect(ctx context.Context, org string, repos []string, rules radar.DetectionRules, progress io.Writer) ([]radar.RepositorySnapshot, error) {
	if err := validateSelectors(org, repos); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	listed, err := c.repositories(ctx, org, repos)
	if err != nil {
		return nil, err
	}
	snapshots := make([]radar.RepositorySnapshot, 0, len(listed))
	for _, repo := range listed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			fmt.Fprintf(progress, "Capturing %s\n", repo.FullName)
		}
		snapshot, err := c.capture(ctx, repo, rules)
		if err != nil {
			return nil, fmt.Errorf("capture %s: %w", repo.FullName, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].ID < snapshots[j].ID })
	return snapshots, nil
}

func validateSelectors(org string, repos []string) error {
	if (org == "") == (len(repos) == 0) {
		return errors.New("supply exactly one GitHub organization or nonempty repository list")
	}
	if org != "" && !ownerPattern.MatchString(org) {
		return errors.New("invalid GitHub organization name")
	}
	seen := map[string]bool{}
	for _, name := range repos {
		if !validRepository(name) {
			return fmt.Errorf("invalid GitHub repository name %q; expected owner/repository", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate GitHub repository %s", name)
		}
		seen[key] = true
	}
	return nil
}

func validRepository(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && ownerPattern.MatchString(parts[0]) &&
		len(parts[1]) <= 100 && repositoryPattern.MatchString(parts[1]) && parts[1] != "." && parts[1] != ".."
}

func (c collector) repositories(ctx context.Context, org string, names []string) ([]repository, error) {
	var repos []repository
	if org == "" {
		for _, name := range names {
			var repo repository
			if err := c.json(ctx, "repos/"+name, &repo); err != nil {
				return nil, fmt.Errorf("read requested repository %s: %w", name, err)
			}
			repos = append(repos, repo)
		}
	} else {
		for page := 1; ; page++ {
			if page > maxRepositoryPages {
				return nil, errors.New("organization listing exceeds 100,000 repositories")
			}
			endpoint := fmt.Sprintf("orgs/%s/repos?type=all&sort=full_name&direction=asc&per_page=100&page=%d", org, page)
			var batch []repository
			if err := c.json(ctx, endpoint, &batch); err != nil {
				return nil, fmt.Errorf("list organization repositories page %d: %w", page, err)
			}
			if len(batch) > 100 {
				return nil, errors.New("organization listing returned an oversized page")
			}
			repos = append(repos, batch...)
			if len(batch) < 100 {
				break
			}
		}
	}
	seen := map[string]bool{}
	for _, repo := range repos {
		if !validRepository(repo.FullName) || repo.DefaultBranch == "" || strings.ContainsAny(repo.DefaultBranch, "\x00\r\n") {
			return nil, errors.New("GitHub returned invalid repository metadata")
		}
		key := strings.ToLower(repo.FullName)
		if seen[key] {
			return nil, fmt.Errorf("GitHub listing contains duplicate repository %s", repo.FullName)
		}
		seen[key] = true
	}
	return repos, nil
}

func (c collector) json(ctx context.Context, endpoint string, target any) error {
	var b bytes.Buffer
	if err := c.fetch(ctx, endpoint, &boundedWriter{writer: &b, remaining: maxMetadataBytes}); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(b.Bytes()), []byte("null")) {
		return errors.New("GitHub returned null metadata")
	}
	if err := json.Unmarshal(b.Bytes(), target); err != nil {
		return fmt.Errorf("invalid GitHub metadata response: %w", err)
	}
	return nil
}

func (c collector) capture(ctx context.Context, repo repository, rules radar.DetectionRules) (snapshot radar.RepositorySnapshot, err error) {
	var commit struct {
		SHA string `json:"sha"`
	}
	err = c.json(ctx, "repos/"+repo.FullName+"/commits/"+url.PathEscape(repo.DefaultBranch), &commit)
	status := "captured"
	if err != nil {
		var apiErr *apiError
		if !errors.As(err, &apiErr) || apiErr.status != 409 || !strings.EqualFold(strings.TrimRight(strings.TrimSpace(apiErr.message), ".!?"), "Git Repository is empty") {
			return radar.RepositorySnapshot{}, fmt.Errorf("resolve default branch: %w", err)
		}
		status = "empty"
	} else if !commitPattern.MatchString(commit.SHA) {
		return radar.RepositorySnapshot{}, errors.New("GitHub did not return a full commit SHA")
	}
	commit.SHA = strings.ToLower(commit.SHA)
	root, err := os.MkdirTemp("", "tech-radar-discovery-")
	if err != nil {
		return radar.RepositorySnapshot{}, errors.New("create private capture directory failed")
	}
	defer func() {
		if cleanupErr := os.RemoveAll(root); cleanupErr != nil {
			snapshot = radar.RepositorySnapshot{}
			err = errors.Join(err, errors.New("remove private capture directory failed"))
		}
	}()
	var warnings []string
	if status != "empty" {
		warnings, err = c.archive(ctx, "repos/"+repo.FullName+"/tarball/"+commit.SHA, root)
		if err != nil {
			return radar.RepositorySnapshot{}, err
		}
	}
	inv, files, err := radar.CaptureRepository(root, rules, c.ignored...)
	if err != nil {
		return radar.RepositorySnapshot{}, fmt.Errorf("inspect captured files: %w", err)
	}
	inv.Source.GitHead = commit.SHA
	inv.Warnings = append(inv.Warnings, warnings...)
	sort.Strings(inv.Warnings)
	return radar.RepositorySnapshot{
		ID: "github:" + strings.ToLower(repo.FullName), Name: repo.FullName, Origin: "github", Revision: commit.SHA,
		Archived: repo.Archived, Fork: repo.Fork, Status: status, Inventory: inv, Files: files,
	}, nil
}

func (c collector) archive(ctx context.Context, endpoint, root string) ([]string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := c.fetch(ctx, endpoint, writer)
		writer.CloseWithError(err)
		done <- err
	}()
	warnings, extractErr := extractArchive(reader, root, defaultArchiveLimits)
	reader.CloseWithError(extractErr)
	if extractErr != nil {
		cancel()
	}
	fetchErr := <-done
	if fetchErr != nil {
		return nil, fmt.Errorf("download pinned archive: %w", fetchErr)
	}
	if extractErr != nil {
		return nil, fmt.Errorf("read pinned archive: %w", extractErr)
	}
	return warnings, nil
}

// apiError keeps the status/message needed to identify empty repos.
// Error omits response bodies so credentials can't leak into CLI output.
type apiError struct {
	status  int
	message string
	cause   error
}

func (e *apiError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("GitHub API request failed (HTTP %d)", e.status)
	}
	return "GitHub API request failed"
}

func (e *apiError) Unwrap() error { return e.cause }

type boundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("GitHub metadata exceeds 16 MiB")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}
