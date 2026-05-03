# Security Policy

`ropen` is a local developer tool that copies remote files and asks the operating system to open the local copy. It does not run a daemon, install a remote agent, or store credentials.

## Reporting A Vulnerability

Please report security issues privately through GitHub Security Advisories for this repository if available. If that is not available, open a minimal public issue that says you have a security report without posting exploit details.

## Security Model

- `ropen` uses existing local credentials only: SSH config/agent, AWS CLI, gcloud/gsutil, Azure CLI, or rclone.
- `ropen` does not store cloud tokens, SSH keys, passwords, or session credentials.
- SSH paths are limited to configured allowed roots.
- Files above the configured maximum size are rejected when size can be checked.
- iTerm2 Smart Selection actions are local commands and may be triggered by terminal text. Treat terminal output as untrusted.

## Hardening Notes

- Review `~/.config/ropen/config.json` before widening `allowed_roots`.
- Prefer read-only credentials for object-storage workflows when possible.
- Avoid clicking links or paths printed by untrusted remote programs.
- Check `~/Library/Logs/ropen-iterm.log` if an iTerm2 click behaves unexpectedly.
