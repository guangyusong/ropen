package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	posixpath "path"
	"runtime"
	"strings"
	"time"
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

type sshTTYInfo struct {
	Host        string
	User        string
	Cwd         string
	ConnectArgs []string
	Tmux        tmuxContext
}

type sshInvocation struct {
	Host        string
	User        string
	ConnectArgs []string
	RemoteArgs  []string
}

type tmuxContext struct {
	SocketName string
	SocketPath string
	Session    string
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

	if host == "" || isLocalHost(host) {
		if in.tty != "" {
			sshInfo, err := detectSSHFromTTY(in.tty)
			if err == nil && sshInfo.Host != "" {
				ttyCwd := sshInfo.Cwd
				if isRelativePath(rawPath) && (cwd == "" || isLikelyLocalCwd(cwd)) {
					if liveCwd, err := queryRemoteTmuxCwd(sshInfo.ConnectArgs, sshInfo.Tmux); err == nil && isUsableRemoteCwd(liveCwd) {
						ttyCwd = liveCwd
					}
				}
				remoteCwd := chooseRemoteCwd(cwd, ttyCwd)
				cleanPath := cleanRemotePath(rawPath, remoteCwd)
				if cleanPath == "" {
					return Target{}, errors.New("empty path")
				}
				return Target{
					Kind: TargetSSH,
					Host: sshInfo.Host,
					User: firstNonEmpty(user, sshInfo.User),
					Cwd:  remoteCwd,
					Path: cleanPath,
					Line: in.line,
					Col:  in.col,
				}, nil
			}
		}
		cleanPath := cleanRemotePath(rawPath, cwd)
		if cleanPath == "" {
			return Target{}, errors.New("empty path")
		}
		return Target{
			Kind: TargetLocal,
			Path: cleanPath,
			Cwd:  cwd,
			Line: in.line,
			Col:  in.col,
		}, nil
	}

	cleanPath := cleanRemotePath(rawPath, cwd)
	if cleanPath == "" {
		return Target{}, errors.New("empty path")
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

func detectSSHFromTTY(tty string) (sshTTYInfo, error) {
	name := strings.TrimPrefix(tty, "/dev/")
	if name == "" || strings.Contains(name, "/") {
		return sshTTYInfo{}, fmt.Errorf("invalid tty %q", tty)
	}
	cmd := exec.Command("ps", "-t", name, "-o", "command=")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return sshTTYInfo{}, fmt.Errorf("ps for tty %s failed: %w: %s", tty, err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		args := shellFields(line)
		if len(args) == 0 || baseName(args[0]) != "ssh" {
			continue
		}
		inv := parseSSHInvocation(args[1:])
		if inv.Host != "" {
			return sshTTYInfo{
				Host:        inv.Host,
				User:        inv.User,
				Cwd:         parseRemoteCwd(inv.RemoteArgs),
				ConnectArgs: inv.ConnectArgs,
				Tmux:        parseTmuxContext(inv.RemoteArgs),
			}, nil
		}
	}
	return sshTTYInfo{}, fmt.Errorf("no ssh process found for tty %s", tty)
}

var sshOptionsWithArgs = map[string]bool{
	"-B": true, "-b": true, "-c": true, "-D": true, "-E": true,
	"-e": true, "-F": true, "-I": true, "-i": true, "-J": true,
	"-L": true, "-l": true, "-m": true, "-O": true, "-o": true,
	"-p": true, "-Q": true, "-R": true, "-S": true, "-W": true,
	"-w": true,
}

var sshQueryOptionsWithArgs = map[string]bool{
	"-B": true, "-b": true, "-c": true, "-F": true, "-I": true,
	"-i": true, "-J": true, "-l": true, "-m": true, "-o": true,
	"-p": true, "-S": true, "-w": true,
}

var sshQueryOptionsNoArgs = map[string]bool{
	"-4": true, "-6": true, "-A": true, "-a": true, "-C": true,
	"-K": true, "-k": true, "-X": true, "-x": true, "-Y": true,
}

func parseSSHDestination(args []string) (host string, user string, remoteArgs []string) {
	inv := parseSSHInvocation(args)
	return inv.Host, inv.User, inv.RemoteArgs
}

func parseSSHInvocation(args []string) sshInvocation {
	var user string
	var connectArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg == "--" {
			if i+1 < len(args) {
				host, splitUser := splitUserHost(args[i+1], user)
				connectArgs = append(connectArgs, args[i+1])
				return sshInvocation{
					Host:        host,
					User:        splitUser,
					ConnectArgs: connectArgs,
					RemoteArgs:  args[i+2:],
				}
			}
			return sshInvocation{}
		}
		if strings.HasPrefix(arg, "-") {
			if strings.HasPrefix(arg, "-l") && len(arg) > 2 {
				user = arg[2:]
				connectArgs = append(connectArgs, arg)
				continue
			}
			if arg == "-l" && i+1 < len(args) {
				user = args[i+1]
				connectArgs = append(connectArgs, arg, args[i+1])
				i++
				continue
			}
			if sshOptionsWithArgs[arg] && i+1 < len(args) {
				if sshQueryOptionsWithArgs[arg] {
					connectArgs = append(connectArgs, arg, args[i+1])
				}
				i++
				continue
			}
			if sshQueryOptionsNoArgs[arg] {
				connectArgs = append(connectArgs, arg)
			}
			continue
		}
		host, splitUser := splitUserHost(arg, user)
		connectArgs = append(connectArgs, arg)
		return sshInvocation{
			Host:        host,
			User:        splitUser,
			ConnectArgs: connectArgs,
			RemoteArgs:  args[i+1:],
		}
	}
	return sshInvocation{}
}

func splitUserHost(dest string, fallbackUser string) (host string, user string) {
	if at := strings.LastIndex(dest, "@"); at > 0 {
		return dest[at+1:], dest[:at]
	}
	return dest, fallbackUser
}

func parseRemoteCwd(args []string) string {
	if cwd := parseTmuxCwd(args); cwd != "" {
		return cwd
	}
	if len(args) == 1 {
		fields := shellFields(args[0])
		if cwd := parseTmuxCwd(fields); cwd != "" {
			return cwd
		}
		if cwd := parseCDPrefix(fields); cwd != "" {
			return cwd
		}
	}
	return parseCDPrefix(args)
}

func parseTmuxContext(args []string) tmuxContext {
	if len(args) == 1 {
		fields := shellFields(args[0])
		if len(fields) > 1 {
			return parseTmuxContext(fields)
		}
	}
	for i, arg := range args {
		if baseName(arg) != "tmux" {
			continue
		}
		var ctx tmuxContext
		for j := i + 1; j < len(args); j++ {
			switch args[j] {
			case "-L":
				if j+1 < len(args) {
					ctx.SocketName = args[j+1]
					j++
				}
			case "-S":
				if j+1 < len(args) {
					ctx.SocketPath = args[j+1]
					j++
				}
			case "-s", "-t":
				if j+1 < len(args) {
					ctx.Session = args[j+1]
					j++
				}
			}
		}
		return ctx
	}
	return tmuxContext{}
}

func parseTmuxCwd(args []string) string {
	for i, arg := range args {
		if baseName(arg) != "tmux" {
			continue
		}
		for j := i + 1; j < len(args); j++ {
			if args[j] == "-c" && j+1 < len(args) {
				return args[j+1]
			}
			if strings.HasPrefix(args[j], "-c") && len(args[j]) > 2 {
				return args[j][2:]
			}
		}
	}
	return ""
}

func queryRemoteTmuxCwd(connectArgs []string, tmux tmuxContext) (string, error) {
	if len(connectArgs) == 0 || !tmux.hasTarget() {
		return "", errors.New("missing ssh or tmux context")
	}
	args := append([]string{}, connectArgs...)
	tmuxArgs := []string{"tmux"}
	if tmux.SocketName != "" {
		tmuxArgs = append(tmuxArgs, "-L", tmux.SocketName)
	}
	if tmux.SocketPath != "" {
		tmuxArgs = append(tmuxArgs, "-S", tmux.SocketPath)
	}
	tmuxArgs = append(tmuxArgs, "display-message", "-p")
	if tmux.Session != "" {
		tmuxArgs = append(tmuxArgs, "-t", tmux.Session)
	}
	tmuxArgs = append(tmuxArgs, "#{pane_current_path}")
	args = append(args, "sh -lc "+shellQuote(shellJoin(tmuxArgs)))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func (t tmuxContext) hasTarget() bool {
	return t.SocketName != "" || t.SocketPath != "" || t.Session != ""
}

func parseCDPrefix(args []string) string {
	if len(args) >= 2 && args[0] == "cd" {
		return args[1]
	}
	return ""
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

func chooseRemoteCwd(itermCwd string, ttyCwd string) string {
	if itermCwd == "" {
		return ttyCwd
	}
	if ttyCwd == "" {
		return itermCwd
	}
	if isLikelyLocalCwd(itermCwd) {
		return ttyCwd
	}
	return itermCwd
}

func isLikelyLocalCwd(p string) bool {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return true
	}
	wd, err := os.Getwd()
	if err == nil && wd != "" && (p == wd || strings.HasPrefix(p, wd+"/")) {
		return true
	}
	return false
}

func isRelativePath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~")
}

func isUsableRemoteCwd(p string) bool {
	return strings.HasPrefix(p, "/") || p == "~" || strings.HasPrefix(p, "~/")
}

func cleanRemotePath(p string, cwd string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		return posixpath.Clean(p)
	}
	if strings.HasPrefix(p, "/") {
		return posixpath.Clean(p)
	}
	if cwd == "" {
		return posixpath.Clean(p)
	}
	if strings.HasPrefix(cwd, "/") || cwd == "~" || strings.HasPrefix(cwd, "~/") {
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
