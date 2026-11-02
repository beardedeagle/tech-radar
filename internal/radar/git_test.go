package radar

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestGitHeadLookupHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a Unix executable fixture")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if head := readGitHead(ctx, t.TempDir()); head != "" {
		t.Fatalf("unexpected revision: %s", head)
	}
	if time.Since(started) > time.Second {
		t.Fatal("Git metadata lookup ignored cancellation")
	}
}
