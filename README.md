# ropen

Click a remote path in your terminal and open it locally in the right macOS app.

`ropen` is for SSH-heavy workflows where terminal output points at files that live on another machine:

```text
/home/user/output.mp4
/var/log/app/error.log
/tmp/results.csv
s3://bucket/run/output.parquet
gs://bucket/video.mov
```

The terminal sees those as plain text. `ropen` turns them into local files by copying them through SSH or cloud CLIs, caching the result, and calling `open`.

## Status

Early v0. The current implementation is a local CLI:

- SSH paths via `ssh` + `scp`
- `s3://` via AWS CLI
- `gs://` via `gcloud storage cp` or `gsutil`
- `az://account/container/blob` via Azure CLI
- `rclone://remote/path` via `rclone copyto`
- iTerm2 Smart Selection friendly flags
- `ropen doctor` for setup checks
- `ropen iterm install` for iTerm2 setup

No daemon, no remote agent, no credentials stored.

## Install

With Homebrew:

```bash
brew tap guangyusong/tap
brew install ropen
ropen doctor
ropen iterm install
```

From source:

```bash
go install github.com/guangyusong/ropen@latest
ropen doctor
ropen iterm install
```

For local development:

```bash
go build ./...
go install .
ropen doctor
```

Tagged GitHub releases also provide prebuilt binaries.

## Use

Open a remote SSH file:

```bash
ropen vm1:/home/user/output.mp4
ropen ubuntu@vm1:/var/log/app/error.log
```

Open from terminal integration flags:

```bash
ropen --host vm1 --user ubuntu --cwd /home/ubuntu/project --path results.csv
```

Open cloud/object paths:

```bash
ropen s3://bucket/path/results.csv
ropen gs://bucket/path/output.mp4
ropen az://account/container/path/file.pdf
ropen rclone://myremote/path/file.mov
```

Print the local cached path without opening:

```bash
ropen --no-open vm1:/tmp/results.csv
```

Remove cached files older than 7 days:

```bash
ropen --gc 7
```

Check local setup:

```bash
ropen doctor
ropen --version
```

## iTerm2 Setup

iTerm2 is the best v0 path because Smart Selection can pass clicked text into `ropen`. Install the rules with:

```bash
ropen iterm install
```

If you are developing from a checkout and want iTerm2 to run a specific binary:

```bash
ropen iterm install --ropen "$(go env GOPATH)/bin/ropen"
```

The installer updates all iTerm2 profiles, creates a timestamped preferences backup, and logs click errors to:

```text
~/Library/Logs/ropen-iterm.log
```

Manual setup is also possible.

Add a Smart Selection rule for absolute paths:

```regex
((?:/[^ \t\r\n"'<>]+(?:\r?\n)?)+)(?::([0-9]+)(?::([0-9]+))?)?
```

Action: **Run Command**

```bash
<absolute-path-to-ropen> --tty "\(tty)" --path "\(matches[1])"
```

Use an absolute path because GUI-launched terminal apps may not inherit your shell `PATH`. After `go install .`, this is usually:

```bash
$(go env GOPATH)/bin/ropen
```

Check **Use interpolated strings for parameters** for the action. The `--tty` flag lets `ropen` recover the SSH destination from the local `ssh` process when iTerm2 Shell Integration is not installed on the remote host.

For object storage URIs, add another rule:

```regex
((?:s3|gs|az|rclone)://[^ \t\r\n"'<>]+(?:\r?\n[^ \t\r\n"'<>]+)*)
```

The regexes allow newline-wrapped paths. `ropen` removes those terminal wrap newlines before resolving the path.

Action:

```bash
<absolute-path-to-ropen> "\(matches[1])"
```

Or run the installer:

```bash
python3 scripts/install_iterm2_smart_selection.py
```

## Config

Config lives at:

```text
macOS: ~/Library/Application Support/ropen/config.json
Linux:  ~/.config/ropen/config.json
```

Example:

```json
{
  "max_bytes": 1073741824,
  "allowed_roots": ["/home", "/tmp", "/var/log", "/mnt", "/data"],
  "hosts": {
    "vm1": {
      "alias": "ubuntu@vm1.tailnet-name.ts.net",
      "allowed_roots": ["/home/ubuntu", "/tmp", "/var/log"]
    }
  }
}
```

`ropen` uses existing credentials only:

- SSH config / ssh-agent for SSH
- AWS CLI credentials for `s3://`
- gcloud / gsutil auth for `gs://`
- Azure CLI auth for `az://`
- rclone config for `rclone://`

## Safety

`ropen` is intentionally small and local:

- no remote agent
- no background daemon
- no credential storage
- no shell string interpolation for local commands
- remote paths are limited to configured roots
- files above `max_bytes` are rejected when size can be checked
- cached files are inspectable under the user cache directory

The v0 transport copies files locally before opening them. Future versions may add mount backends for very large files, but copy/cache is simpler and safer for the first release.

## Roadmap

- macOS URL handler: `ropen://...`
- OSC 8 helper for Ghostty and tools that can emit explicit links
- better rclone mount/cache strategy for large object-storage files
- optional read-only SSHFS backend
- Homebrew formula
