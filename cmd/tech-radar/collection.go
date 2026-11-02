package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"tech-radar/internal/discovery"
	"tech-radar/internal/radar"
)

// collectRepositoryInputs is shared by discover and check. Collection errors
// never return a partial repository set.
func collectRepositoryInputs(paths repoPaths, org, list string, rules radar.DetectionRules, progress io.Writer, ignored ...[32]byte) ([]radar.RepositorySnapshot, error) {
	var inputs []repositoryInput
	for _, p := range paths {
		inputs = append(inputs, repositoryInput{Path: p})
	}
	if list != "" {
		data, err := readInput(list)
		if err != nil {
			return nil, err
		}
		inputs, err = readRepositoryList(data)
		if err != nil {
			return nil, err
		}
		for i := range inputs {
			if inputs[i].Path != "" && !filepath.IsAbs(inputs[i].Path) {
				inputs[i].Path = filepath.Join(filepath.Dir(list), inputs[i].Path)
			}
		}
	}
	var snapshots []radar.RepositorySnapshot
	var remote []string
	for _, input := range inputs {
		if input.GitHub != "" {
			remote = append(remote, input.GitHub)
			continue
		}
		root, err := filepath.Abs(input.Path)
		if err != nil {
			return nil, err
		}
		name := input.Name
		if name == "" {
			name = filepath.Base(root)
		}
		inv, files, err := radar.CaptureRepository(root, rules, ignored...)
		if err != nil {
			return nil, fmt.Errorf("capture %s: %w", name, err)
		}
		snapshots = append(snapshots, radar.RepositorySnapshot{ID: "local:" + name, Name: name, Origin: "local", Revision: inv.Source.GitHead, Status: "captured", Inventory: inv, Files: files})
	}
	if org != "" || len(remote) > 0 {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		captured, err := discovery.Collect(ctx, org, remote, rules, progress, ignored...)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, captured...)
	}
	return snapshots, nil
}
