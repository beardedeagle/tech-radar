package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tech-radar/internal/radar"
)

// readPrevious checks the report and its adjacent evidence file, if present.
// Without the evidence file, saved decisions and review flags still carry forward.
func readPrevious(name string) (radar.Report, error) {
	b, err := readInput(name)
	if err != nil {
		return radar.Report{}, err
	}
	r, err := radar.ReadReport(b)
	if err != nil || r.EvidenceFile == "" {
		return r, err
	}
	companion := filepath.Join(filepath.Dir(name), r.EvidenceFile)
	info, err := os.Lstat(companion)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() {
		return r, fmt.Errorf("evidence companion must be a regular file")
	}
	confined, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return r, err
	}
	defer confined.Close()
	f, err := confined.Open(r.EvidenceFile)
	if err != nil {
		return r, err
	}
	defer f.Close()
	b, err = readOpenedInput(f, companion)
	if err != nil {
		return r, err
	}
	digest := sha256.Sum256(b)
	if hex.EncodeToString(digest[:]) != r.EvidenceSHA256 {
		return r, fmt.Errorf("evidence file checksum doesn't match; keep these files and regenerate from an intact report")
	}
	full, err := radar.ReadReport(b)
	if err != nil {
		return r, err
	}
	if full.EvidenceFile != "" {
		return r, fmt.Errorf("evidence companion cannot reference another companion")
	}
	return full, nil
}

const maxInputBytes = 16 << 20

func readInput(name string) ([]byte, error) {
	return readInputLimit(name, maxInputBytes)
}

func readInputLimit(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readOpenedInputLimit(f, name, limit)
}

func readOpenedInput(f *os.File, name string) ([]byte, error) {
	return readOpenedInputLimit(f, name, maxInputBytes)
}

func readOpenedInputLimit(f *os.File, name string, limit int64) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular input file", name)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d MiB input limit", name, limit>>20)
	}
	return b, nil
}
