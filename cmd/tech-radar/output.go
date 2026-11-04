package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"

	"tech-radar/internal/radar"
)

// outputFile holds the write lock and the output's original identity and hash.
// Keep the lock file in place so later writers lock the same inode.
type outputFile struct {
	path  string
	prior outputState
	lock  *flock.Flock
}

// beginOutput locks name and records the original file state. It returns an
// error on contention or failed validation. Call it before reading saved state,
// and Close the returned file when done.
func beginOutput(name string, force bool, protectedPaths ...string) (_ *outputFile, err error) {
	abs, err := resolveOutput(name, protectedPaths...)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return nil, err
	}
	lockPath := abs + ".lock"
	if err := rejectSymlinks(lockPath); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(lockPath); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("output lock must be a regular file: %s", lockPath)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lock := flock.New(lockPath, flock.SetFlag(os.O_CREATE|os.O_RDWR))
	defer func() {
		if err != nil {
			err = errors.Join(err, lock.Close())
		}
	}()
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock output %s: %w", abs, err)
	}
	if !locked {
		return nil, fmt.Errorf("another tech-radar run is writing %s; retry when it finishes", abs)
	}
	if err := rejectSymlinks(lockPath); err != nil {
		return nil, err
	}
	opened, err := lock.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(lockPath)
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return nil, fmt.Errorf("output lock changed while opening: %s", lockPath)
	}
	_, prior, err := prepareOutput(abs, force, protectedPaths...)
	if err != nil {
		return nil, err
	}
	return &outputFile{path: abs, prior: prior, lock: lock}, nil
}

// Close releases the lock but leaves its file in place.
func (f *outputFile) Close() error {
	if err := f.lock.Close(); err != nil {
		return fmt.Errorf("release output lock for %s: %w", f.path, err)
	}
	return nil
}

// writeDocument writes the evidence file before the report that links it.
// A failed write can leave unused evidence, but can't leave a broken report link.
// Existing evidence files must match byte for byte.
func (f *outputFile) writeDocument(report radar.Report, format string, protectedPaths ...string) error {
	name := f.path
	evidence, err := radar.Render(report, "json")
	if err != nil {
		return err
	}
	digest := sha256.Sum256(evidence)
	sha := hex.EncodeToString(digest[:])
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)) + ".evidence." + sha + ".json"
	companion := filepath.Join(filepath.Dir(name), base)
	if _, err := outputPath(companion, true, protectedPaths...); err != nil {
		return err
	}
	document, err := radar.RenderDocument(report, format, base, sha)
	if err != nil {
		return err
	}
	prior, err := readInput(companion)
	if err == nil {
		if !bytes.Equal(prior, evidence) {
			return fmt.Errorf("refusing to replace different evidence companion %s", companion)
		}
	} else if os.IsNotExist(err) {
		if err := writeOutput(companion, evidence, false, protectedPaths...); err != nil {
			return err
		}
	} else {
		return err
	}
	return f.write(document)
}

func rejectSymlinks(name string) error {
	for current := name; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink output path %s", current)
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

type outputState struct {
	info   os.FileInfo
	digest [32]byte
}

func outputPath(name string, force bool, protectedPaths ...string) (string, error) {
	abs, _, err := prepareOutput(name, force, protectedPaths...)
	return abs, err
}

func resolveOutput(name string, protectedPaths ...string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	// Resolve macOS system aliases such as /tmp. Other symlink parents stay rejected.
	if runtime.GOOS == "darwin" {
		for _, prefix := range []string{"/var", "/tmp", "/etc"} {
			if strings.HasPrefix(abs, prefix+"/") {
				resolved, err := filepath.EvalSymlinks(prefix)
				if err != nil {
					return "", err
				}
				abs = resolved + strings.TrimPrefix(abs, prefix)
				break
			}
		}
	}
	for _, inputPath := range protectedPaths {
		if inputPath == "" {
			continue
		}
		policyAbs, err := filepath.Abs(inputPath)
		if err != nil {
			return "", err
		}
		inputInfo, _ := os.Stat(inputPath)
		outputInfo, _ := os.Stat(abs)
		if policyAbs == abs || (inputInfo != nil && outputInfo != nil && os.SameFile(inputInfo, outputInfo)) {
			return "", fmt.Errorf("output must not replace an input file")
		}
	}
	switch filepath.Base(abs) {
	case "package.json", "package-lock.json", "mix.exs", "mix.lock", "go.mod", "go.sum", ".tool-versions", "pyproject.toml", "requirements.txt", "requirements-dev.txt", "Gemfile", "composer.json", "plugin.json", ".mcp.json", "SKILL.md":
		return "", fmt.Errorf("output must not replace a source manifest")
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if ext != ".md" && ext != ".html" && ext != ".json" {
		return "", fmt.Errorf("output needs a .md, .html or .json filename")
	}
	if err := rejectSymlinks(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func prepareOutput(name string, force bool, protectedPaths ...string) (string, outputState, error) {
	var priorState outputState
	abs, err := resolveOutput(name, protectedPaths...)
	if err != nil {
		return "", priorState, err
	}
	if info, err := os.Lstat(abs); err == nil {
		if !force {
			return "", priorState, fmt.Errorf("%s already exists; use --force only to replace a generated export", name)
		}
		f, err := os.Open(abs)
		if err != nil {
			return "", priorState, err
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil {
			return "", priorState, err
		}
		if !os.SameFile(info, opened) {
			return "", priorState, fmt.Errorf("output changed while reading: %s", name)
		}
		prior, err := readOpenedInputLimit(f, abs, radar.MaxContextBytes)
		if err != nil {
			return "", priorState, err
		}
		if _, err := radar.ReadReport(prior); err != nil {
			if _, err := radar.ReadContext(prior); err != nil {
				if _, err := radar.ReadCheckResult(prior); err != nil {
					return "", priorState, fmt.Errorf("refusing to overwrite a non-generated or edited file: %s", name)
				}
			}
		}
		priorState = outputState{info: opened, digest: sha256.Sum256(prior)}
	} else if !os.IsNotExist(err) {
		return "", priorState, err
	}
	return abs, priorState, nil
}

func writeOutput(name string, data []byte, force bool, protectedPaths ...string) (err error) {
	f, err := beginOutput(name, force, protectedPaths...)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return f.write(data)
}

// write stages data, then checks the original output state before publishing it.
func (output *outputFile) write(data []byte) error {
	dir := filepath.Dir(output.path)
	f, err := os.CreateTemp(dir, ".tech-radar-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, 0644); err != nil {
		return err
	}
	return publishOutput(tmp, output.path, output.prior)
}

func publishOutput(tmp, abs string, prior outputState) error {
	if prior.info == nil {
		// Link fails if another process created the destination while we were writing.
		return os.Link(tmp, abs)
	}
	if err := rejectSymlinks(abs); err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(prior.info, info) {
		return fmt.Errorf("output changed during generation; keep the edit and regenerate: %s", abs)
	}
	data, err := readInputLimit(abs, radar.MaxContextBytes)
	if err != nil {
		return err
	}
	if sha256.Sum256(data) != prior.digest {
		return fmt.Errorf("output changed during generation; keep the edit and regenerate: %s", abs)
	}
	// Another program can still edit the file between this check and Rename.
	return os.Rename(tmp, abs)
}
