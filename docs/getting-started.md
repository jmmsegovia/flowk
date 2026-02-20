# Getting Started with FlowK

This guide will walk you through installing FlowK, configuring your environment, and running your first flow.

## Installation

### Prerequisites
- **Go**: Version 1.21 or higher.
- **Make**: (Optional) for using Makefile shortcuts.

### Building from Source

Clone the repository and build the binary:

```bash
git clone https://github.com/yourusername/flowk.git
cd flowk
go build -o ./bin/flowk ./cmd/flowk/main.go
```

Verify the installation:

```bash
./bin/flowk -version
```

## Running FlowK

FlowK has two primary modes of operation: **CLI Mode** (headless) and **UI Mode** (server).

### CLI Mode (Headless)

Useful for CI/CD pipelines or scripting where no visual interface is needed.

```bash
./bin/flowk run -flow ./path/to/your/flow.json
```

**Common Flags:**
- `-flow <path>`: Path to the JSON flow definition file (required).
- `-validate-only`: Validates the flow schema and imports without executing tasks.
- `-config <path>`: Path to a custom `config.yaml` file.
- `-vars`: Pass dynamic variables (e.g., `-vars "env=prod,retries=3"`).

### UI Mode (Visual)

Starts a local web server to visualize the flow execution in real-time.

```bash
./bin/flowk run -serve-ui -flow ./path/to/your/flow.json
```

Access the UI at: `http://localhost:8080` (default)

## Configuration

FlowK looks for a configuration file in the following order:
1. Path specified by `-config` flag.
2. `$XDG_CONFIG_HOME/flowk/config.yaml`
3. Default values if no file is found.

### Example `config.yaml`

```yaml
ui:
  host: "0.0.0.0"
  port: 8080
  cors_allowed_origins: ["*"]
  static_dir: "./ui/dist" # Path to built UI assets

logging:
  level: "info"
  format: "text" # or "json"
```

## Next Steps

- Learn about [Core Concepts](./core-concepts.md) like Flows, Tasks, and Variables.
- Explore the [Actions Reference](./actions/README.md) to see what you can automate.
- Check out the [UI Guide](./ui-guide.md) to master the visual interface.
