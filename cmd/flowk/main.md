# Functional Overview

The `main.go` file provides the command-line entry point for the FlowK tool. It parses command-line flags to locate the flow definition, optionally controls which tasks should run, and launches the orchestration logic (or validates the flow definition without executing tasks). When incorrect arguments are provided it prints a human-friendly usage message and exits with an error.

# Technical Implementation Details

* **Logging configuration:** The standard library `log` package is configured with `log.SetFlags(0)` to remove timestamp prefixes so messages remain concise.
* **Argument parsing:**
  * `parseRunArgs` iterates over the raw `os.Args[1:]` slice and recognises both `-flag value` and `-flag=value` syntaxes. It supports the `-flow`, `-begin-from-task`, `-run-task`, `-run-subtask`, `-run-flow`, and `-validate-only` flags, plus a positional fallback for the required flow path.
  * The helper `parseFlagValue` consumes the next element in the argument list when the flag is encountered without an inline value, and returns detailed errors when values are missing or when unexpected positional arguments are present.
  * Mutual exclusivity is enforced between run modes (for example `-begin-from-task` versus `-run-task`), and `-validate-only` cannot be combined with execution or UI flags.
  * `runHelpMessage` formats a usage string dynamically using the program name so help output stays accurate.
* **Execution context:** A cancellable context is created with `context.WithCancel`, and the deferred `cancel` ensures resources are released if the application ends early.
* **Application invocation:** The `app.Run` function from `flowk/internal/app` receives the prepared context, file paths, default logger, and optional task identifiers. `app.ValidateFlow` loads the flow definition without running tasks when `-validate-only` is requested. Any error returned is surfaced to the user with `log.Fatalf`, which prints the message and terminates with a non-zero status.
