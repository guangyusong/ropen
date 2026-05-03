package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type itermInstallOptions struct {
	ropen    string
	prefs    string
	defaults string
}

func runIterm(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(`Usage:
  ropen iterm install [--ropen /path/to/ropen]

Commands:
  install   Install iTerm2 Smart Selection rules for ropen
`)
		return nil
	}
	switch args[0] {
	case "install":
		return runItermInstall(args[1:])
	default:
		return fmt.Errorf("unknown iterm command %q", args[0])
	}
}

func runItermInstall(args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iTerm2 integration is only supported on macOS")
	}
	var opts itermInstallOptions
	exe, _ := os.Executable()
	opts.ropen = exe
	home, err := os.UserHomeDir()
	if err == nil {
		opts.prefs = home + "/Library/Preferences/com.googlecode.iterm2.plist"
	}
	opts.defaults = "/Applications/iTerm.app/Contents/Resources/SmartSelectionRules.plist"

	fs := flag.NewFlagSet("ropen iterm install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.ropen, "ropen", opts.ropen, "absolute path to the ropen binary iTerm2 should run")
	fs.StringVar(&opts.prefs, "prefs", opts.prefs, "iTerm2 preferences plist")
	fs.StringVar(&opts.defaults, "defaults", opts.defaults, "iTerm2 default SmartSelectionRules plist")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `Usage:
  ropen iterm install [--ropen /path/to/ropen]

Installs ropen Smart Selection rules into each iTerm2 profile.

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
	if opts.ropen == "" {
		return fmt.Errorf("could not determine ropen executable path; pass --ropen")
	}

	cmd := exec.Command("python3", "-c", itermInstallPython, opts.ropen, opts.prefs, opts.defaults)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := stderr.String()
		if detail == "" {
			detail = stdout.String()
		}
		return fmt.Errorf("iTerm2 install failed: %w: %s", err, detail)
	}
	fmt.Print(stdout.String())
	return nil
}

const itermInstallPython = `
import datetime as dt
import pathlib
import plistlib
import shlex
import shutil
import subprocess
import sys
import tempfile

PATH_REGEX = r"""((?:/[^ \t\r\n"'<>]+(?:\r?\n)?)+)(?::([0-9]+)(?::([0-9]+))?)?"""
RELATIVE_PATH_REGEX = r"""((?:(?:\.{1,2}/)?[A-Za-z0-9._-]+/(?:[A-Za-z0-9._-]+/)*[A-Za-z0-9._-]+)(?:\r?\n[A-Za-z0-9._/-]+)*)(?::([0-9]+)(?::([0-9]+))?)?"""
OBJECT_REGEX = r"""((?:s3|gs|az|rclone)://[^ \t\r\n"'<>]+(?:\r?\n[^ \t\r\n"'<>]+)*)"""

ropen = pathlib.Path(sys.argv[1])
prefs_path = pathlib.Path(sys.argv[2])
defaults_path = pathlib.Path(sys.argv[3])

if not ropen.exists():
    raise SystemExit(f"ropen binary not found: {ropen}")
if not prefs_path.exists():
    raise SystemExit(f"iTerm2 preferences not found: {prefs_path}")
if not defaults_path.exists():
    raise SystemExit(f"iTerm2 default smart selection rules not found: {defaults_path}")

stamp = dt.datetime.utcnow().strftime("%Y%m%dT%H%M%SZ")
backup = prefs_path.with_suffix(prefs_path.suffix + f".ropen-backup-{stamp}")
shutil.copy2(prefs_path, backup)

with prefs_path.open("rb") as f:
    prefs = plistlib.load(f)
with defaults_path.open("rb") as f:
    default_rules = plistlib.load(f)["Rules"]

log_path = pathlib.Path.home() / "Library/Logs/ropen-iterm.log"
ropen_cmd = shlex.quote(str(ropen))
log_cmd = shlex.quote(str(log_path))
path_command = f'{ropen_cmd} --tty "\\(tty)" --cwd "\\(path)" --path "\\(matches[1])" >> {log_cmd} 2>&1'
object_command = f'{ropen_cmd} "\\(matches[1])" >> {log_cmd} 2>&1'

path_rule = {
    "notes": "ropen remote absolute path",
    "precision": "very_high",
    "regex": PATH_REGEX,
    "actions": [
        {
            "title": "Open remote path with ropen",
            "action": 2,
            "parameter": path_command,
        }
    ],
}
relative_path_rule = {
    "notes": "ropen remote relative path",
    "precision": "very_high",
    "regex": RELATIVE_PATH_REGEX,
    "actions": [
        {
            "title": "Open relative remote path with ropen",
            "action": 2,
            "parameter": path_command,
        }
    ],
}
object_rule = {
    "notes": "ropen object storage URI",
    "precision": "very_high",
    "regex": OBJECT_REGEX,
    "actions": [
        {
            "title": "Open object URI with ropen",
            "action": 2,
            "parameter": object_command,
        }
    ],
}

profiles = prefs.get("New Bookmarks") or []
changed = 0
for profile in profiles:
    rules = profile.get("Smart Selection Rules") or list(default_rules)
    rules = [r for r in rules if not str(r.get("notes", "")).startswith("ropen ")]
    profile["Smart Selection Rules"] = [object_rule, path_rule, relative_path_rule] + rules
    profile["Smart Selection Actions Use Interpolated Strings"] = True
    changed += 1

if changed == 0:
    raise SystemExit("no iTerm2 profiles found")

with tempfile.NamedTemporaryFile("wb", delete=False) as tmp:
    plistlib.dump(prefs, tmp, fmt=plistlib.FMT_BINARY)
    tmp_path = tmp.name

subprocess.run(["defaults", "import", "com.googlecode.iterm2", tmp_path], check=True)
pathlib.Path(tmp_path).unlink(missing_ok=True)

print(f"updated {changed} iTerm2 profiles")
print(f"backup: {backup}")
print(f"click logs: {log_path}")
print("If existing panes do not pick this up immediately, restart iTerm2 or open a new pane.")
`
