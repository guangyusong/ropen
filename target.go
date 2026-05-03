package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	posixpath "path"
	"runtime"
	"strings"
)

var errNoTarget = errors.New("no target provided")

type TargetKind string

const (
	TargetLocal  TargetKind = "local"
	TargetSSH    TargetKind = "ssh"
	TargetObject TargetKind = "object"
)

type Target struct {
	Kind   TargetKind
	Scheme string
	URI    string
	Host   string
	User   string
	Cwd    string
	Path   string
	Line   string
	Col    string
}

type targetInput struct {
	args []string
	host string
	user string
	cwd  string
	tty  string
	path string
	line string
	col  string
}

func parseTarget(in targetInput) (Target, error) {
	host := firstNonEmpty(in.host, os.Getenv("ROPEN_HOST"))
	user := firstNonEmpty(in.user, os.Getenv("ROPEN_USER"))
	cwd := firstNonEmpty(in.cwd, os.Getenv("ROPEN_CWD"))
	rawPath := in.path

	if rawPath == "" {
		switch len(in.args) {
		case 0:
			return Target{}, errNoTarget
		case 1:
			rawPath = in.args[0]
		default:
			return Target{}, fmt.Errorf("expected one target, got %d", len(in.args))
		}
	}
	if rawPath == "" {
		return Target{}, errNoTarget
	}
	rawPath = normalizeTerminalWrappedPath(rawPath)
	if hasControl(rawPath) || hasControl(host) || hasControl(user) || hasControl(cwd) {
		return Target{}, errors.New("target contains control characters")
	}

	if scheme := objectScheme(rawPath); scheme != "" {
		return Target{
			Kind:   TargetObject,
			Scheme: scheme,
			URI:    rawPath,
			Line:   in.line,
			Col:    in.col,
		}, nil
	}

	if parsedHost, parsedPath, ok := parseSCPStyle(rawPath); ok {
		host = parsedHost
		rawPath = parsedPath
	}

	cleanPath := cleanRemotePath(rawPath, cwd)
	if cleanPath == "" {
		return Target{}, errors.New("empty path")
	}

	if host == "" || isLocalHost(host) {
		if in.tty != "" {
			sshHost, sshUser, err := detectSSHFromTTY(in.tty)
			if err == nil && sshHost != "" {
				return Target{
					Kind: TargetSSH,
					Host: sshHost,
					User: firstNonEmpty(user, sshUser),
					Cwd:  cwd,
					Path: cleanPath,
					Line: in.line,
					Col:  in.col,
				}, nil
			}
		}
		return Target{
			Kind: TargetLocal,
			Path: cleanPath,
			Cwd:  cwd,
			Line: in.line,
			Col:  in.col,
		}, nil
	}

	return Target{
		Kind: TargetSSH,
		Host: host,
		User: user,
		Cwd:  cwd,
		Path: cleanPath,
		Line: in.line,
		Col:  in.col,
	}, nil
}

func parseSCPStyle(s string) (host string, path string, ok bool) {
	if strings.Contains(s, "://") {
		return "", "", false
	}
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", "", false
	}
	prefix := s[:i]
	rest := s[i+1:]
	if strings.Contains(prefix, "/") || rest == "" {
		return "", "", false
	}
	return prefix, rest, true
}

func objectScheme(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "s3", "gs", "az", "rclone":
		return strings.ToLower(u.Scheme)
	default:
		return ""
	}
}

func normalizeTerminalWrappedPath(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if !strings.Contains(s, "\n") {
		return s
	}
	parts := strings.Split(s, "\n")
	var b strings.Builder
	for i, part := range parts {
		if i == 0 {
			b.WriteString(part)
			continue
		}
		b.WriteString(strings.TrimLeft(part, " \t"))
	}
	return b.String()
}

func detectSSHFromTTY(tty string) (host string, user string, err error) {
	name := strings.TrimPrefix(tty, "/dev/")
	if name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("invalid tty %q", tty)
	}
	cmd := exec.Command("ps", "-t", name, "-o", "command=")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("ps for tty %s failed: %w: %s", tty, err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		args := shellFields(line)
		if len(args) == 0 || baseName(args[0]) != "ssh" {
			continue
		}
		if h, u := parseSSHDestination(args[1:]); h != "" {
			return h, u, nil
		}
	}
	return "", "", fmt.Errorf("no ssh process found for tty %s", tty)
}

func parseSSHDestination(args []string) (host string, user string) {
	optionArgs := map[string]bool{
		"-B": true, "-b": true, "-c": true, "-D": true, "-E": true,
		"-e": true, "-F": true, "-I": true, "-i": true, "-J": true,
		"-L": true, "-l": true, "-m": true, "-O": true, "-o": true,
		"-p": true, "-Q": true, "-R": true, "-S": true, "-W": true,
		"-w": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg == "--" {
			if i+1 < len(args) {
				return splitUserHost(args[i+1], user)
			}
			return "", ""
		}
		if strings.HasPrefix(arg, "-") {
			if strings.HasPrefix(arg, "-l") && len(arg) > 2 {
				user = arg[2:]
				continue
			}
			if arg == "-l" && i+1 < len(args) {
				user = args[i+1]
				i++
				continue
			}
			if optionArgs[arg] && i+1 < len(args) {
				i++
			}
			continue
		}
		return splitUserHost(arg, user)
	}
	return "", ""
}

func splitUserHost(dest string, fallbackUser string) (host string, user string) {
	if at := strings.LastIndex(dest, "@"); at > 0 {
		return dest[at+1:], dest[:at]
	}
	return dest, fallbackUser
}

func shellFields(s string) []string {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	return fields
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func cleanRemotePath(p string, cwd string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		return p
	}
	if strings.HasPrefix(p, "/") {
		return posixpath.Clean(p)
	}
	if cwd == "" {
		return posixpath.Clean(p)
	}
	if strings.HasPrefix(cwd, "/") {
		return posixpath.Clean(posixpath.Join(cwd, p))
	}
	return posixpath.Clean(p)
}

func isAllowedRemotePath(p string, roots []string) bool {
	if strings.HasPrefix(p, "~") {
		return true
	}
	if !strings.HasPrefix(p, "/") {
		return false
	}
	cleaned := posixpath.Clean(p)
	for _, root := range roots {
		root = posixpath.Clean(root)
		if root == "/" {
			return true
		}
		if cleaned == root || strings.HasPrefix(cleaned, root+"/") {
			return true
		}
	}
	return false
}

func isLocalHost(host string) bool {
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	name, err := os.Hostname()
	if err == nil && (host == name || host == strings.Split(name, ".")[0]) {
		return true
	}
	if runtime.GOOS == "darwin" && strings.HasSuffix(host, ".local") {
		short := strings.TrimSuffix(host, ".local")
		if err == nil && (short == name || short == strings.Split(name, ".")[0]) {
			return true
		}
	}
	return false
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
