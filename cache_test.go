package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCachePathForTildePathPreservesExtension(t *testing.T) {
	got := cachePathForSSH("/tmp/ropen-cache", "vm1", "~/Files/Github/research/asi_drops/drop.zip")
	if filepath.Ext(got) != ".zip" {
		t.Fatalf("cache path = %q", got)
	}
	if !strings.Contains(got, filepath.Join("ssh", "vm1")) {
		t.Fatalf("cache path = %q", got)
	}
}
