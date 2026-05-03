package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type doctorOptions struct {
	configPath string
	tty        string
	path       string
}

type doctorCheck struct {
	Name   string
	Status string
	Detail string
}

func runDoctor(args []string) error {
	var opts doctorOptions
	fs := flag.NewFlagSet("ropen doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.configPath, "config", "", "config file path")
	fs.StringVar(&opts.tty, "tty", "", "terminal tty to inspect, e.g. /dev/ttys006")
	fs.StringVar(&opts.path, "path", "", "path to parse with the provided tty")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `Usage:
  ropen doctor [--tty /dev/ttys006] [--path /tmp/file]

Checks local dependencies, config, iTerm2 setup, and optional SSH pane detection.

Options:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	checks := []doctorCheck{
		{Name: "version", Status: "ok", Detail: versionString()},
	}

	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		checks = append(checks, doctorCheck{Name: "config", Status: "fail", Detail: err.Error()})
	} else {
		path, _ := defaultConfigPath()
		if opts.configPath != "" {
			path = opts.configPath
		}
		checks = append(checks, doctorCheck{Name: "config", Status: "ok", Detail: path})
	}
	if cfg.CacheDir == "" {
		if cacheDir, err := defaultCacheDir(); err == nil {
			cfg.CacheDir = cacheDir
		}
	}
	if cfg.OpenCommand == "" {
		cfg.OpenCommand = defaultOpenCommand()
	}

	checks = append(checks, commandCheck("ssh", true))
	checks = append(checks, commandCheck("scp", true))
	checks = append(checks, commandCheck(cfg.OpenCommand, true))
	checks = append(checks, commandCheck("aws", false))
	checks = append(checks, commandCheck("gcloud", false))
	checks = append(checks, commandCheck("gsutil", false))
	checks = append(checks, commandCheck("az", false))
	checks = append(checks, commandCheck("rclone", false))

	if cfg.CacheDir != "" {
		checks = append(checks, cacheDirCheck(cfg.CacheDir))
	}
	if runtime.GOOS == "darwin" {
		checks = append(checks, itermRulesCheck())
	}
	if opts.tty != "" {
		checks = append(checks, ttyCheck(opts.tty, cfg))
	}
	if opts.path != "" || opts.tty != "" {
		checks = append(checks, parseCheck(opts, cfg))
	}

	failed := false
	for _, check := range checks {
		fmt.Printf("%-8s %-18s %s\n", check.Status, check.Name, check.Detail)
		if check.Status == "fail" {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("doctor found required failures")
	}
	return nil
}

func commandCheck(name string, required bool) doctorCheck {
	if name == "" {
		return doctorCheck{Name: "command", Status: "fail", Detail: "empty command"}
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return doctorCheck{Name: "command:" + name, Status: "ok", Detail: path}
	}
	status := "warn"
	if required {
		status = "fail"
	}
	return doctorCheck{Name: "command:" + name, Status: status, Detail: "not found in PATH"}
}

func cacheDirCheck(path string) doctorCheck {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return doctorCheck{Name: "cache", Status: "fail", Detail: err.Error()}
	}
	tmp, err := os.CreateTemp(path, ".ropen-doctor-*")
	if err != nil {
		return doctorCheck{Name: "cache", Status: "fail", Detail: err.Error()}
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return doctorCheck{Name: "cache", Status: "ok", Detail: path}
}

func itermRulesCheck() doctorCheck {
	home, err := os.UserHomeDir()
	if err != nil {
		return doctorCheck{Name: "iterm2", Status: "warn", Detail: err.Error()}
	}
	prefs := home + "/Library/Preferences/com.googlecode.iterm2.plist"
	if _, err := os.Stat(prefs); err != nil {
		return doctorCheck{Name: "iterm2", Status: "warn", Detail: "preferences not found"}
	}
	script := `
import pathlib, plistlib, sys
p = pathlib.Path(sys.argv[1])
data = plistlib.load(open(p, "rb"))
profiles = data.get("New Bookmarks") or []
installed = 0
for profile in profiles:
    rules = profile.get("Smart Selection Rules") or []
    if any(str(rule.get("notes", "")).startswith("ropen ") for rule in rules):
        installed += 1
if installed:
    print(f"ropen rules installed in {installed}/{len(profiles)} profiles")
else:
    print(f"ropen rules not found in {len(profiles)} profiles")
    sys.exit(2)
`
	cmd := exec.Command("python3", "-c", script, prefs)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stdout.String())
		if detail == "" {
			detail = strings.TrimSpace(stderr.String())
		}
		if detail == "" {
			detail = err.Error()
		}
		return doctorCheck{Name: "iterm2", Status: "warn", Detail: detail}
	}
	return doctorCheck{Name: "iterm2", Status: "ok", Detail: strings.TrimSpace(stdout.String())}
}

func ttyCheck(tty string, cfg Config) doctorCheck {
	host, user, err := detectSSHFromTTY(tty)
	if err != nil {
		return doctorCheck{Name: "tty", Status: "warn", Detail: err.Error()}
	}
	alias := cfg.hostAlias(host, user)
	return doctorCheck{Name: "tty", Status: "ok", Detail: fmt.Sprintf("%s -> %s", tty, alias)}
}

func parseCheck(opts doctorOptions, cfg Config) doctorCheck {
	target, err := parseTarget(targetInput{tty: opts.tty, path: opts.path})
	if err != nil {
		return doctorCheck{Name: "target", Status: "fail", Detail: err.Error()}
	}
	detail := fmt.Sprintf("%s %s", target.Kind, target.Path)
	if target.Kind == TargetSSH {
		detail = fmt.Sprintf("%s %s:%s", target.Kind, cfg.hostAlias(target.Host, target.User), target.Path)
	} else if target.Kind == TargetObject {
		detail = fmt.Sprintf("%s %s", target.Kind, target.URI)
	}
	return doctorCheck{Name: "target", Status: "ok", Detail: detail}
}
