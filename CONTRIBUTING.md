# Contributing

Thanks for improving `ropen`.

## Development

Requirements:

- Go 1.22+
- macOS for iTerm2 integration testing
- Existing SSH/cloud CLI credentials for live transport tests

Common commands:

```bash
make test
make build
go run . --version
go run . doctor
```

Install the local binary:

```bash
make install
ropen doctor
ropen iterm install
```

## Pull Requests

Keep changes small and focused. Please include tests for parser, config, and transport behavior when possible.

Before opening a PR:

```bash
gofmt -w .
go test ./...
go build ./...
python3 -m py_compile scripts/install_iterm2_smart_selection.py
```

## Scope

Good fits:

- safer path parsing
- clearer diagnostics
- terminal integration improvements
- conservative transport backends
- packaging and release polish

Non-goals for now:

- remote agents
- remote desktop
- cloud dashboards
- terminal multiplexing
- replacing SSH, rclone, or provider CLIs
