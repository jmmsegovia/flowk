# Contributing to FlowK

Thanks for your interest in contributing to FlowK 🚀

## Development setup

1. Install Go (version defined in `go.mod`).
2. Clone the repository.
3. Build the CLI:

```bash
CGO_ENABLED=0 go build -o ./bin/flowk ./cmd/flowk/main.go
```

4. (Optional) Build UI assets:

```bash
cd ui && npm ci && npm run build
```

## Local validation before opening a PR

Run these checks from repo root:

```bash
go test ./...
go vet ./...
./scripts/validate_flows.sh ./flows
```

## Pull request expectations

- Keep PRs focused and small when possible.
- Include tests or validation steps for behavior changes.
- Update docs when user-facing behavior changes.
- Use clear commit messages.

## Reporting bugs

Please include:
- FlowK version (`flowk version`)
- OS and architecture
- Reproducible flow JSON snippet
- Exact command and error output

## Security issues

Please **do not** open public issues for security vulnerabilities.
Use the process in `SECURITY.md`.
