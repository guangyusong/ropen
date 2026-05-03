# Architecture

`ropen` solves one problem:

```text
remote artifact reference -> local file -> system opener
```

It does not manage machines, run agents, or provide a cloud dashboard.

## Flow

```text
iTerm2 Smart Selection / CLI / future OSC 8 URL
  -> ropen target parser
  -> resolver
     -> SSH copy/cache
     -> object-storage copy/cache
     -> local path
  -> macOS open or VS Code for line-numbered text files
```

## v0 Design

The first version copies files into a local cache and opens the copy. This is less magical than mounting, but it has better failure modes:

- no kernel extension required
- no long-lived mount process
- works with common CLIs and existing credentials
- easy to inspect and garbage collect

## Backends

### SSH

Uses:

- `ssh` for remote stat
- `scp -p` for download

Host identity comes from:

- explicit `host:/path`
- `--host`, `--user`, `--cwd`, `--path` flags from terminal integrations
- optional host aliases in `~/.config/ropen/config.json`

### Object Storage

Uses existing local CLIs:

- `aws s3 cp` for `s3://`
- `gcloud storage cp` or `gsutil cp` for `gs://`
- `az storage blob download` for `az://account/container/blob`
- `rclone copyto` for `rclone://remote/path`

Object storage is not POSIX. Mounting may be added later for large files, but copy/cache is the v0 behavior.

## Security Model

Remote terminal output is untrusted text. `ropen` assumes the user made an intentional click, but still keeps a narrow local surface:

- no credential storage
- no remote code execution beyond `ssh stat` and file copy
- no local shell interpolation
- remote paths with control characters are rejected
- SSH paths must be inside allowed roots
- cached files are opened through OS file associations

## Non-Goals

- remote desktop
- terminal multiplexing
- cloud cost inventory
- background agents on remote hosts
- replacing VS Code Remote-SSH
- replacing rclone, sshfs, or provider CLIs
