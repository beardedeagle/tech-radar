package discovery

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"tech-radar/internal/radar"
)

type archiveLimits struct {
	compressed, expanded int64
	entries              int
}

var defaultArchiveLimits = archiveLimits{compressed: 512 << 20, expanded: 2 << 30, entries: 100_000}

func extractArchive(input io.Reader, root string, limits archiveLimits) ([]string, error) {
	compressed := &boundedReader{reader: input, remaining: limits.compressed, label: "compressed archive"}
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	expanded := &boundedReader{reader: gz, remaining: limits.expanded, label: "expanded archive"}
	tr := tar.NewReader(expanded)
	confined, err := os.OpenRoot(root)
	if err != nil {
		return nil, errors.New("open private capture directory failed")
	}
	defer confined.Close()
	seen := map[string]bool{}
	archiveRoot := ""
	var warnings []string
	for entries := 0; ; entries++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if entries >= limits.entries {
			return nil, errors.New("archive entry count exceeds limit")
		}
		name, prefix, err := archivePath(h.Name, h.Typeflag == tar.TypeDir)
		if err != nil {
			return nil, err
		}
		if archiveRoot == "" {
			archiveRoot = prefix
		}
		if prefix != archiveRoot {
			return nil, errors.New("archive contains multiple root directories")
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if name == "" {
			return nil, errors.New("archive contains a root-level regular file")
		}
		if seen[name] {
			return nil, errors.New("archive contains a duplicate regular file path")
		}
		seen[name] = true
		if !radar.PortfolioPathEligible(name) {
			continue
		}
		if h.Size > radar.MaxFileBytes {
			warnings = append(warnings, fmt.Sprintf("Skipped %s: eligible file exceeds 2 MiB.", name))
			continue
		}
		if err := confined.MkdirAll(path.Dir(name), 0700); err != nil {
			return nil, errors.New("create captured file directory failed")
		}
		f, err := confined.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, errors.New("create captured file failed")
		}
		_, copyErr := io.CopyN(f, tr, h.Size)
		closeErr := f.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, errors.New("close captured file failed")
		}
	}
	// Read past tar's end marker to check gzip's checksum and count all trailing data.
	if _, err := io.Copy(io.Discard, expanded); err != nil {
		return nil, err
	}
	return warnings, nil
}

func archivePath(raw string, directory bool) (string, string, error) {
	if len(raw) > 4096 {
		return "", "", errors.New("archive path exceeds 4096 bytes")
	}
	if strings.ContainsAny(raw, "\\\x00") || strings.HasPrefix(raw, "/") {
		return "", "", errors.New("archive contains an unsafe path")
	}
	name := raw
	if directory {
		name = strings.TrimSuffix(name, "/")
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", errors.New("archive contains an unsafe path component")
		}
	}
	if strings.Contains(parts[0], ":") {
		return "", "", errors.New("archive contains an absolute drive path")
	}
	return strings.Join(parts[1:], "/"), parts[0], nil
}

type boundedReader struct {
	reader    io.Reader
	remaining int64
	label     string
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if r.remaining < 0 {
		return 0, fmt.Errorf("%s exceeds byte limit", r.label)
	}
	if int64(len(p)) > r.remaining+1 {
		p = p[:r.remaining+1]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	if r.remaining < 0 {
		return n, fmt.Errorf("%s exceeds byte limit", r.label)
	}
	return n, err
}
