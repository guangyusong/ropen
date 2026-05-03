# Ghostty / OSC 8

Ghostty can open terminal hyperlinks, but a plain path like `/tmp/results.csv` does not contain remote host context. For Ghostty, the reliable path is explicit OSC 8 links.

This helper can live on a remote VM as `ropen-link`:

```sh
#!/bin/sh
set -eu

path="${1:?usage: ropen-link PATH}"
case "$path" in
  /*) ;;
  *) path="$(pwd)/$path" ;;
esac

host="${ROPEN_HOST:-$(hostname)}"
user="${USER:-}"

python3 - "$host" "$user" "$path" <<'PY'
import sys
import urllib.parse

host, user, path = sys.argv[1:4]
params = {"host": host, "path": path}
if user:
    params["user"] = user

url = "ropen://open?" + urllib.parse.urlencode(params)
label = path
sys.stdout.write(f"\033]8;;{url}\033\\{label}\033]8;;\033\\\n")
PY
```

The `ropen://` URL handler is planned. Until then, iTerm2 Smart Selection is the main supported integration.
