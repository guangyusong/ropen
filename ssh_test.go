package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRemoteHomeExpansionSnippet(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-lc", `p='~/Files/Github/research/file.md'; case "$p" in '~') p="$HOME" ;; '~/'*) p="$HOME/${p#\~/}" ;; esac; printf '%s\n' "$p"`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(out))
	want := home + "/Files/Github/research/file.md"
	if got != want {
		t.Fatalf("expanded path = %q, want %q", got, want)
	}
}
