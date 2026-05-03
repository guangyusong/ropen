# Changelog

All notable changes to `ropen` are documented here.

## Unreleased

No unreleased changes yet.

## v0.1.0

Initial public release.

- Add `ropen doctor` for dependency, config, iTerm2, and SSH-pane checks.
- Add `ropen iterm install` for installing iTerm2 Smart Selection rules from the binary.
- Add `ropen version` / `ropen --version`.
- Add clearer SSH failure messages.
- Add release workflow for tagged builds.
- Open SSH remote paths by copying through `scp`, caching locally, and dispatching to the OS opener.
- Open `s3://`, `gs://`, `az://`, and `rclone://` object paths through existing local CLIs.
- Support iTerm2 Smart Selection integration.
- Infer SSH destinations from the local iTerm pane TTY when shell integration does not know the remote host.
- Resolve detected raw SSH hostnames/IPs back through `~/.ssh/config` aliases when possible.
- Normalize newline-wrapped terminal paths before resolving.
