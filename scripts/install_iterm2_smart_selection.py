#!/usr/bin/env python3
import argparse
import datetime as dt
import pathlib
import plistlib
import shlex
import shutil
import subprocess
import tempfile


PATH_REGEX = r"""((?:/[^ \t\r\n"'<>]+(?:\r?\n)?)+)(?::([0-9]+)(?::([0-9]+))?)?"""
OBJECT_REGEX = r"""((?:s3|gs|az|rclone)://[^ \t\r\n"'<>]+(?:\r?\n[^ \t\r\n"'<>]+)*)"""


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ropen", default=default_ropen_path())
    parser.add_argument("--prefs", default=str(pathlib.Path.home() / "Library/Preferences/com.googlecode.iterm2.plist"))
    parser.add_argument("--defaults", default="/Applications/iTerm.app/Contents/Resources/SmartSelectionRules.plist")
    args = parser.parse_args()

    prefs_path = pathlib.Path(args.prefs)
    defaults_path = pathlib.Path(args.defaults)
    if not pathlib.Path(args.ropen).exists():
        raise SystemExit(f"ropen binary not found: {args.ropen}")
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
    ropen_cmd = shlex.quote(args.ropen)
    log_cmd = shlex.quote(str(log_path))
    path_command = f'{ropen_cmd} --tty "\\(tty)" --path "\\(matches[1])" >> {log_cmd} 2>&1'
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
        profile["Smart Selection Rules"] = [object_rule, path_rule] + rules
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
    return 0


def default_ropen_path() -> str:
    found = shutil.which("ropen")
    if found:
        return found
    try:
        result = subprocess.run(
            ["go", "env", "GOPATH"],
            check=True,
            text=True,
            capture_output=True,
        )
    except Exception:
        return "ropen"
    return str(pathlib.Path(result.stdout.strip()) / "bin" / "ropen")


if __name__ == "__main__":
    raise SystemExit(main())
