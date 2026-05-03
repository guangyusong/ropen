package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type remoteFileInfo struct {
	Size    int64
	ModTime time.Time
	Path    string
}

func fetchSSH(target Target, cfg Config, refresh bool, dryRun bool) (string, error) {
	if !isAllowedRemotePath(target.Path, cfg.allowedRootsForHost(target.Host)) {
		return "", fmt.Errorf("remote path %q is outside allowed roots for %s; add an allowed root in config or pass a safer path", target.Path, target.Host)
	}

	alias := cfg.hostAlias(target.Host, target.User)
	if dryRun {
		localPath := cachePathForSSH(cfg.CacheDir, target.Host, target.Path)
		fmt.Fprintf(os.Stderr, "would scp %s:%s -> %s\n", alias, target.Path, localPath)
		return localPath, nil
	}

	info, err := statRemote(alias, target.Path)
	if err != nil {
		return "", err
	}
	maxBytes := cfg.maxBytesForHost(target.Host)
	if info.Size > maxBytes {
		return "", fmt.Errorf("remote file is %s, above max-size %s", formatBytes(info.Size), formatBytes(maxBytes))
	}

	remotePath := firstNonEmpty(info.Path, target.Path)
	localPath := cachePathForSSH(cfg.CacheDir, target.Host, remotePath)
	if !refresh && cacheIsFresh(localPath, info) {
		return localPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", err
	}
	tmp := atomicTarget(localPath)
	remoteSpec := alias + ":" + remotePath
	cmd := exec.Command("scp", "-p", remoteSpec, tmp)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("copy remote file failed: %s", sshErrorHint("scp", alias, remotePath, err, stderr.String()))
	}
	if err := os.Rename(tmp, localPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = os.Chtimes(localPath, info.ModTime, info.ModTime)
	return localPath, nil
}

func statRemote(alias string, remotePath string) (remoteFileInfo, error) {
	quotedPath := shellQuote(remotePath)
	script := "p=" + quotedPath + `; ` +
		`case "$p" in '~') p="$HOME" ;; '~/'*) p="$HOME/${p#\~/}" ;; esac; ` +
		`if [ ! -e "$p" ]; then echo "not found: $p" >&2; exit 2; fi; ` +
		`if [ -d "$p" ]; then echo "is a directory: $p" >&2; exit 3; fi; ` +
		`if stat -c '%s %Y' -- "$p" >/dev/null 2>&1; then stat -c '%s %Y' -- "$p"; ` +
		`elif stat -f '%z %m' -- "$p" >/dev/null 2>&1; then stat -f '%z %m' -- "$p"; ` +
		`else wc -c < "$p" | awk '{print $1 " 0"}'; fi; ` +
		`printf '%s\n' "$p"`
	cmd := exec.Command("ssh", alias, "sh -lc "+shellQuote(script))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return remoteFileInfo{}, fmt.Errorf("inspect remote file failed: %s", sshErrorHint("ssh", alias, remotePath, err, stderr.String()))
	}
	output := strings.TrimRight(stdout.String(), "\r\n")
	lines := strings.Split(output, "\n")
	fields := strings.Fields(lines[0])
	if len(fields) < 2 {
		return remoteFileInfo{}, fmt.Errorf("remote stat returned unexpected output: %q", strings.TrimSpace(stdout.String()))
	}
	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return remoteFileInfo{}, fmt.Errorf("parse remote size: %w", err)
	}
	mtime, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return remoteFileInfo{}, fmt.Errorf("parse remote mtime: %w", err)
	}
	resolvedPath := remotePath
	if len(lines) > 1 {
		resolvedPath = lines[len(lines)-1]
	}
	return remoteFileInfo{Size: size, ModTime: time.Unix(mtime, 0), Path: resolvedPath}, nil
}

func cacheIsFresh(localPath string, remote remoteFileInfo) bool {
	info, err := os.Stat(localPath)
	if err != nil {
		return false
	}
	if info.Size() != remote.Size {
		return false
	}
	if remote.ModTime.IsZero() || remote.ModTime.Unix() == 0 {
		return true
	}
	return info.ModTime().Unix() >= remote.ModTime.Unix()
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func sshErrorHint(tool string, alias string, remotePath string, err error, stderr string) string {
	message := strings.TrimSpace(stderr)
	lower := strings.ToLower(message)
	target := alias + ":" + remotePath
	switch {
	case strings.Contains(lower, "permission denied"):
		return fmt.Sprintf("%s authentication failed for %s; check your SSH config, ssh-agent, and IdentityFile settings. %v: %s", tool, target, err, message)
	case strings.Contains(lower, "could not resolve hostname") || strings.Contains(lower, "name or service not known"):
		return fmt.Sprintf("%s could not resolve host for %s; check the host alias or ~/.ssh/config. %v: %s", tool, target, err, message)
	case strings.Contains(lower, "no such file") || strings.Contains(lower, "not found"):
		return fmt.Sprintf("%s could not find %s; the clicked text may be a partial/wrapped path or the file no longer exists. %v: %s", tool, target, err, message)
	case strings.Contains(lower, "is a directory"):
		return fmt.Sprintf("%s target is a directory, not a file: %s. %v: %s", tool, target, err, message)
	case strings.Contains(lower, "connection timed out") || strings.Contains(lower, "operation timed out"):
		return fmt.Sprintf("%s timed out connecting to %s; check network/Tailscale/VPN reachability. %v: %s", tool, alias, err, message)
	}
	if message == "" {
		return fmt.Sprintf("%s failed for %s: %v", tool, target, err)
	}
	return fmt.Sprintf("%s failed for %s: %v: %s", tool, target, err, message)
}
