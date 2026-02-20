# Functional Overview

The `shell` package implements the **SHELL** action. It runs arbitrary operating system commands
inside the current flow executor process, captures their output, and reports the exit status back
to the flow runtime. The action can augment the command environment with additional variables,
including proxy settings sourced from previously declared VARIABLES entries.

# Linux Usage Example

The following flow excerpt targets Linux hosts by invoking `/bin/bash -lc`. It injects custom
environment variables, exports additional shell variables during execution, and consumes those
values in subsequent commands.

```yaml
tasks:
  - id: configure_proxy
    type: VARIABLES
    values:
      corp_proxy:
        type: proxy
        value:
          http: http://proxy.internal:8080
          https: http://proxy.internal:8443

  - id: greet_linux
    type: SHELL
    command: |
      export TARGET_USER="linux-user"
      export PATH="$PATH:/opt/custom/bin"
      echo "Hola ${TARGET_USER}, GREETING=${GREETING}"
    shell:
      program: /bin/bash
      args:
        - -lc
    workingDirectory: /tmp
    environment:
      - name: GREETING
        value: "Bienvenido"
    proxyVariables:
      - corp_proxy
```

When the task runs, the command exports `TARGET_USER` and extends `PATH`, making them available to
the rest of the script. The injected `GREETING` environment variable and `corp_proxy` values are
propagated into the process so that downstream tools honour the proxy configuration.

# Technical Implementation Details

* **Command execution:**
  * `Payload.Validate` enforces that a command string is present, normalises the working directory,
    and optionally records a custom interpreter. When `shell` options are omitted the action falls
    back to `/bin/sh -c` on Unix-like systems or `cmd.exe /C` on Windows.
  * `Execute` builds the concrete command (`exec.CommandContext`) and attaches buffers to capture
    `stdout` and `stderr`. The helper `resolveShell` combines the interpreter arguments with the
    validated command string.
  * `timeoutSeconds` controls an optional per-command deadline via `context.WithTimeout`, while
    `continueOnError` lets flows opt into receiving the structured result even when the exit code
    is non-zero.

* **Environment management:**
  * `environment` entries are validated to ensure names are well formed and optionally marked as
    `secret` to redact values in logs and results.
  * `proxy` flags on environment entries (or entries referenced through `proxyVariables`) expand to
    both upper- and lower-case proxy variables (`HTTP_PROXY`, `http_proxy`, etc.) so downstream
    tooling honours the configuration regardless of platform conventions.
  * `proxyVariables` looks up VARIABLES declared with `type: "proxy"`, normalises their contents,
    and injects the resulting map into the command environment.
  * `environmentBuilder` merges overrides with the inherited process environment, tracks the
    redacted view exposed in task results, and produces a deterministic snapshot for logging.

* **Result reporting and logging:**
  * `ExecutionResult` returns the command invocation (program plus arguments), working directory,
    exit code, captured output, execution duration, and the sanitised list of environment overrides.
  * `logCommand` and `logCommandOutcome` emit structured log messages summarising the invocation,
    applied environment variables, and the collected output to aid troubleshooting.
