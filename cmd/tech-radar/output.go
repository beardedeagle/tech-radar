package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
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
