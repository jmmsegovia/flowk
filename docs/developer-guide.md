# Developer Guide

This guide is for developers who want to contribute to FlowK or understand its internal architecture.

## Architecture Overview

FlowK is built as a modular monolith in Go. For a comprehensive technical deep-dive (including C4 diagrams and data flow), see the **[Architecture Reference](./architecture.md)**.

- **`cmd/flowk`**: Entry point. Handles CLI flags and startup.
- **`internal/app`**: The core execution engine. Orchestrates flow loading and task execution.
- **`internal/flow`**: Data models for Flows and Tasks, including JSON schema validation.
- **`internal/actions`**: Pluggable action implementations.
- **`internal/server/ui`**: Embedded HTTP server for the Web UI.

### Action Plugin System
Actions are self-contained packages in `internal/actions`. They register themselves via `init()` functions called by `registry.Register()`.

## Setting Up Development Environment

1.  **Clone the repo**:
    ```bash
    git clone https://github.com/your-org/flowk.git
    cd flowk
    ```

2.  **Build**:
    ```bash
    go build -o ./bin/flowk ./cmd/flowk/main.go
    ```

3.  **Run Tests**:
    ```bash
    go test ./...
    ```

## Adding a New Action

To add a new action (e.g., `SLACK`):

1.  Create a new directory: `internal/actions/network/slack`.
2.  Define your task config struct and validation logic.
3.  Implement the `Action` interface (`Name()` and `Execute()`).
4.  Implement `SchemaProvider` to return the JSON schema for validation.
5.  Call `registry.Register` in `init()`.

## Contributing to UI

The UI source code is located in `ui/`. It is a React application.

1.  **Install dependencies**:
    ```bash
    cd ui
    npm install
    ```

2.  **Run dev server**:
    ```bash
    npm run dev
    ```

3.  **Build for production**:
    ```bash
    npm run build
    ```
    The built assets should be embedded or served by the Go server.
