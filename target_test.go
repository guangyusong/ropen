package main

import "testing"

func TestParseTargetSCPStyle(t *testing.T) {
	target, err := parseTarget(targetInput{args: []string{"ubuntu@vm1:/home/ubuntu/output.mp4"}})
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != TargetSSH {
		t.Fatalf("kind = %q, want %q", target.Kind, TargetSSH)
	}
	if target.Host != "ubuntu@vm1" {
		t.Fatalf("host = %q", target.Host)
	}
	if target.Path != "/home/ubuntu/output.mp4" {
		t.Fatalf("path = %q", target.Path)
	}
}

func TestParseTargetItermFlagsRelativePath(t *testing.T) {
	target, err := parseTarget(targetInput{
		host: "vm1",
		user: "dev",
		cwd:  "/home/dev/project",
		path: "results.csv",
		line: "42",
		col:  "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != TargetSSH {
		t.Fatalf("kind = %q, want %q", target.Kind, TargetSSH)
	}
	if target.Path != "/home/dev/project/results.csv" {
		t.Fatalf("path = %q", target.Path)
	}
	if target.Line != "42" || target.Col != "7" {
		t.Fatalf("line/col = %q/%q", target.Line, target.Col)
	}
}

func TestParseTargetObject(t *testing.T) {
	target, err := parseTarget(targetInput{args: []string{"s3://bucket/path/file.csv"}})
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != TargetObject || target.Scheme != "s3" {
		t.Fatalf("target = %#v", target)
	}
}

func TestParseTargetWrappedPath(t *testing.T) {
	target, err := parseTarget(targetInput{host: "vm1", path: "/home/dev/output/\nlong-name.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != "/home/dev/output/long-name.mp4" {
		t.Fatalf("path = %q", target.Path)
	}
}

func TestParseTargetWrappedObject(t *testing.T) {
	target, err := parseTarget(targetInput{path: "s3://bucket/path/\nlong-name.csv"})
	if err != nil {
		t.Fatal(err)
	}
	if target.URI != "s3://bucket/path/long-name.csv" {
		t.Fatalf("uri = %q", target.URI)
	}
}

func TestParseSSHDestinationUserAtHost(t *testing.T) {
	host, user := parseSSHDestination([]string{"-i", "/tmp/key", "-o", "ServerAliveInterval=30", "dev@example-host", "-t", "tmux"})
	if host != "example-host" || user != "dev" {
		t.Fatalf("host/user = %q/%q", host, user)
	}
}

func TestParseSSHDestinationLoginOption(t *testing.T) {
	host, user := parseSSHDestination([]string{"-l", "ubuntu", "vm1"})
	if host != "vm1" || user != "ubuntu" {
		t.Fatalf("host/user = %q/%q", host, user)
	}
}

func TestShellFields(t *testing.T) {
	got := shellFields(`ssh dev@host -t tmux -c '~/project/'`)
	want := []string{"ssh", "dev@host", "-t", "tmux", "-c", "~/project/"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestAllowedRemotePath(t *testing.T) {
	roots := []string{"/home", "/tmp"}
	if !isAllowedRemotePath("/home/dev/file.txt", roots) {
		t.Fatal("expected /home path to be allowed")
	}
	if isAllowedRemotePath("/etc/passwd", roots) {
		t.Fatal("expected /etc path to be denied")
	}
}

func TestParseSize(t *testing.T) {
	got, err := parseSize("1.5GB")
	if err != nil {
		t.Fatal(err)
	}
	want := int64(1.5 * 1024 * 1024 * 1024)
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote("/tmp/it's fine.txt")
	want := `'/tmp/it'"'"'s fine.txt'`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
